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
package hpa

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestCreateHPA(t *testing.T) {
	type args struct {
		objectMeta   metav1.ObjectMeta
		componentExt *v1beta1.ComponentExtensionSpec
	}

	cpuResource := v1beta1.MetricCPU
	memoryResource := v1beta1.MetricMemory

	defaultMinReplicas := int32(1)
	defaultUtilization := int32(80)
	igMinReplicas := int32(2)
	igUtilization := int32(30)
	predictorMinReplicas := int32(5)
	predictorUtilization := int32(50)

	tests := []struct {
		name     string
		args     args
		expected *autoscalingv2.HorizontalPodAutoscaler
		err      error
	}{
		{
			name: "inference graph default hpa",
			args: args{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						"label":                            "label-value",
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
				},
				componentExt: &v1beta1.ComponentExtensionSpec{},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						"label":                            "label-value",
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "basic-ig",
					},
					MinReplicas: &defaultMinReplicas,
					MaxReplicas: 1,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("cpu"),
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: &defaultUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
		{
			name: "inference graph specified hpa",
			args: args{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						"label":                            "label-value",
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
				},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 5,
					ScaleTarget: ptr.To(int32(30)),
					ScaleMetric: &cpuResource,
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						"label":                            "label-value",
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "basic-ig",
					},
					MinReplicas: &igMinReplicas,
					MaxReplicas: 5,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("cpu"),
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &igUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
		},
		{
			name: "predictor hpa cpu metric",
			args: args{
				objectMeta: metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: nil,
					MaxReplicas: 0,
					AutoScaling: &v1beta1.AutoScalingSpec{
						Metrics: []v1beta1.MetricsSpec{
							{
								Type: v1beta1.ResourceMetricSourceType,
								Resource: &v1beta1.ResourceMetricSource{
									Name: v1beta1.ResourceMetricCPU,
								},
							},
						},
					},
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					MinReplicas: &defaultMinReplicas,
					MaxReplicas: 1,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("cpu"),
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: ptr.To(int32(80)),
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
		{
			name: "predictor hpa memory utilization metric",
			args: args{
				objectMeta: metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: nil,
					MaxReplicas: 0,
					AutoScaling: &v1beta1.AutoScalingSpec{
						Metrics: []v1beta1.MetricsSpec{
							{
								Type: v1beta1.ResourceMetricSourceType,
								Resource: &v1beta1.ResourceMetricSource{
									Name: v1beta1.ResourceMetricMemory,
									Target: v1beta1.MetricTarget{
										Type:               v1beta1.UtilizationMetricType,
										AverageUtilization: ptr.To(int32(80)),
									},
								},
							},
						},
					},
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					MinReplicas: &defaultMinReplicas,
					MaxReplicas: 1,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("memory"),
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: ptr.To(int32(80)),
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
		{
			name: "predictor hpa memory metric",
			args: args{
				objectMeta: metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: nil,
					MaxReplicas: 0,
					AutoScaling: &v1beta1.AutoScalingSpec{
						Metrics: []v1beta1.MetricsSpec{
							{
								Type: v1beta1.ResourceMetricSourceType,
								Resource: &v1beta1.ResourceMetricSource{
									Name: v1beta1.ResourceMetricMemory,
									Target: v1beta1.MetricTarget{
										Type:         v1beta1.AverageValueMetricType,
										AverageValue: ptr.To(resource.MustParse("1Gi")),
									},
								},
							},
						},
					},
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					MinReplicas: &defaultMinReplicas,
					MaxReplicas: 1,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("memory"),
								Target: autoscalingv2.MetricTarget{
									Type:         autoscalingv2.AverageValueMetricType,
									AverageValue: ptr.To(resource.MustParse("1Gi")),
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
		{
			name: "predictor hpa with ScaleMetric",
			args: args{
				objectMeta: metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: ptr.To(int32(5)),
					MaxReplicas: 10,
					ScaleTarget: ptr.To(int32(50)),
					ScaleMetric: &cpuResource,
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					MinReplicas: &predictorMinReplicas,
					MaxReplicas: 10,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceName("cpu"),
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: &predictorUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
		{
			name: "invalid memory scale target for hpa",
			args: args{
				objectMeta: metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{
					MinReplicas: ptr.To(int32(0)),
					MaxReplicas: -10,
					ScaleTarget: nil,
					ScaleMetric: &memoryResource,
				},
			},
			expected: &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", APIVersion: "apps/v1"},
					MinReplicas:    ptr.To(int32(1)),
					MaxReplicas:    int32(1),
					Behavior:       &autoscalingv2.HorizontalPodAutoscalerBehavior{},
				},
			},
			err: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createHPA(tt.args.objectMeta, tt.args.componentExt)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Test %q unexpected hpa (-want +got): %v", tt.name, diff)
			}
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestSemanticHPAEquals(t *testing.T) {
	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{},
		}))

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(4))},
		}))

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "external"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		}))

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "external"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"unrelated": "true"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"unrelated": "false"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		}))
}
