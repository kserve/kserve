/*
Copyright 2023 The KServe Authors.

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

package modelconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

//nolint:unparam // keeping param for test flexibility
func makeTrainedModel(name, isvc, model string, deletion bool) *v1alpha1.TrainedModel {
	tm := &v1alpha1.TrainedModel{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.TrainedModelSpec{
			InferenceService: isvc,
			Model:            v1alpha1.ModelSpec{StorageURI: model},
		},
	}
	if deletion {
		now := metav1.NewTime(time.Now())
		tm.DeletionTimestamp = &now
	}
	return tm
}

//nolint:unparam // keeping param for test flexibility
func makeConfigMap(name, ns string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: data,
	}
}

func TestModelConfigReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	const (
		ns         = "test-ns"
		isvc       = "my-isvc"
		model      = "my-model"
		tmName     = "tm1"
		shardId    = "0"
		configName = "my-isvc-0-modelconfig"
	)

	t.Run("configmap not found", func(t *testing.T) {
		tm := makeTrainedModel(tmName, isvc, model, false)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		clientset := k8sfake.NewSimpleClientset()
		clientset.PrependReactor("get", "configmaps", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("not found")
		})

		reconciler := NewModelConfigReconciler(client, clientset, scheme)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

		err := reconciler.Reconcile(t.Context(), req, tm)
		assert.Error(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		tm := makeTrainedModel(tmName, isvc, model, false)
		cm := makeConfigMap(configName, ns, map[string]string{})
		client := &fakeErrorClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()}
		clientset := k8sfake.NewSimpleClientset(cm)

		reconciler := NewModelConfigReconciler(client, clientset, scheme)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

		err := reconciler.Reconcile(t.Context(), req, tm)
		assert.Error(t, err)
	})
}

func TestModelConfigReconciler_Reconcile_AddOrUpdateModel_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	const (
		ns         = "test-ns"
		isvc       = "my-isvc"
		model      = "gs://foo/bar"
		tmName     = "tm1"
		shardId    = "0"
		configName = "modelconfig-my-isvc-0"
	)

	tm := makeTrainedModel(tmName, isvc, model, false)
	cm := makeConfigMap(configName, ns, map[string]string{})

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	clientset := k8sfake.NewSimpleClientset(cm)

	reconciler := NewModelConfigReconciler(client, clientset, scheme)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

	err := reconciler.Reconcile(t.Context(), req, tm)
	assert.NoError(t, err)
}

func TestModelConfigReconciler_Reconcile_DeleteModel_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	const (
		ns         = "test-ns"
		isvc       = "my-isvc"
		model      = "gs://foo/bar"
		tmName     = "tm1"
		shardId    = "0"
		configName = "modelconfig-my-isvc-0"
	)

	tm := makeTrainedModel(tmName, isvc, model, true)
	cm := makeConfigMap(configName, ns, map[string]string{
		tmName: `{"name":"tm1","storageUri":"gs://foo/bar"}`,
	})

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	clientset := k8sfake.NewSimpleClientset(cm)

	reconciler := NewModelConfigReconciler(client, clientset, scheme)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

	err := reconciler.Reconcile(t.Context(), req, tm)
	assert.NoError(t, err)
}

func TestModelConfigReconciler_Reconcile_ProcessError_AddOrUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	const (
		ns         = "test-ns"
		isvc       = "my-isvc"
		model      = "gs://foo/bar"
		tmName     = "tm1"
		shardId    = "0"
		configName = "my-isvc-0-modelconfig"
	)

	tm := makeTrainedModel(tmName, isvc, model, false)
	// Intentionally create a configmap with invalid data to cause Process to fail
	cm := makeConfigMap(configName, ns, map[string]string{
		"invalid": string([]byte{0xff, 0xfe}),
	})

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	clientset := k8sfake.NewSimpleClientset(cm)

	reconciler := NewModelConfigReconciler(client, clientset, scheme)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

	err := reconciler.Reconcile(t.Context(), req, tm)
	assert.Error(t, err)
}

func TestModelConfigReconciler_Reconcile_ProcessError_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	const (
		ns         = "test-ns"
		isvc       = "my-isvc"
		model      = "gs://foo/bar"
		tmName     = "tm1"
		shardId    = "0"
		configName = "my-isvc-0-modelconfig"
	)

	tm := makeTrainedModel(tmName, isvc, model, true)
	// Intentionally create a configmap with invalid data to cause Process to fail
	cm := makeConfigMap(configName, ns, map[string]string{
		"invalid": string([]byte{0xff, 0xfe}),
	})

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	clientset := k8sfake.NewSimpleClientset(cm)

	reconciler := NewModelConfigReconciler(client, clientset, scheme)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: tmName}}

	err := reconciler.Reconcile(t.Context(), req, tm)
	assert.Error(t, err)
}

// fakeErrorClient returns error on Update
type fakeErrorClient struct {
	client.Client
}

func (f *fakeErrorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return errors.New("update error")
}
