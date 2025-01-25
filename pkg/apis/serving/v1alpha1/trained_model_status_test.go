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

package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

func TestTrainedModelStatus_IsReady(t *testing.T) {
	cases := []struct {
		name          string
		ServiceStatus TrainedModelStatus
		isReady       bool
	}{{
		name:          "empty status should not be ready",
		ServiceStatus: TrainedModelStatus{},
		isReady:       false,
	}, {
		name: "Different condition type should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   "Foo",
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type: knservingv1.ConfigurationConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "ConfigurationsReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "Foo",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ConfigurationConditionReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isReady, tc.ServiceStatus.IsReady(); e != a {
				t.Errorf("%q expected: %v got: %v conditions: %v", tc.name, e, a, tc.ServiceStatus.Conditions)
			}
		})
	}
}

func TestTrainedModelStatus_GetCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name          string
		ServiceStatus TrainedModelStatus
		Condition     apis.ConditionType
		matcher       *apis.Condition
	}{{
		name:          "Empty status should return nil",
		ServiceStatus: TrainedModelStatus{},
		Condition:     knservingv1.ServiceConditionReady,
		matcher:       nil,
	}, {
		name: "Get custom condition",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   "Foo",
					Status: corev1.ConditionFalse,
				}},
			},
		},
		Condition: "Foo",
		matcher: &apis.Condition{
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		},
	}, {
		name: "Get Ready condition",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		Condition: knservingv1.ServiceConditionReady,
		matcher: &apis.Condition{
			Type:   knservingv1.ServiceConditionReady,
			Status: corev1.ConditionUnknown,
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.ServiceStatus.GetCondition(tc.Condition)
			g.Expect(res).Should(gomega.Equal(tc.matcher))
		})
	}
}

func TestTrainedModelStatus_IsConditionReady(t *testing.T) {
	cases := []struct {
		name          string
		ServiceStatus TrainedModelStatus
		Condition     apis.ConditionType
		isReady       bool
	}{{
		name:          "empty status should not be ready",
		ServiceStatus: TrainedModelStatus{},
		Condition:     knservingv1.ServiceConditionReady,
		isReady:       false,
	}, {
		name: "Different condition type should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   "Foo",
					Status: corev1.ConditionTrue,
				}},
			},
		},
		Condition: "Bar",
		isReady:   false,
	}, {
		name: "False condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		Condition: knservingv1.ServiceConditionReady,
		isReady:   false,
	}, {
		name: "Unknown condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		Condition: knservingv1.ServiceConditionReady,
		isReady:   false,
	}, {
		name: "Missing condition status should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type: knservingv1.ConfigurationConditionReady,
				}},
			},
		},
		Condition: knservingv1.ServiceConditionReady,
		isReady:   false,
	}, {
		name: "True condition status should be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "ConfigurationsReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		Condition: knservingv1.ServiceConditionReady,
		isReady:   true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		ServiceStatus: TrainedModelStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "Foo",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ConfigurationConditionReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
		Condition: knservingv1.ConfigurationConditionReady,
		isReady:   false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isReady, tc.ServiceStatus.IsConditionReady(tc.Condition); e != a {
				t.Errorf("%q expected: %v got: %v conditions: %v", tc.name, e, a, tc.ServiceStatus.Conditions)
			}
		})
	}
}

func TestTrainedModelStatus_SetCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name          string
		serviceStatus TrainedModelStatus
		condition     *apis.Condition
		conditionType apis.ConditionType
		expected      *apis.Condition
	}{
		{
			name:          "set condition on empty status",
			serviceStatus: TrainedModelStatus{},
			condition: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			},
			conditionType: "Foo",
			expected: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			},
		}, {
			name: "modify existing condition",
			serviceStatus: TrainedModelStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{{
						Type:   "Foo",
						Status: corev1.ConditionTrue,
					}},
				},
			},
			condition: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionFalse,
			},
			conditionType: "Foo",
			expected: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionFalse,
			},
		}, {
			name: "set condition unknown",
			serviceStatus: TrainedModelStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{{
						Type:   "Foo",
						Status: corev1.ConditionFalse,
					}},
				},
			},
			condition: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionUnknown,
				Reason: "For testing purpose",
			},
			conditionType: "Foo",
			expected: &apis.Condition{
				Type:   "Foo",
				Status: corev1.ConditionUnknown,
				Reason: "For testing purpose",
			},
		}, {
			name: "condition is nil",
			serviceStatus: TrainedModelStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{{
						Type:   knservingv1.ServiceConditionReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			condition:     nil,
			conditionType: "Foo",
			expected:      nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.serviceStatus.SetCondition(tc.conditionType, tc.condition)
			res := tc.serviceStatus.GetCondition(tc.conditionType)
			g.Expect(cmp.Equal(res, tc.expected, cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime", "Severity"))).To(gomega.BeTrue())
		})
	}
}
