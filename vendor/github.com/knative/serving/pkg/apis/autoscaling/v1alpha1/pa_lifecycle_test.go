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

	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/apis/duck"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	apitest "github.com/knative/pkg/apis/testing"
	"github.com/knative/serving/pkg/apis/autoscaling"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodAutoscalerDuckTypes(t *testing.T) {
	tests := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&PodAutoscaler{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(PodAutoscaler, %T) = %v", test.t, err)
			}
		})
	}
}

func TestGeneration(t *testing.T) {
	r := PodAutoscaler{}
	if a := r.GetGeneration(); a != 0 {
		t.Errorf("empty pa generation should be 0 was: %d", a)
	}

	r.SetGeneration(5)
	if e, a := int64(5), r.GetGeneration(); e != a {
		t.Errorf("getgeneration mismatch expected: %d got: %d", e, a)
	}

}

func TestCanScaleToZero(t *testing.T) {
	cases := []struct {
		name   string
		status PodAutoscalerStatus
		result bool
		grace  time.Duration
	}{{
		name:   "empty status",
		status: PodAutoscalerStatus{},
		result: false,
		grace:  10 * time.Second,
	}, {
		name: "active condition",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		result: false,
		grace:  10 * time.Second,
	}, {
		name: "inactive condition (no LTT)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionFalse,
					// No LTT = beginning of time, so for sure we can.
				}},
			},
		},
		result: true,
		grace:  10 * time.Second,
	}, {
		name: "inactive condition (LTT longer than grace period ago)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionFalse,
					LastTransitionTime: apis.VolatileTime{
						Inner: metav1.NewTime(time.Now().Add(-30 * time.Second)),
					},
					// LTT = 30 seconds ago.
				}},
			},
		},
		result: true,
		grace:  10 * time.Second,
	}, {
		name: "inactive condition (LTT less than grace period ago)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionFalse,
					LastTransitionTime: apis.VolatileTime{
						Inner: metav1.NewTime(time.Now().Add(-10 * time.Second)),
					},
					// LTT = 10 seconds ago.
				}},
			},
		},
		result: false,
		grace:  30 * time.Second,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.result, tc.status.CanScaleToZero(tc.grace); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestCanMarkInactive(t *testing.T) {
	cases := []struct {
		name   string
		status PodAutoscalerStatus
		result bool
		idle   time.Duration
	}{{
		name:   "empty status",
		status: PodAutoscalerStatus{},
		result: false,
		idle:   10 * time.Second,
	}, {
		name: "inactive condition",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		result: false,
		idle:   10 * time.Second,
	}, {
		name: "active condition (no LTT)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
					// No LTT = beginning of time, so for sure we can.
				}},
			},
		},
		result: true,
		idle:   10 * time.Second,
	}, {
		name: "active condition (LTT longer than idle period ago)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
					LastTransitionTime: apis.VolatileTime{
						Inner: metav1.NewTime(time.Now().Add(-30 * time.Second)),
					},
					// LTT = 30 seconds ago.
				}},
			},
		},
		result: true,
		idle:   10 * time.Second,
	}, {
		name: "active condition (LTT less than idle period ago)",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
					LastTransitionTime: apis.VolatileTime{
						Inner: metav1.NewTime(time.Now().Add(-10 * time.Second)),
					},
					// LTT = 10 seconds ago.
				}},
			},
		},
		result: false,
		idle:   30 * time.Second,
	}}

	for _, tc := range cases {
		if e, a := tc.result, tc.status.CanMarkInactive(tc.idle); e != a {
			t.Errorf("%q expected: %v got: %v", tc.name, e, a)
		}
	}
}

func TestIsActivating(t *testing.T) {
	cases := []struct {
		name         string
		status       PodAutoscalerStatus
		isActivating bool
	}{{
		name:         "empty status",
		status:       PodAutoscalerStatus{},
		isActivating: false,
	}, {
		name: "active=unknown",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isActivating: true,
	}, {
		name: "active=true",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isActivating: false,
	}, {
		name: "active=false",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isActivating: false,
	}}

	for _, tc := range cases {
		if e, a := tc.isActivating, tc.status.IsActivating(); e != a {
			t.Errorf("%q expected: %v got: %v", tc.name, e, a)
		}
	}
}

