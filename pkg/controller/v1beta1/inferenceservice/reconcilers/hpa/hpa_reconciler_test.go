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
	"context"
	"testing"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			got := createHPA(tt.args.objectMeta, tt.args.componentExt)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Test %q unexpected hpa (-want +got): %v", tt.name, diff)
			}
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

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.To(int32(3))},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "none"}},
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

func TestCheckHPAExist(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)

	type testCase struct {
		name           string
		existingHPA    *autoscalingv2.HorizontalPodAutoscaler
		desiredHPA     *autoscalingv2.HorizontalPodAutoscaler
		expectedResult constants.CheckResultType
	}

	tests := []testCase{
		{
			name:        "hpa does not exist and should be created",
			existingHPA: nil,
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
			},
			expectedResult: constants.CheckResultCreate,
		},
		{
			name:        "hpa does not exist and should be skipped due to external autoscaler",
			existingHPA: nil,
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassExternal)},
				},
			},
			expectedResult: constants.CheckResultSkipped,
		},
		{
			name: "hpa exists and is equivalent",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			expectedResult: constants.CheckResultExisted,
		},
		{
			name: "hpa exists but should be updated",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 5,
				},
			},
			expectedResult: constants.CheckResultUpdate,
		},
		{
			name: "hpa exists but should be deleted due to external autoscaler, owner is kserve",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "serving.kserve.io/v1beta1",
							Kind:       "InferenceService",
							Name:       "my-inferenceservice",
							UID:        "12345",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassExternal)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			expectedResult: constants.CheckResultDelete,
		},
		{
			name: "hpa with different autoscaler class",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassHPA)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassKeda)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			expectedResult: constants.CheckResultUpdate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			// Create the existing HPA if it's provided
			if tc.existingHPA != nil {
				err := client.Create(context.TODO(), tc.existingHPA)
				require.NoError(t, err)
			}

			reconciler := &HPAReconciler{
				client: client,
				scheme: scheme,
				HPA:    tc.desiredHPA,
			}

			result, hpa, err := reconciler.checkHPAExist(context.TODO(), client)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, result)

			if tc.existingHPA != nil {
				assert.NotNil(t, hpa)
			} else if result == constants.CheckResultCreate || result == constants.CheckResultSkipped {
				assert.Nil(t, hpa)
			}
		})
	}
}

func TestGetHPAMetrics(t *testing.T) {
	cpuResource := v1beta1.MetricCPU
	memoryResource := v1beta1.MetricMemory

	tests := []struct {
		name          string
		componentExt  *v1beta1.ComponentExtensionSpec
		expectedSpecs []autoscalingv2.MetricSpec
	}{
		{
			name: "default cpu metric with scaleMetric",
			componentExt: &v1beta1.ComponentExtensionSpec{
				ScaleMetric: &cpuResource,
				ScaleTarget: ptr.To(int32(70)),
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(70)),
						},
					},
				},
			},
		},
		{
			name: "default cpu metric without scaleTarget",
			componentExt: &v1beta1.ComponentExtensionSpec{
				ScaleMetric: &cpuResource,
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(80)),
						},
					},
				},
			},
		},
		{
			name: "memory metric with scaleTarget",
			componentExt: &v1beta1.ComponentExtensionSpec{
				ScaleMetric: &memoryResource,
				ScaleTarget: ptr.To(int32(60)),
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(60)),
						},
					},
				},
			},
		},
		{
			name: "memory metric without scaleTarget",
			componentExt: &v1beta1.ComponentExtensionSpec{
				ScaleMetric: &memoryResource,
			},
			expectedSpecs: nil,
		},
		{
			name: "autoscaling with CPU utilization metric",
			componentExt: &v1beta1.ComponentExtensionSpec{
				AutoScaling: &v1beta1.AutoScalingSpec{
					Metrics: []v1beta1.MetricsSpec{
						{
							Type: v1beta1.ResourceMetricSourceType,
							Resource: &v1beta1.ResourceMetricSource{
								Name: v1beta1.ResourceMetricCPU,
								Target: v1beta1.MetricTarget{
									Type:               v1beta1.UtilizationMetricType,
									AverageUtilization: ptr.To(int32(75)),
								},
							},
						},
					},
				},
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(75)),
						},
					},
				},
			},
		},
		{
			name: "autoscaling with CPU metric without utilization",
			componentExt: &v1beta1.ComponentExtensionSpec{
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
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(80)),
						},
					},
				},
			},
		},
		{
			name: "autoscaling with memory utilization metric",
			componentExt: &v1beta1.ComponentExtensionSpec{
				AutoScaling: &v1beta1.AutoScalingSpec{
					Metrics: []v1beta1.MetricsSpec{
						{
							Type: v1beta1.ResourceMetricSourceType,
							Resource: &v1beta1.ResourceMetricSource{
								Name: v1beta1.ResourceMetricMemory,
								Target: v1beta1.MetricTarget{
									Type:               v1beta1.UtilizationMetricType,
									AverageUtilization: ptr.To(int32(65)),
								},
							},
						},
					},
				},
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(65)),
						},
					},
				},
			},
		},
		{
			name: "autoscaling with memory average value metric",
			componentExt: &v1beta1.ComponentExtensionSpec{
				AutoScaling: &v1beta1.AutoScalingSpec{
					Metrics: []v1beta1.MetricsSpec{
						{
							Type: v1beta1.ResourceMetricSourceType,
							Resource: &v1beta1.ResourceMetricSource{
								Name: v1beta1.ResourceMetricMemory,
								Target: v1beta1.MetricTarget{
									Type:         v1beta1.AverageValueMetricType,
									AverageValue: ptr.To(resource.MustParse("500Mi")),
								},
							},
						},
					},
				},
			},
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: ptr.To(resource.MustParse("500Mi")),
						},
					},
				},
			},
		},
		{
			name: "multiple metrics in autoscaling",
			componentExt: &v1beta1.ComponentExtensionSpec{
				AutoScaling: &v1beta1.AutoScalingSpec{
					Metrics: []v1beta1.MetricsSpec{
						{
							Type: v1beta1.ResourceMetricSourceType,
							Resource: &v1beta1.ResourceMetricSource{
								Name: v1beta1.ResourceMetricCPU,
								Target: v1beta1.MetricTarget{
									Type:               v1beta1.UtilizationMetricType,
									AverageUtilization: ptr.To(int32(75)),
								},
							},
						},
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
			expectedSpecs: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(75)),
						},
					},
				},
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: ptr.To(resource.MustParse("1Gi")),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := getHPAMetrics(tt.componentExt)
			if diff := cmp.Diff(tt.expectedSpecs, metrics); diff != "" {
				t.Errorf("Test %q unexpected metrics (-want +got): %v", tt.name, diff)
			}
		})
	}
}

