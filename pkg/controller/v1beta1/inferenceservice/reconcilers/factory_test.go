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

package reconcilers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestCreateWorkloadReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	factory := NewReconcilerFactory()
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	params := WorkloadReconcilerParams{
		Client:        fakeClient,
		Scheme:        scheme,
		ComponentMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", Labels: map[string]string{"app": "test"}},
		ComponentExt:  &v1beta1.ComponentExtensionSpec{},
		PodSpec:       &corev1.PodSpec{Containers: []corev1.Container{{Name: "test"}}},
	}

	// Should succeed for Standard mode
	rec, err := factory.CreateWorkloadReconciler(t.Context(), constants.Standard, params)
	require.NoError(t, err)
	assert.NotNil(t, rec)

	// Should fail for unsupported modes
	rec, err = factory.CreateWorkloadReconciler(t.Context(), constants.Knative, params)
	require.Error(t, err)
	assert.Nil(t, rec)
}

func TestCreateServiceReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	factory := NewReconcilerFactory()
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	params := ServiceReconcilerParams{
		Client:        fakeClient,
		Scheme:        scheme,
		ComponentMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		ComponentExt:  &v1beta1.ComponentExtensionSpec{},
		PodSpec:       &corev1.PodSpec{Containers: []corev1.Container{{Name: "test"}}},
	}

	// Should succeed for Standard mode
	rec, err := factory.CreateServiceReconciler(constants.Standard, params)
	require.NoError(t, err)
	assert.NotNil(t, rec)

	// Should fail for unsupported modes
	rec, err = factory.CreateServiceReconciler(constants.Knative, params)
	require.Error(t, err)
	assert.Nil(t, rec)
}

func TestCreateIngressReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	factory := NewReconcilerFactory()
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	fakeClientset := fake.NewSimpleClientset()

	// Standard mode with Gateway API
	params := IngressReconcilerParams{
		Client:        fakeClient,
		Clientset:     fakeClientset,
		Scheme:        scheme,
		IngressConfig: &v1beta1.IngressConfig{EnableGatewayAPI: true},
		IsvcConfig:    &v1beta1.InferenceServicesConfig{},
	}
	rec, err := factory.CreateIngressReconciler(constants.Standard, params)
	require.NoError(t, err)
	assert.NotNil(t, rec)

	// Standard mode with Ingress
	params.IngressConfig.EnableGatewayAPI = false
	rec, err = factory.CreateIngressReconciler(constants.Standard, params)
	require.NoError(t, err)
	assert.NotNil(t, rec)

	// Knative mode
	rec, err = factory.CreateIngressReconciler(constants.Knative, params)
	require.NoError(t, err)
	assert.NotNil(t, rec)

	// Invalid mode
	rec, err = factory.CreateIngressReconciler(constants.DeploymentModeType("invalid"), params)
	require.Error(t, err)
	assert.Nil(t, rec)
}
