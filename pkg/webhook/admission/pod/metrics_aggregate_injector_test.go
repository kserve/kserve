/*
Copyright 2022 The KServe Authors.

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
package pod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmp"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const sklearnPrometheusPort = "8080"

func TestInjectMetricsAggregator(t *testing.T) {
	qpextAggregateMetricsPort, err := utils.StringToInt32(constants.QueueProxyAggregatePrometheusMetricsPort)
	if err != nil {
		t.Errorf("Error converting string to int32 %v", err)
	}
	scenarios := map[string]struct {
		original *corev1.Pod
		expected *corev1.Pod
	}{
		"EnableMetricAggTrue": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"EnableMetricAggTrueIdempotent": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"EnableMetricAggNotSet": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"EnableMetricAggFalse": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"setPromAnnotationTrueWithAggTrue": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation:     "true",
						constants.SetPrometheusAnnotation:     "true",
						constants.PrometheusPortAnnotationKey: constants.QueueProxyAggregatePrometheusMetricsPort,
						constants.PrometheusPathAnnotationKey: constants.DefaultPrometheusPath,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"setPromAnnotationTrueWithAggTrueIdempotent": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation:     "true",
						constants.SetPrometheusAnnotation:     "true",
						constants.PrometheusPortAnnotationKey: constants.QueueProxyAggregatePrometheusMetricsPort,
						constants.PrometheusPathAnnotationKey: constants.DefaultPrometheusPath,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation:     "true",
						constants.SetPrometheusAnnotation:     "true",
						constants.PrometheusPortAnnotationKey: constants.QueueProxyAggregatePrometheusMetricsPort,
						constants.PrometheusPathAnnotationKey: constants.DefaultPrometheusPath,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"setPromAnnotationTrueWithAggFalse": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
						constants.SetPrometheusAnnotation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation:     "false",
						constants.SetPrometheusAnnotation:     "true",
						constants.PrometheusPortAnnotationKey: constants.DefaultPodPrometheusPort,
						constants.PrometheusPathAnnotationKey: constants.DefaultPrometheusPath,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"SetPromAnnotationFalse": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:  "queue-proxy",
							Ports: []corev1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"SetPromAnnotationFalseIdempotent": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sklearn",
						},
						{
							Name: "queue-proxy",
							Env: []corev1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: qpextAggregateMetricsPort, Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
	}

	cfgMap := corev1.ConfigMap{Data: map[string]string{"enableMetricAggregation": "false", "enablePrometheusScraping": "false"}}
	ma := newMetricsAggregator(&cfgMap)

	for name, scenario := range scenarios {
		ma.InjectMetricsAggregator(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
