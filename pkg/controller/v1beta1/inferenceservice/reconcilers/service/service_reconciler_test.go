/*
Copyright 2024 The KServe Authors.

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
package service

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var emptyServiceConfig = &v1beta1.ServiceConfig{}

func TestCreateDefaultDeployment(t *testing.T) {
	type args struct {
		componentMeta    metav1.ObjectMeta
		componentExt     *v1beta1.ComponentExtensionSpec
		podSpec          *corev1.PodSpec
		multiNodeEnabled bool
	}

	testInput := map[string]args{
		"default-service": {
			componentMeta: metav1.ObjectMeta{
				Name:      "default-predictor",
				Namespace: "default-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.DeploymentMode:  string(constants.Standard),
					constants.AutoscalerClass: string(constants.DefaultAutoscalerClass),
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{},
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "default-predictor-example-volume",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: "default-predictor-example-image",
						Env: []corev1.EnvVar{
							{Name: "default-predictor-example-env", Value: "example-env"},
						},
					},
				},
			},
			multiNodeEnabled: false,
		},

		"multiNode-service": {
			componentMeta: metav1.ObjectMeta{
				Name:      "default-predictor",
				Namespace: "default-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.RawDeploymentAppLabel:                 "isvc.default-predictor",
					constants.InferenceServicePodLabelKey:           "default-predictor",
					constants.KServiceComponentLabel:                string(v1beta1.PredictorComponent),
					constants.InferenceServiceGenerationPodLabelKey: "1",
				},
			},

			componentExt: &v1beta1.ComponentExtensionSpec{},
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "default-predictor-example-volume",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: "default-predictor-example-image",
						Env: []corev1.EnvVar{
							{Name: "default-predictor-example-env", Value: "example-env"},
						},
					},
				},
			},
			multiNodeEnabled: true,
		},
	}

	expectedServices := map[string][]*corev1.Service{
		"default-service": {
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-predictor",
					Namespace: "default-predictor-namespace",
					Labels: map[string]string{
						constants.RawDeploymentAppLabel: "isvc.default-predictor",
						constants.AutoscalerClass:       string(constants.DefaultAutoscalerClass),
						constants.DeploymentMode:        string(constants.Standard),
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "default-predictor",
							Protocol:   corev1.ProtocolTCP,
							Port:       80,
							TargetPort: intstr.IntOrString{IntVal: 8080},
						},
					},
					Selector: map[string]string{
						constants.RawDeploymentAppLabel: "isvc.default-predictor",
					},
				},
			},
			nil,
		},
		"multiNode-service": {
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-predictor",
					Namespace: "default-predictor-namespace",
					Labels: map[string]string{
						constants.RawDeploymentAppLabel:                 "isvc.default-predictor",
						constants.KServiceComponentLabel:                "predictor",
						constants.InferenceServicePodLabelKey:           "default-predictor",
						constants.InferenceServiceGenerationPodLabelKey: "1",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "default-predictor",
							Protocol:   corev1.ProtocolTCP,
							Port:       80,
							TargetPort: intstr.IntOrString{IntVal: 8080},
						},
					},
					Selector: map[string]string{
						"app": "isvc.default-predictor",
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-head-1",
					Namespace: "default-predictor-namespace",
					Labels: map[string]string{
						constants.RawDeploymentAppLabel:                 "isvc.default-predictor",
						constants.KServiceComponentLabel:                "predictor",
						constants.InferenceServicePodLabelKey:           "default-predictor",
						constants.InferenceServiceGenerationPodLabelKey: "1",
						constants.MultiNodeRoleLabelKey:                 constants.MultiNodeHead,
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						constants.RawDeploymentAppLabel:                 "isvc.default-predictor",
						constants.InferenceServiceGenerationPodLabelKey: "1",
					},
					ClusterIP:                "None",
					PublishNotReadyAddresses: true,
				},
			},
		},
	}

	tests := []struct {
		name     string
		args     args
		expected []*corev1.Service
	}{
		{
			name: "default service",
			args: args{
				componentMeta:    testInput["default-service"].componentMeta,
				componentExt:     testInput["default-service"].componentExt,
				podSpec:          testInput["default-service"].podSpec,
				multiNodeEnabled: testInput["default-service"].multiNodeEnabled,
			},
			expected: expectedServices["default-service"],
		},
		{
			name: "multiNode service",
			args: args{
				componentMeta:    testInput["multiNode-service"].componentMeta,
				componentExt:     testInput["multiNode-service"].componentExt,
				podSpec:          testInput["multiNode-service"].podSpec,
				multiNodeEnabled: testInput["multiNode-service"].multiNodeEnabled,
			},
			expected: expectedServices["multiNode-service"],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createService(tt.args.componentMeta, tt.args.componentExt, tt.args.podSpec, tt.args.multiNodeEnabled, emptyServiceConfig)
			for i, service := range got {
				if diff := cmp.Diff(tt.expected[i], service); diff != "" {
					t.Errorf("Test %q unexpected service (-want +got): %v", tt.name, diff)
				}
			}
		})
	}
}

func TestCreateServiceRawServiceConfigEmpty(t *testing.T) {
	// nothing expected
	runTestServiceCreate(emptyServiceConfig, "", t)
}

func TestCreateServiceRawServiceAndConfigNil(t *testing.T) {
	var serviceConfig *v1beta1.ServiceConfig
	// no service means empty
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawFalseAndConfigTrue(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: true,
	}
	runTestServiceCreate(serviceConfig, corev1.ClusterIPNone, t)
}

func TestCreateServiceRawTrueAndConfigFalse(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: false,
	}
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawFalseAndConfigNil(t *testing.T) {
	runTestServiceCreate(emptyServiceConfig, "", t)
}

func TestCreateServiceRawTrueAndConfigNil(t *testing.T) {
	// service is there, but no property, should be empty
	runTestServiceCreate(emptyServiceConfig, "", t)
}

func runTestServiceCreate(serviceConfig *v1beta1.ServiceConfig, expectedClusterIP string, t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{}
	podSpec := &corev1.PodSpec{}

	service := createService(componentMeta, componentExt, podSpec, false, serviceConfig)

	// The ObjectMeta should now include the app label that was added by createService
	expectedMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
		Labels: map[string]string{
			"app": "isvc.test-service",
		},
	}
	assert.Equal(t, expectedMeta, service[0].ObjectMeta, "Expected ObjectMeta to be equal")
	assert.Equal(t, map[string]string{"app": "isvc.test-service"}, service[0].Spec.Selector, "Expected Selector to be equal")
	assert.Equal(t, expectedClusterIP, service[0].Spec.ClusterIP, "Expected ClusterIP to be equal")
}

func TestAppLabelRespectedInServices(t *testing.T) {
	tests := []struct {
		name                  string
		userSpecifiedAppLabel string
		componentName         string
		expectedServiceLabel  string
		expectedSelectorLabel string
	}{
		{
			name:                  "User specifies custom app label",
			userSpecifiedAppLabel: "my-custom-app",
			componentName:         "my-model-predictor",
			expectedServiceLabel:  "my-custom-app",
			expectedSelectorLabel: "my-custom-app",
		},
		{
			name:                  "User does not specify app label - use default",
			userSpecifiedAppLabel: "",
			componentName:         "my-model-predictor",
			expectedServiceLabel:  "isvc.my-model-predictor",
			expectedSelectorLabel: "isvc.my-model-predictor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create componentMeta with optional user-specified app label
			componentMeta := metav1.ObjectMeta{
				Name:      tt.componentName,
				Namespace: "default",
				Labels:    make(map[string]string),
			}
			if tt.userSpecifiedAppLabel != "" {
				componentMeta.Labels["app"] = tt.userSpecifiedAppLabel
			}

			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  constants.InferenceServiceContainerName,
						Image: "test-image",
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: 8080,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			}

			// Call createService which should set the app label correctly
			services := createService(componentMeta, &v1beta1.ComponentExtensionSpec{}, podSpec, false, emptyServiceConfig)
			assert.NotNil(t, services)
			assert.Len(t, services, 1)

			service := services[0]

			// Verify Service metadata label
			assert.Equal(t, tt.expectedServiceLabel, service.ObjectMeta.Labels["app"],
				"Service metadata should have correct app label")

			// Verify Service selector
			assert.Equal(t, tt.expectedSelectorLabel, service.Spec.Selector["app"],
				"Service selector should match the app label")
		})
	}
}

func TestAppLabelRespectedInHeadlessServices(t *testing.T) {
	tests := []struct {
		name                  string
		userSpecifiedAppLabel string
		componentName         string
		expectedServiceLabel  string
		expectedSelectorLabel string
	}{
		{
			name:                  "User specifies custom app label for headless service",
			userSpecifiedAppLabel: "my-custom-headless-app",
			componentName:         "my-model-predictor",
			expectedServiceLabel:  "my-custom-headless-app",
			expectedSelectorLabel: "my-custom-headless-app",
		},
		{
			name:                  "User does not specify app label for headless service - use default",
			userSpecifiedAppLabel: "",
			componentName:         "my-model-predictor",
			expectedServiceLabel:  "isvc.my-model-predictor",
			expectedSelectorLabel: "isvc.my-model-predictor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create componentMeta with optional user-specified app label
			componentMeta := metav1.ObjectMeta{
				Name:      tt.componentName,
				Namespace: "default",
				Labels: map[string]string{
					constants.InferenceServiceGenerationPodLabelKey: "1",
				},
			}
			if tt.userSpecifiedAppLabel != "" {
				componentMeta.Labels["app"] = tt.userSpecifiedAppLabel
			}

			// Set the app label correctly (respect user's choice or use default)
			// This simulates what createService does before calling createHeadlessSvc
			setAppLabelOrDefault(&componentMeta, constants.GetRawServiceLabel(tt.componentName))

			// Test createHeadlessSvc directly
			headlessService := createHeadlessSvc(componentMeta)

			// Verify Headless Service metadata label
			assert.Equal(t, tt.expectedServiceLabel, headlessService.ObjectMeta.Labels["app"],
				"Headless Service metadata should have correct app label")

			// Verify Headless Service selector
			assert.Equal(t, tt.expectedSelectorLabel, headlessService.Spec.Selector["app"],
				"Headless Service selector should match the app label")

			// Verify it's actually headless
			assert.Equal(t, "None", headlessService.Spec.ClusterIP,
				"Headless Service should have ClusterIP set to None")
		})
	}
}
