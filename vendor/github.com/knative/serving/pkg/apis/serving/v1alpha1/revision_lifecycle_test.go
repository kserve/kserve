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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	apitest "github.com/knative/pkg/apis/testing"
	net "github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRevisionDuckTypes(t *testing.T) {
	tests := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&Revision{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(Revision, %T) = %v", test.t, err)
			}
		})
	}
}

func TestIsActivationRequired(t *testing.T) {
	cases := []struct {
		name                 string
		status               RevisionStatus
		isActivationRequired bool
	}{{
		name:                 "empty status should not be inactive",
		status:               RevisionStatus{},
		isActivationRequired: false,
	}, {
		name: "Ready status should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isActivationRequired: false,
	}, {
		name: "Inactive status should be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionActive,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isActivationRequired: true,
	}, {
		name: "Updating status should be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
					Reason: "Updating",
				}, {
					Type:   RevisionConditionActive,
					Status: corev1.ConditionUnknown,
					Reason: "Updating",
				}},
			},
		},
		isActivationRequired: true,
	}, {
		name: "NotReady status without reason should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isActivationRequired: false,
	}, {
		name: "Ready/Unknown status without reason should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isActivationRequired: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isActivationRequired, tc.status.IsActivationRequired(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	cases := []struct {
		name    string
		status  RevisionStatus
		isReady bool
	}{{
		name:    "empty status should not be ready",
		status:  RevisionStatus{},
		isReady: false,
	}, {
		name: "Different condition type should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionBuildSucceeded,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type: RevisionConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status should be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionBuildSucceeded,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionBuildSucceeded,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RevisionConditionReady,
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

func TestGetSetCondition(t *testing.T) {
	rs := &RevisionStatus{}
	if a := rs.GetCondition(RevisionConditionReady); a != nil {
		t.Errorf("empty RevisionStatus returned %v when expected nil", a)
	}

	rc := &apis.Condition{
		Type:     RevisionConditionBuildSucceeded,
		Status:   corev1.ConditionTrue,
		Severity: apis.ConditionSeverityError,
	}

	rs.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
		}},
	})

	if diff := cmp.Diff(rc, rs.GetCondition(RevisionConditionBuildSucceeded), cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime")); diff != "" {
		t.Errorf("GetCondition refs diff (-want +got): %v", diff)
	}
	if a := rs.GetCondition(RevisionConditionReady); a != nil {
		t.Errorf("GetCondition expected nil got: %v", a)
	}
}

func TestTypicalFlowWithBuild(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	// Empty BuildStatus keeps things as-is.
	r.PropagateBuildStatus(duckv1alpha1.Status{})
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	r.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionUnknown,
		}},
	})
	const want = "Building"
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	if got := r.GetCondition(RevisionConditionBuildSucceeded); got == nil || got.Reason != want {
		t.Errorf("PropagateBuildStatus(Unknown) = %v, wanted %v", got, want)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Reason != want {
		t.Errorf("PropagateBuildStatus(Unknown) = %v, wanted %v", got, want)
	}

	r.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
		}},
	})
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	// All of these conditions should get this status.
	const want2 = "TheReason"
	r.MarkDeploying(want2)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Reason != want2 {
		t.Errorf("MarkDeploying = %v, want2ed %v", got, want2)
	}
	if got := r.GetCondition(RevisionConditionContainerHealthy); got == nil || got.Reason != want2 {
		t.Errorf("MarkDeploying = %v, want2ed %v", got, want2)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Reason != want2 {
		t.Errorf("MarkDeploying = %v, want2ed %v", got, want2)
	}

	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionActive, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	if r.IsReady() {
		t.Error("IsReady() = true, want false")
	}

	r.MarkContainerHealthy()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionActive, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	if r.IsReady() {
		t.Error("IsReady() = true, want false")
	}

	r.MarkResourcesAvailable()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionActive, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	if !r.IsReady() {
		t.Error("IsReady() = false, want true")
	}
}

func TestTypicalFlowWithBuildFailure(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	r.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionUnknown,
		}},
	})
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const wantReason, wantMessage = "this is the reason", "and this the message"
	r.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:    duckv1alpha1.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  wantReason,
			Message: wantMessage,
		}},
	})
	apitest.CheckConditionFailed(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	if got := r.GetCondition(RevisionConditionBuildSucceeded); got == nil {
		t.Errorf("MarkBuildFailed = nil, wanted %v", wantReason)
	} else if got.Reason != wantReason {
		t.Errorf("MarkBuildFailed = %v, wanted %v", got.Reason, wantReason)
	} else if got.Message != wantMessage {
		t.Errorf("MarkBuildFailed = %v, wanted %v", got.Reason, wantMessage)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil {
		t.Errorf("MarkBuildFailed = nil, wanted %v", wantReason)
	} else if got.Reason != wantReason {
		t.Errorf("MarkBuildFailed = %v, wanted %v", got.Reason, wantReason)
	} else if got.Message != wantMessage {
		t.Errorf("MarkBuildFailed = %v, wanted %v", got.Reason, wantMessage)
	}
}

func TestTypicalFlowWithServiceTimeout(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	r.MarkServiceTimeout()
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
}

func TestTypicalFlowWithProgressDeadlineExceeded(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "the error message"
	r.MarkProgressDeadlineExceeded(want)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Message != want {
		t.Errorf("MarkProgressDeadlineExceeded = %v, want %v", got, want)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Message != want {
		t.Errorf("MarkProgressDeadlineExceeded = %v, want %v", got, want)
	}
}

func TestTypicalFlowWithContainerMissing(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "something about the container being not found"
	r.MarkContainerMissing(want)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionFailed(r.duck(), RouteConditionReady, t)
	if got := r.GetCondition(RevisionConditionContainerHealthy); got == nil || got.Message != want {
		t.Errorf("MarkContainerMissing = %v, want %v", got, want)
	} else if got.Reason != "ContainerMissing" {
		t.Errorf("MarkContainerMissing = %v, want %v", got, "ContainerMissing")
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Message != want {
		t.Errorf("MarkContainerMissing = %v, want %v", got, want)
	} else if got.Reason != "ContainerMissing" {
		t.Errorf("MarkContainerMissing = %v, want %v", got, "ContainerMissing")
	}
}

func TestTypicalFlowWithSuspendResume(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	// Enter a Ready state.
	r.MarkActive()
	r.MarkContainerHealthy()
	r.MarkResourcesAvailable()
	r.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: []duckv1alpha1.Condition{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
			Reason: "NoBuild",
		}},
	})
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From a Ready state, make the revision inactive to simulate scale to zero.
	const want = "Deactivated"
	r.MarkInactive(want, "Reserve")
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionActive, t)
	if got := r.GetCondition(RevisionConditionActive); got == nil || got.Reason != want {
		t.Errorf("MarkInactive = %v, want %v", got, want)
	}
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From an Inactive state, start to activate the revision.
	const want2 = "Activating"
	r.MarkActivating(want2, "blah blah blah")
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionActive, t)
	if got := r.GetCondition(RevisionConditionActive); got == nil || got.Reason != want2 {
		t.Errorf("MarkInactive = %v, want %v", got, want2)
	}
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From the activating state, simulate the transition back to readiness.
	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)
}

