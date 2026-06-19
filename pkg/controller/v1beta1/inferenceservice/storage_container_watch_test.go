/*
Copyright 2025 The KServe Authors.

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

package inferenceservice

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestStorageContainerFunc(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	isvc1 := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc1", Namespace: "ns1"},
	}
	isvc2 := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc2", Namespace: "ns1"},
	}
	isvcOtherNS := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc3", Namespace: "ns2"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(isvc1, isvc2, isvcOtherNS).Build()

	r := &InferenceServiceReconciler{
		Client: fakeClient,
		Log:    logr.Discard(),
	}

	sc := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sc", Namespace: "ns1"},
	}

	requests := r.storageContainerFunc(context.Background(), sc)
	assert.Len(t, requests, 2, "should enqueue both ISVCs in ns1")
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "isvc1")
	assert.Contains(t, names, "isvc2")
}

func TestStorageContainerFunc_WrongType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &InferenceServiceReconciler{
		Client: fakeClient,
		Log:    logr.Discard(),
	}

	// Pass wrong type — should return nil
	csc := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-type"},
	}
	requests := r.storageContainerFunc(context.Background(), csc)
	assert.Nil(t, requests)
}

func TestClusterStorageContainerFunc(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	isvc1 := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc1", Namespace: "ns1"},
	}
	isvc2 := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "isvc2", Namespace: "ns2"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(isvc1, isvc2).Build()

	r := &InferenceServiceReconciler{
		Client: fakeClient,
		Log:    logr.Discard(),
	}

	csc := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-csc"},
	}

	requests := r.clusterStorageContainerFunc(context.Background(), csc)
	assert.Len(t, requests, 2, "should enqueue ISVCs from all namespaces")
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "isvc1")
	assert.Contains(t, names, "isvc2")
}

func TestClusterStorageContainerFunc_WrongType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &InferenceServiceReconciler{
		Client: fakeClient,
		Log:    logr.Discard(),
	}

	// Pass wrong type — should return nil
	sc := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-type", Namespace: "ns1"},
	}
	requests := r.clusterStorageContainerFunc(context.Background(), sc)
	assert.Nil(t, requests)
}
