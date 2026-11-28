/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcilers

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// nodeListErrorClient fails every Node list so GetNodesFromNodeGroup returns an error.
type nodeListErrorClient struct {
	client.Client
	err error
}

func (c *nodeListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.NodeList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

func newLocalModelCacheClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelCache{}).
		WithObjects(objs...).
		Build()
}

func isvcConsumer(name, namespace string) cacheConsumers {
	return cacheConsumers{isvcs: []v1beta1.InferenceService{{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}}}
}

type nodeGetRecorder struct {
	client.Reader
	names []string
}

func (r *nodeGetRecorder) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*v1alpha1.LocalModelNode); ok {
		r.names = append(r.names, key.Name)
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

type pagedNodeReader struct {
	client.Reader
	secondPageErr error
	requests      []string
}

func (r *pagedNodeReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := (&client.ListOptions{}).ApplyOptions(opts)
	r.requests = append(r.requests, options.Continue)
	if options.Limit <= 0 {
		return errors.New("expected a bounded LocalModelNode list")
	}
	switch options.Continue {
	case "":
		page := list.(*v1alpha1.LocalModelNodeList)
		page.Items = []v1alpha1.LocalModelNode{{ObjectMeta: metav1.ObjectMeta{Name: "unrelated"}}}
		page.Continue = "second"
		return nil
	case "second":
		if r.secondPageErr != nil {
			return r.secondPageErr
		}
		return r.Reader.List(ctx, list)
	default:
		return errors.New("unexpected continuation token")
	}
}

func TestDeleteModelFromNodesRetainsFinalizerAcrossPageErrors(t *testing.T) {
	ctx := t.Context()
	cache := &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "model", Finalizers: []string{FinalizerName}}}
	node := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "last-node"},
		Spec:       v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{{ModelName: cache.Name}}},
	}
	cl := newLocalModelCacheClient(t, cache, node)
	pageErr := errors.New("second page unavailable")
	reader := &pagedNodeReader{Reader: cl, secondPageErr: pageErr}
	sweep := func() error {
		_, err := DeleteModelFromNodes(ctx, cl, reader, nil, logr.Discard(), cache, nil, nil, nil)
		return err
	}
	if err := sweep(); !errors.Is(err, pageErr) {
		t.Fatalf("first sweep error = %v, want %v", err, pageErr)
	}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(cache), cache); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cache.Finalizers, FinalizerName) {
		t.Fatal("lost finalizer before the last page was read")
	}

	reader.secondPageErr = nil
	if err := sweep(); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"", "second", "", "second"}, reader.requests); diff != "" {
		t.Errorf("continuation tokens (-want +got):\n%s", diff)
	}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(node), node); err != nil {
		t.Fatal(err)
	}
	if len(node.Spec.LocalModels) != 0 {
		t.Errorf("last page still has %v", node.Spec.LocalModels)
	}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(cache), cache); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cache.Finalizers, FinalizerName) {
		t.Error("finalizer still present after the last page was cleaned")
	}
}

func TestDeleteModelFromNodesOnlyFetchesMatchingNodes(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{
		Name: "target", Namespace: "tenant", Finalizers: []string{NamespaceCacheFinalizerName},
	}}
	target := v1alpha1.LocalModelInfo{ModelName: cache.Name, Namespace: cache.Namespace}
	otherModel := v1alpha1.LocalModelInfo{ModelName: "other", Namespace: cache.Namespace}
	otherNamespace := v1alpha1.LocalModelInfo{ModelName: cache.Name, Namespace: "other-tenant"}
	node := func(name string, models ...v1alpha1.LocalModelInfo) *v1alpha1.LocalModelNode {
		return &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       v1alpha1.LocalModelNodeSpec{LocalModels: models},
		}
	}
	matching := node("matching", otherModel, target, target)
	cl := newLocalModelCacheClient(t, cache, matching, node("unrelated", otherModel, otherNamespace), node("empty"))
	reader := &nodeGetRecorder{Reader: cl}

	ctx := t.Context()
	if _, err := DeleteModelFromNodes(ctx, cl, reader, nil, logr.Discard(), nil, cache, nil, nil); err != nil {
		t.Fatalf("DeleteModelFromNodes() error = %v", err)
	}
	if diff := cmp.Diff([]string{matching.Name}, reader.names); diff != "" {
		t.Errorf("LocalModelNode Gets (-want +got):\n%s", diff)
	}
	got := &v1alpha1.LocalModelNode{}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(matching), got); err != nil {
		t.Fatalf("get matching node: %v", err)
	}
	if diff := cmp.Diff([]v1alpha1.LocalModelInfo{otherModel}, got.Spec.LocalModels); diff != "" {
		t.Errorf("remaining LocalModels (-want +got):\n%s", diff)
	}
}

