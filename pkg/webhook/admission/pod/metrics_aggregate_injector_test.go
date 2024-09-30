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
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmp"
	"strconv"

	"testing"
)

const sklearnPrometheusPort = "8080"

func TestInjectMetricsAggregator(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"EnableMetricAggTrue": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name: "queue-proxy",
							Env: []v1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: strconv.Itoa(int(constants.QueueProxyAggregatePrometheusMetricsPort))},
							},
							Ports: []v1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: int32(constants.QueueProxyAggregatePrometheusMetricsPort), Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"EnableMetricAggNotSet": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"EnableMetricAggFalse": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"setPromAnnotationTrueWithAggTrue": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation:     "true",
						constants.SetPrometheusAnnotation:     "true",
						constants.PrometheusPortAnnotationKey: strconv.Itoa(int(constants.QueueProxyAggregatePrometheusMetricsPort)),
						constants.PrometheusPathAnnotationKey: constants.DefaultPrometheusPath,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name: "queue-proxy",
							Env: []v1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: strconv.Itoa(int(constants.QueueProxyAggregatePrometheusMetricsPort))},
							},
							Ports: []v1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: int32(constants.QueueProxyAggregatePrometheusMetricsPort), Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
		"setPromAnnotationTrueWithAggFalse": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "false",
						constants.SetPrometheusAnnotation: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
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
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
		},
		"SetPromAnnotationFalse": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  "queue-proxy",
							Ports: []v1.ContainerPort{{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.EnableMetricAggregation: "true",
						constants.SetPrometheusAnnotation: "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name: "queue-proxy",
							Env: []v1.EnvVar{
								{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: sklearnPrometheusPort},
								{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: constants.DefaultPrometheusPath},
								{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: strconv.Itoa(int(constants.QueueProxyAggregatePrometheusMetricsPort))},
							},
							Ports: []v1.ContainerPort{
								{Name: "http-usermetric", ContainerPort: 9091, Protocol: "TCP"},
								{Name: constants.AggregateMetricsPortName, ContainerPort: int32(constants.QueueProxyAggregatePrometheusMetricsPort), Protocol: "TCP"},
							},
						},
					},
				},
			},
		},
	}

	cfgMap := v1.ConfigMap{Data: map[string]string{"enableMetricAggregation": "false", "enablePrometheusScraping": "false"}}
	ma, err := newMetricsAggregator(&cfgMap)
	if err != nil {
		t.Errorf("Error creating the metrics aggregator %v", err)
	}

	for name, scenario := range scenarios {
		ma.InjectMetricsAggregator(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
