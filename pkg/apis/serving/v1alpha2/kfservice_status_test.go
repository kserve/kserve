/*
Copyright 2019 kubeflow.org.

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

package v1alpha2

import (
	"k8s.io/api/core/v1"
	"knative.dev/pkg/apis/duck"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	"testing"
)

func TestKFServiceDuckType(t *testing.T) {
	cases := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&KFService{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(KFService, %T) = %v", test.t, err)
			}
		})
	}
}

func TestKFServiceIsReady(t *testing.T) {
	cases := []struct {
		name                 string
		defaultServiceStatus v1alpha1.ServiceStatus
		canaryServiceStatus  v1alpha1.ServiceStatus
		routeStatus          v1alpha1.RouteStatus
		isReady              bool
	}{{
		name:                 "empty status should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{},
		routeStatus:          v1alpha1.RouteStatus{},
		isReady:              false,
	}, {
		name: "Different condition type should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type: v1alpha1.ConfigurationConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		routeStatus: v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.RouteConditionReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Default configuration, route conditions with ready status should be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}, {
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionTrue,
				},
				},
			},
		},
		routeStatus: v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.RouteConditionReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Default/canary configuration, route conditions with ready status should be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionTrue,
				},
				},
			},
		},
		canaryServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionTrue,
				},
				},
			},
		},
		routeStatus: v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.RouteConditionReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		defaultServiceStatus: v1alpha1.ServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}, {
					Type:   v1alpha1.ConfigurationConditionReady,
					Status: v1.ConditionFalse,
				}},
			},
		},
		routeStatus: v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha1.RouteConditionReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := KFServiceStatus{}
			status.PropagateDefaultConfigurationStatus(&tc.defaultServiceStatus)
			status.PropagateCanaryConfigurationStatus(&tc.canaryServiceStatus)
			status.PropagateRouteStatus(&tc.routeStatus)
			if e, a := tc.isReady, status.IsReady(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}