func TestIsReady(t *testing.T) {
	cases := []struct {
		name    string
		status  PodAutoscalerStatus
		isReady bool
	}{{
		name:    "empty status should not be ready",
		status:  PodAutoscalerStatus{},
		isReady: false,
	}, {
		name: "Different condition type should not be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type: PodAutoscalerConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status should be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}, {
					Type:   PodAutoscalerConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		status: PodAutoscalerStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   PodAutoscalerConditionActive,
					Status: corev1.ConditionTrue,
				}, {
					Type:   PodAutoscalerConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		if e, a := tc.isReady, tc.status.IsReady(); e != a {
			t.Errorf("%q expected: %v got: %v", tc.name, e, a)
		}
	}
}

func TestTargetAnnotation(t *testing.T) {
	cases := []struct {
		name       string
		pa         *PodAutoscaler
		wantTarget int32
		wantOk     bool
	}{{
		name:       "not present",
		pa:         pa(map[string]string{}),
		wantTarget: 0,
		wantOk:     false,
	}, {
		name: "present",
		pa: pa(map[string]string{
			autoscaling.TargetAnnotationKey: "1",
		}),
		wantTarget: 1,
		wantOk:     true,
	}, {
		name: "invalid zero",
		pa: pa(map[string]string{
			autoscaling.TargetAnnotationKey: "0",
		}),
		wantTarget: 0,
		wantOk:     false,
	}, {
		name: "invalid format",
		pa: pa(map[string]string{
			autoscaling.TargetAnnotationKey: "sandwich",
		}),
		wantTarget: 0,
		wantOk:     false,
	}, {
		name: "invalid negative",
		pa: pa(map[string]string{
			autoscaling.TargetAnnotationKey: "-1",
		}),
		wantTarget: 0,
		wantOk:     false,
	}, {
		name: "invalid overflow int32",
		pa: pa(map[string]string{
			autoscaling.TargetAnnotationKey: "100000000000000000000",
		}),
		wantTarget: 0,
		wantOk:     false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTarget, gotOk := tc.pa.Target()
			if gotTarget != tc.wantTarget {
				t.Errorf("got target: %v wanted: %v", gotTarget, tc.wantTarget)
			}
			if gotOk != tc.wantOk {
				t.Errorf("got ok: %v wanted %v", gotOk, tc.wantOk)
			}
		})
	}
}

func TestScaleBounds(t *testing.T) {
	cases := []struct {
		name    string
		pa      *PodAutoscaler
		wantMin int32
		wantMax int32
	}{{
		name: "present",
		pa: pa(map[string]string{
			autoscaling.MinScaleAnnotationKey: "1",
			autoscaling.MaxScaleAnnotationKey: "100",
		}),
		wantMin: 1,
		wantMax: 100,
	}, {
		name:    "absent",
		pa:      pa(map[string]string{}),
		wantMin: 0,
		wantMax: 0,
	}, {
		name: "only min",
		pa: pa(map[string]string{
			autoscaling.MinScaleAnnotationKey: "1",
		}),
		wantMin: 1,
		wantMax: 0,
	}, {
		name: "only max",
		pa: pa(map[string]string{
			autoscaling.MaxScaleAnnotationKey: "1",
		}),
		wantMin: 0,
		wantMax: 1,
	}, {
		name: "malformed",
		pa: pa(map[string]string{
			autoscaling.MinScaleAnnotationKey: "ham",
			autoscaling.MaxScaleAnnotationKey: "sandwich",
		}),
		wantMin: 0,
		wantMax: 0,
	}, {
		name: "too small",
		pa: pa(map[string]string{
			autoscaling.MinScaleAnnotationKey: "-1",
			autoscaling.MaxScaleAnnotationKey: "-1",
		}),
		wantMin: 0,
		wantMax: 0,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			min, max := tc.pa.ScaleBounds()
			if min != tc.wantMin {
				t.Errorf("got min: %v wanted: %v", min, tc.wantMin)
			}
			if max != tc.wantMax {
				t.Errorf("got max: %v wanted: %v", max, tc.wantMax)
			}
		})
	}
}

