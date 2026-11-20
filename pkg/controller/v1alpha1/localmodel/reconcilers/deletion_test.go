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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// newConfigMapOnlyClientset returns a clientset backed by a test server that
// serves an empty inferenceservice-config ConfigMap and answers every other
// request with 404 Not Found.
func newConfigMapOnlyClientset(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	configMapPath := "/api/v1/namespaces/" + constants.KServeNamespace + "/configmaps/" + constants.InferenceServiceConfigMapName
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == configMapPath {
			_, _ = w.Write([]byte(`{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"` +
				constants.InferenceServiceConfigMapName + `","namespace":"` + constants.KServeNamespace + `"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
	}))
	t.Cleanup(srv.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	return clientset
}

func newDeletionTestClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(), scheme
}

func localModelNodeWithModels(name string, models ...v1alpha1.LocalModelInfo) *v1alpha1.LocalModelNode {
	return &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1alpha1.LocalModelNodeSpec{LocalModels: models},
	}
}

func assertLocalModels(t *testing.T, c client.Client, nodeName string, want ...string) {
	t.Helper()
	node := &v1alpha1.LocalModelNode{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: nodeName}, node); err != nil {
		t.Fatalf("get LocalModelNode %s: %v", nodeName, err)
	}
	got := make([]string, 0, len(node.Spec.LocalModels))
	for _, m := range node.Spec.LocalModels {
		got = append(got, m.Namespace+"/"+m.ModelName)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("LocalModelNode %s models = %v, want %v", nodeName, got, want)
	}
}

func TestLocalModelCacheDeletionWithDeletedNodeGroup(t *testing.T) {
	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "model",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"deleted-group"},
		},
	}
	node := localModelNodeWithModels("node-1",
		v1alpha1.LocalModelInfo{ModelName: "model"},
		v1alpha1.LocalModelInfo{ModelName: "other"},
	)
	c, scheme := newDeletionTestClient(t, cache, node)
	r := &LocalModelReconciler{
		Client:    c,
		Clientset: newConfigMapOnlyClientset(t),
		Log:       logr.Discard(),
		Scheme:    scheme,
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: cache.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// The fake client deletes the object once its last finalizer is removed.
	if err := c.Get(context.Background(), types.NamespacedName{Name: cache.Name}, &v1alpha1.LocalModelCache{}); !apierr.IsNotFound(err) {
		t.Fatalf("expected LocalModelCache to be gone after finalizer removal, got err = %v", err)
	}
	assertLocalModels(t, c, "node-1", "/other")
}

func TestLocalModelCacheWithDeletedNodeGroupStillErrorsWhenNotDeleting(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "model"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"deleted-group"},
		},
	}
	c, scheme := newDeletionTestClient(t, cache)
	r := &LocalModelReconciler{
		Client:    c,
		Clientset: newConfigMapOnlyClientset(t),
		Log:       logr.Discard(),
		Scheme:    scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: cache.Name}})
	if !apierr.IsNotFound(err) {
		t.Fatalf("Reconcile() error = %v, want NotFound for the missing node group", err)
	}
}

func TestLocalModelNamespaceCacheDeletionWithDeletedNodeGroup(t *testing.T) {
	now := metav1.NewTime(time.Now())
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "model",
			Namespace:         "team",
			DeletionTimestamp: &now,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"deleted-group"},
		},
	}
	node := localModelNodeWithModels("node-1",
		v1alpha1.LocalModelInfo{ModelName: "model", Namespace: "team"},
		v1alpha1.LocalModelInfo{ModelName: "model", Namespace: "other-team"},
		v1alpha1.LocalModelInfo{ModelName: "model"},
	)
	c, scheme := newDeletionTestClient(t, cache, node)
	r := &LocalModelNamespaceCacheReconciler{
		Client:    c,
		Clientset: newConfigMapOnlyClientset(t),
		Log:       logr.Discard(),
		Scheme:    scheme,
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if err := c.Get(context.Background(), types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}, &v1alpha1.LocalModelNamespaceCache{}); !apierr.IsNotFound(err) {
		t.Fatalf("expected LocalModelNamespaceCache to be gone after finalizer removal, got err = %v", err)
	}
	assertLocalModels(t, c, "node-1", "other-team/model", "/model")
}