// trackingClient is a wrapper around client.Client that tracks actions performed
type trackingClient struct {
	client.Client
	actualAction *string
}

// Create tracks create actions
func (c *trackingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	*c.actualAction = "create"
	return c.Client.Create(ctx, obj, opts...)
}

// Update tracks update actions
func (c *trackingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	*c.actualAction = "update"
	return c.Client.Update(ctx, obj, opts...)
}

// Delete tracks delete actions
func (c *trackingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	*c.actualAction = "delete"
	return c.Client.Delete(ctx, obj, opts...)
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)

	type testCase struct {
		name           string
		existingHPA    *autoscalingv2.HorizontalPodAutoscaler
		desiredHPA     *autoscalingv2.HorizontalPodAutoscaler
		componentExt   *v1beta1.ComponentExtensionSpec
		expectedResult error
		expectedAction string
	}

	tests := []testCase{
		{
			name:        "create new hpa",
			existingHPA: nil,
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 3,
			},
			expectedResult: nil,
			expectedAction: "create",
		},
		{
			name: "update existing hpa",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 5,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(2)),
				MaxReplicas: 5,
			},
			expectedResult: nil,
			expectedAction: "update",
		},
		{
			name: "delete hpa with external autoscaler and kserve owner",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "serving.kserve.io/v1beta1",
							Kind:       "InferenceService",
							Name:       "my-inferenceservice",
							UID:        "12345",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassExternal)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 3,
			},
			expectedResult: nil,
			expectedAction: "delete",
		},
		{
			name: "do not delete hpa with external autoscaler and not owned by kserve",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "some-deployment",
							UID:        "54321",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassExternal)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 3,
			},
			expectedResult: nil,
			expectedAction: "update",
		},
		{
			name: "skip when hpa exists and is equivalent",
			existingHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 3,
			},
			expectedResult: nil,
			expectedAction: "skip",
		},
		{
			name:        "skip creating hpa with external autoscaler",
			existingHPA: nil,
			desiredHPA: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-hpa",
					Namespace:   "default",
					Annotations: map[string]string{constants.AutoscalerClass: string(constants.AutoscalerClassExternal)},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 3,
			},
			expectedResult: nil,
			expectedAction: "skip",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)

			// Create the existing HPA if it's provided
			if tc.existingHPA != nil {
				clientBuilder = clientBuilder.WithObjects(tc.existingHPA)
			}

			client := clientBuilder.Build()

			// Create a tracking client wrapper
			var actualAction string
			trackingClient := &trackingClient{
				Client:       client,
				actualAction: &actualAction,
			}

			reconciler := &HPAReconciler{
				client:       trackingClient,
				scheme:       scheme,
				HPA:          tc.desiredHPA,
				componentExt: tc.componentExt,
			}

			result := reconciler.Reconcile(context.TODO())

			assert.Equal(t, tc.expectedResult, result)
			if tc.expectedAction == "skip" {
				assert.Equal(t, "", actualAction, "Expected no action but got %s", actualAction)
			} else if tc.expectedAction != "" {
				assert.Equal(t, tc.expectedAction, actualAction, "Expected action %s but got %s", tc.expectedAction, actualAction)
			}
			// Verify the HPA state after reconciliation
			resultHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			err := client.Get(context.TODO(), types.NamespacedName{
				Namespace: tc.desiredHPA.Namespace,
				Name:      tc.desiredHPA.Name,
			}, resultHPA)

			if tc.expectedAction == "create" || tc.expectedAction == "update" {
				require.NoError(t, err, "Expected HPA to exist after %s action", tc.expectedAction)
				assert.Equal(t, tc.desiredHPA.Spec.MinReplicas, resultHPA.Spec.MinReplicas)
				assert.Equal(t, tc.desiredHPA.Spec.MaxReplicas, resultHPA.Spec.MaxReplicas)
			} else if tc.expectedAction == "delete" {
				assert.True(t, apierr.IsNotFound(err), "Expected HPA to be deleted")
			}
		})
	}
}

func TestSetControllerReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	owner := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-service",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "default",
		},
	}

	reconciler := &HPAReconciler{
		HPA: hpa,
	}

	err := reconciler.SetControllerReferences(owner, scheme)
	require.NoError(t, err)

	// Verify owner reference is set
	assert.Len(t, hpa.OwnerReferences, 1)
	assert.Equal(t, "owner-service", hpa.OwnerReferences[0].Name)
	assert.Equal(t, "owner-uid", string(hpa.OwnerReferences[0].UID))
	assert.True(t, *hpa.OwnerReferences[0].Controller)
}
