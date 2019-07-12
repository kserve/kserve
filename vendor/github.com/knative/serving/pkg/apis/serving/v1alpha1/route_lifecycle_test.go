/*
Copyright 2018 The Knative Authors

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

	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	apitesting "github.com/knative/pkg/apis/testing"
	netv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRouteDuckTypes(t *testing.T) {
	tests := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}, {
		name: "legacy targetable",
		t:    &duckv1alpha1.LegacyTargetable{},
	}, {
		name: "addressable",
		t:    &duckv1alpha1.Addressable{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&Route{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(Route, %T) = %v", test.t, err)
			}
		})
	}
}

func TestRouteIsReady(t *testing.T) {
	cases := []struct {
		name    string
		status  RouteStatus
		isReady bool
	}{{
		name:    "empty status should not be ready",
		status:  RouteStatus{},
		isReady: false,
	}, {
		name: "Different condition type should not be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionAllTrafficAssigned,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{

				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type: RouteConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status should be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionAllTrafficAssigned,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RouteConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		status: RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RouteConditionAllTrafficAssigned,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RouteConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isReady, tc.status.IsReady(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestTypicalRouteFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkTrafficAssigned()
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionReady, t)
}

func TestTrafficNotAssignedFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkMissingTrafficTarget("Revision", "does-not-exist")
	apitesting.CheckConditionFailed(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionReady, t)
}

func TestTargetConfigurationNotYetReadyFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkConfigurationNotReady("i-have-no-ready-revision")
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)
}

func TestUnknownErrorWhenConfiguringTraffic(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkUnknownTrafficError("unknown-error")
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)
}

func TestTargetConfigurationFailedToBeReadyFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkConfigurationFailed("permanently-failed")
	apitesting.CheckConditionFailed(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionReady, t)
}

func TestTargetRevisionNotYetReadyFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkRevisionNotReady("not-yet-ready")
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)
}

func TestTargetRevisionFailedToBeReadyFlow(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkRevisionFailed("cannot-find-image")
	apitesting.CheckConditionFailed(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionReady, t)
}

func TestClusterIngressFailureRecovery(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionUnknown,
			}},
		},
	})
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	// Empty IngressStatus keeps things as-is.
	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{})
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkTrafficAssigned()
	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionReady, t)

	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
	})
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionReady, t)

	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionSucceeded(r.duck(), RouteConditionReady, t)
}

func TestRouteNotOwnedStuff(t *testing.T) {
	r := &RouteStatus{}
	r.InitializeConditions()
	r.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   netv1alpha1.ClusterIngressConditionReady,
				Status: corev1.ConditionUnknown,
			}},
		},
	})

	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionReady, t)

	r.MarkServiceNotOwned("evan")
	apitesting.CheckConditionOngoing(r.duck(), RouteConditionAllTrafficAssigned, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionIngressReady, t)
	apitesting.CheckConditionFailed(r.duck(), RouteConditionReady, t)
}

func TestRouteGetGroupVersionKind(t *testing.T) {
	r := &Route{}
	want := schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1alpha1",
		Kind:    "Route",
	}
	if got := r.GetGroupVersionKind(); got != want {
		t.Errorf("got: %v, want: %v", got, want)
	}
}