func TestMarkResourceNotOwned(t *testing.T) {
	pa := pa(map[string]string{})
	pa.Status.MarkResourceNotOwned("doesn't", "matter")
	active := pa.Status.GetCondition("Active")
	if active.Status != corev1.ConditionFalse {
		t.Errorf("TestMarkResourceNotOwned expected active.Status: False got: %v", active.Status)
	}
	if active.Reason != "NotOwned" {
		t.Errorf("TestMarkResourceNotOwned expected active.Reason: NotOwned got: %v", active.Reason)
	}
}

func TestMarkResourceFailedCreation(t *testing.T) {
	pa := &PodAutoscalerStatus{}
	pa.MarkResourceFailedCreation("doesn't", "matter")
	apitest.CheckConditionFailed(pa.duck(), PodAutoscalerConditionActive, t)

	active := pa.GetCondition("Active")
	if active.Status != corev1.ConditionFalse {
		t.Errorf("TestMarkResourceFailedCreation expected active.Status: False got: %v", active.Status)
	}
	if active.Reason != "FailedCreate" {
		t.Errorf("TestMarkResourceFailedCreation expected active.Reason: FailedCreate got: %v", active.Reason)
	}
}

func TestClass(t *testing.T) {
	cases := []struct {
		name string
		pa   *PodAutoscaler
		want string
	}{{
		name: "kpa class",
		pa: pa(map[string]string{
			autoscaling.ClassAnnotationKey: autoscaling.KPA,
		}),
		want: "kpa.autoscaling.knative.dev",
	}, {
		name: "hpa class",
		pa: pa(map[string]string{
			autoscaling.ClassAnnotationKey: autoscaling.HPA,
		}),
		want: "hpa.autoscaling.knative.dev",
	}, {
		name: "default class",
		pa:   pa(map[string]string{}),
		want: "kpa.autoscaling.knative.dev",
	}, {
		name: "custom class",
		pa: pa(map[string]string{
			autoscaling.ClassAnnotationKey: "yolo.sandwich.com",
		}),
		want: "yolo.sandwich.com",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pa.Class()
			if got != tc.want {
				t.Errorf("got class: %q wanted: %q", got, tc.want)
			}
		})
	}
}

func pa(annotations map[string]string) *PodAutoscaler {
	p := &PodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "test-namespace",
			Name:        "test-name",
			Annotations: annotations,
		},
		Spec: PodAutoscalerSpec{
			ContainerConcurrency: 0,
		},
		Status: PodAutoscalerStatus{},
	}
	return p
}

func TestTypicalFlow(t *testing.T) {
	r := &PodAutoscalerStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionOngoing(r.duck(), PodAutoscalerConditionReady, t)

	// When we see traffic, mark ourselves active.
	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionReady, t)

	// Check idempotency.
	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionReady, t)

	// When we stop seeing traffic, mark outselves inactive.
	r.MarkInactive("TheReason", "the message")
	apitest.CheckConditionFailed(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionFailed(r.duck(), PodAutoscalerConditionReady, t)

	// When traffic hits the activator and we scale up the deployment we mark
	// ourselves as activating.
	r.MarkActivating("Activating", "Red team, GO!")
	apitest.CheckConditionOngoing(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionOngoing(r.duck(), PodAutoscalerConditionReady, t)

	// When the activator successfully forwards traffic to the deployment,
	// we mark ourselves as active once more.
	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionActive, t)
	apitest.CheckConditionSucceeded(r.duck(), PodAutoscalerConditionReady, t)
}
