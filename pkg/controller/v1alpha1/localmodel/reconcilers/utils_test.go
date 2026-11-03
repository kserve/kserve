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
	"testing"

	"github.com/go-logr/logr"
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
