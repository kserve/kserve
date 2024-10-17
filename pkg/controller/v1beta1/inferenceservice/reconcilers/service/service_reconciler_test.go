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
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
					constants.DeploymentMode:  string(constants.RawDeployment),
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
					constants.RawDeploymentAppLabel:       "isvc.default-predictor",
					constants.InferenceServicePodLabelKey: "default-predictor",
					constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
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
						constants.AutoscalerClass: string(constants.DefaultAutoscalerClass),
						constants.DeploymentMode:  string(constants.RawDeployment),
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
						constants.RawDeploymentAppLabel:       "isvc.default-predictor",
						constants.KServiceComponentLabel:      "predictor",
						constants.InferenceServicePodLabelKey: "default-predictor",
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
					Name:      "default-head",
					Namespace: "default-predictor-namespace",
					Labels: map[string]string{
						constants.RawDeploymentAppLabel:       "isvc.default-predictor",
						constants.KServiceComponentLabel:      "predictor",
						constants.InferenceServicePodLabelKey: "default-predictor",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						constants.RawDeploymentAppLabel: "isvc.default-predictor",
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
			got := createService(tt.args.componentMeta, tt.args.componentExt, tt.args.podSpec, tt.args.multiNodeEnabled)
			for i, service := range got {
				if diff := cmp.Diff(tt.expected[i], service); diff != "" {
					t.Errorf("Test %q unexpected service (-want +got): %v", tt.name, diff)
				}

			}
		})
	}
}