func TestRevisionNotOwnedStuff(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionBuildSucceeded, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "NotOwned"
	r.MarkResourceNotOwned("Resource", "mark")
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Reason != want {
		t.Errorf("MarkResourceNotOwned = %v, want %v", got, want)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Reason != want {
		t.Errorf("MarkResourceNotOwned = %v, want %v", got, want)
	}
}

func TestRevisionGetGroupVersionKind(t *testing.T) {
	r := &Revision{}
	want := schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1alpha1",
		Kind:    "Revision",
	}
	if got := r.GetGroupVersionKind(); got != want {
		t.Errorf("got: %v, want: %v", got, want)
	}
}

func TestRevisionBuildRefFromName(t *testing.T) {
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
		Spec: RevisionSpec{
			DeprecatedBuildName: "bar-build",
		},
	}
	got := *r.BuildRef()
	want := corev1.ObjectReference{
		APIVersion: "build.knative.dev/v1alpha1",
		Kind:       "Build",
		Namespace:  "foo-space",
		Name:       "bar-build",
	}
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionBuildRef(t *testing.T) {
	buildRef := corev1.ObjectReference{
		APIVersion: "testing.build.knative.dev/v1alpha1",
		Kind:       "Build",
		Namespace:  "foo-space",
		Name:       "foo-build",
	}
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
		Spec: RevisionSpec{
			DeprecatedBuildName: "bar",
			BuildRef:            &buildRef,
		},
	}
	got := *r.BuildRef()
	want := buildRef
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionBuildRefNil(t *testing.T) {
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
	}
	got := r.BuildRef()

	var want *corev1.ObjectReference
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionGetProtocol(t *testing.T) {
	containerWithPortName := func(name string) corev1.Container {
		return corev1.Container{Ports: []corev1.ContainerPort{{Name: name}}}
	}

	tests := []struct {
		name      string
		container corev1.Container
		protocol  net.ProtocolType
	}{{
		name:      "undefined",
		container: corev1.Container{},
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "http1",
		container: containerWithPortName("http1"),
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "h2c",
		container: containerWithPortName("h2c"),
		protocol:  net.ProtocolH2C,
	}, {
		name:      "unknown",
		container: containerWithPortName("whatever"),
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "empty",
		container: containerWithPortName(""),
		protocol:  net.ProtocolHTTP1,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Revision{
				Spec: RevisionSpec{
					Container: &tt.container,
				},
			}

			got := r.GetProtocol()
			want := tt.protocol

			if got != want {
				t.Errorf("got: %#v, want: %#v", got, want)
			}
		})
	}
}

func TestRevisionGetLastPinned(t *testing.T) {
	cases := []struct {
		name              string
		annotations       map[string]string
		expectTime        time.Time
		setLastPinnedTime time.Time
		expectErr         error
	}{{
		name:        "Nil annotations",
		annotations: nil,
		expectErr: LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		},
	}, {
		name:        "Empty map annotations",
		annotations: map[string]string{},
		expectErr: LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		},
	}, {
		name:              "Empty map annotations - with set time",
		annotations:       map[string]string{},
		setLastPinnedTime: time.Unix(1000, 0),
		expectTime:        time.Unix(1000, 0),
	}, {
		name:        "Invalid time",
		annotations: map[string]string{serving.RevisionLastPinnedAnnotationKey: "abcd"},
		expectErr: LastPinnedParseError{
			Type:  AnnotationParseErrorTypeInvalid,
			Value: "abcd",
		},
	}, {
		name:        "Valid time",
		annotations: map[string]string{serving.RevisionLastPinnedAnnotationKey: "10000"},
		expectTime:  time.Unix(10000, 0),
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rev := Revision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			if tc.setLastPinnedTime != (time.Time{}) {
				rev.SetLastPinned(tc.setLastPinnedTime)
			}

			pt, err := rev.GetLastPinned()
			failErr := func() {
				t.Fatalf("Expected error %v got %v", tc.expectErr, err)
			}

			if tc.expectErr == nil {
				if err != nil {
					failErr()
				}
			} else {
				if tc.expectErr.Error() != err.Error() {
					failErr()
				}
			}

			if tc.expectTime != pt {
				t.Fatalf("Expected pin time %v got %v", tc.expectTime, pt)
			}
		})
	}
}