func TestReconcileLocalModelNodeRecordsConsumersWithoutNodeGroups(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}}
	cl := newLocalModelCacheClient(t, cache)

	err := ReconcileLocalModelNode(context.Background(), cl, logr.Discard(), cache, nil,
		map[string]*v1alpha1.LocalModelNodeGroup{}, isvcConsumer("isvc", "ns"))
	if err != nil {
		t.Fatalf("ReconcileLocalModelNode() error = %v", err)
	}

	persisted := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cache"}, persisted); err != nil {
		t.Fatalf("get cache: %v", err)
	}
	want := v1alpha1.NamespacedName{Name: "isvc", Namespace: "ns"}
	if len(persisted.Status.InferenceServices) != 1 {
		t.Fatalf("Status.InferenceServices = %#v, want exactly [%#v]", persisted.Status.InferenceServices, want)
	}
	if got := persisted.Status.InferenceServices[0]; got != want {
		t.Fatalf("Status.InferenceServices[0] = %#v, want %#v", got, want)
	}
}

func TestReconcileLocalModelNodeRecordsConsumersOnNodeGroupError(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}}
	listErr := errors.New("node list failed")
	cl := &nodeListErrorClient{Client: newLocalModelCacheClient(t, cache), err: listErr}
	nodeGroups := map[string]*v1alpha1.LocalModelNodeGroup{
		"gpu": {ObjectMeta: metav1.ObjectMeta{Name: "gpu"}},
	}

	err := ReconcileLocalModelNode(context.Background(), cl, logr.Discard(), cache, nil, nodeGroups, isvcConsumer("isvc", "ns"))
	if !errors.Is(err, listErr) {
		t.Fatalf("ReconcileLocalModelNode() error = %v, want %v", err, listErr)
	}

	persisted := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cache"}, persisted); err != nil {
		t.Fatalf("get cache: %v", err)
	}
	if len(persisted.Status.InferenceServices) != 1 || persisted.Status.InferenceServices[0].Name != "isvc" {
		t.Fatalf("Status.InferenceServices = %#v, want the isvc consumer recorded despite the node-group error", persisted.Status.InferenceServices)
	}
}

func TestUpdateLocalModelNodeReplacesImagePullSecrets(t *testing.T) {
	node := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{{
				ModelName:        "cache",
				SourceModelUri:   "oci://registry.example/model:v1",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred-a"}},
			}},
		},
	}
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri:   "oci://registry.example/model:v1",
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred-b"}},
		},
	}
	cl := newLocalModelCacheClient(t, node, cache)
	if err := UpdateLocalModelNode(context.Background(), cl, logr.Discard(), node, cache, nil, "gpu"); err != nil {
		t.Fatalf("UpdateLocalModelNode() error = %v", err)
	}
	got := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "worker"}, got); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if len(got.Spec.LocalModels) != 1 || len(got.Spec.LocalModels[0].ImagePullSecrets) != 1 ||
		got.Spec.LocalModels[0].ImagePullSecrets[0].Name != "reg-cred-b" {
		t.Fatalf("LocalModels = %#v, want imagePullSecrets [reg-cred-b]", got.Spec.LocalModels)
	}
}
