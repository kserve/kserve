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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func TestCreateHPA(t *testing.T) {
	type args struct {
		objectMeta   metav1.ObjectMeta
		componentExt *v1beta1.ComponentExtensionSpec
	}

	cpuResource := v1beta1.MetricCPU
	memoryResource := v1beta1.MetricMemory

	testInput := map[string]args{
		"igdefaulthpa": {
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
		"igspecifiedhpa": {
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
				MinReplicas: utils.ToPointer(int32(2)),
				MaxReplicas: 5,
				ScaleTarget: utils.ToPointer(30),
				ScaleMetric: &cpuResource,
			},
		},
		"predictordefaulthpa": {
			objectMeta: metav1.ObjectMeta{},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: nil,
				MaxReplicas: 0,
				ScaleTarget: nil,
				ScaleMetric: &memoryResource,
			},
		},
		"predictorspecifiedhpa": {
			objectMeta: metav1.ObjectMeta{},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: utils.ToPointer(int32(5)),
				MaxReplicas: 10,
				ScaleTarget: utils.ToPointer(50),
				ScaleMetric: &cpuResource,
			},
		},
		"invalidinputhpa": {
			objectMeta: metav1.ObjectMeta{},
			componentExt: &v1beta1.ComponentExtensionSpec{
				MinReplicas: utils.ToPointer(int32(0)),
				MaxReplicas: -10,
				ScaleTarget: nil,
				ScaleMetric: &memoryResource,
			},
		},
	}

	defaultminreplicas := int32(1)
	defaultutilization := int32(80)
	igminreplicas := int32(2)
	igutilization := int32(30)
	predictorminreplicas := int32(5)
	predictorutilization := int32(50)

	expectedHPASpecs := map[string]*autoscalingv2.HorizontalPodAutoscaler{
		"igdefaulthpa": {
			ObjectMeta: testInput["igdefaulthpa"].objectMeta,
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       testInput["igdefaulthpa"].objectMeta.Name,
				},
				MinReplicas: &defaultminreplicas,
				MaxReplicas: 1,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceName("cpu"),
							Target: autoscalingv2.MetricTarget{
								Type:               "Utilization",
								AverageUtilization: &defaultutilization,
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
			},
		},
		"igspecifiedhpa": {
			ObjectMeta: testInput["igspecifiedhpa"].objectMeta,
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       testInput["igspecifiedhpa"].objectMeta.Name,
				},
				MinReplicas: &igminreplicas,
				MaxReplicas: 5,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceName("cpu"),
							Target: autoscalingv2.MetricTarget{
								Type:               "Utilization",
								AverageUtilization: &igutilization,
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
			},
		},
		"predictordefaulthpa": {
			ObjectMeta: metav1.ObjectMeta{},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				MinReplicas: &defaultminreplicas,
				MaxReplicas: 1,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceName("memory"),
							Target: autoscalingv2.MetricTarget{
								Type:               "Utilization",
								AverageUtilization: &defaultutilization,
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
			},
		},
		"predictorspecifiedhpa": {
			ObjectMeta: metav1.ObjectMeta{},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				MinReplicas: &predictorminreplicas,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceName("cpu"),
							Target: autoscalingv2.MetricTarget{
								Type:               "Utilization",
								AverageUtilization: &predictorutilization,
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
			},
		},
	}

	tests := []struct {
		name     string
		args     args
		expected *autoscalingv2.HorizontalPodAutoscaler
		err      error
	}{
		{
			name: "inference graph default hpa",
			args: args{
				objectMeta:   testInput["igdefaulthpa"].objectMeta,
				componentExt: testInput["igdefaulthpa"].componentExt,
			},
			expected: expectedHPASpecs["igdefaulthpa"],
			err:      nil,
		},
		{
			name: "inference graph specified hpa",
			args: args{
				objectMeta:   testInput["igspecifiedhpa"].objectMeta,
				componentExt: testInput["igspecifiedhpa"].componentExt,
			},
			expected: expectedHPASpecs["igspecifiedhpa"],
		},
		{
			name: "predictor default hpa",
			args: args{
				objectMeta:   testInput["predictordefaulthpa"].objectMeta,
				componentExt: testInput["predictordefaulthpa"].componentExt,
			},
			expected: expectedHPASpecs["predictordefaulthpa"],
			err:      nil,
		},
		{
			name: "predictor specified hpa",
			args: args{
				objectMeta:   testInput["predictorspecifiedhpa"].objectMeta,
				componentExt: testInput["predictorspecifiedhpa"].componentExt,
			},
			expected: expectedHPASpecs["predictorspecifiedhpa"],
			err:      nil,
		},
		{
			name: "invalid input for hpa",
			args: args{
				objectMeta:   testInput["invalidinputhpa"].objectMeta,
				componentExt: testInput["invalidinputhpa"].componentExt,
			},
			expected: expectedHPASpecs["predictordefaulthpa"],
			err:      nil,
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
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(4)},
		}))

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "external"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		}))

	assert.False(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "external"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{constants.AutoscalerClass: "hpa"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		}))

	assert.True(t, semanticHPAEquals(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"unrelated": "true"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"unrelated": "false"}},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MinReplicas: ptr.Int32(3)},
		}))
}
