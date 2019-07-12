/*
Copyright 2017 The Knative Authors

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestStatus is to validate ConditionAccessor interface works
type TestStatus struct {
	c Conditions
}

func (t *TestStatus) GetConditions() Conditions {
	return t.c
}

func (t *TestStatus) SetConditions(conditions Conditions) {
	t.c = conditions
}

// TestStatusReflection is for valid reflection case.
type TestStatusReflection struct {
	Conditions Conditions
}

// TestFatFingers is suppose to simulate not using the correct "Conditions" field.
type TestFatFingers struct {
	Conditionals Conditions
}

var (
	ignoreFields = cmpopts.IgnoreFields(Condition{}, "LastTransitionTime", "Severity")
)

func TestGetCondition(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		get    ConditionType
		expect *Condition
	}{{
		name: "simple",
		status: &TestStatus{c: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get: ConditionReady,
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}, {
		name:   "nil",
		status: nil,
		get:    ConditionReady,
		expect: nil,
	}, {
		name: "missing",
		status: &TestStatus{c: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get:    "Missing",
		expect: nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.get)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestGetConditionReflection(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		get    ConditionType
		expect *Condition
	}{{
		name: "simple",
		status: &TestStatusReflection{Conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get: ConditionReady,
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}, {
		name:   "nil",
		status: nil,
		get:    ConditionReady,
		expect: nil,
	}, {
		name: "missing",
		status: &TestStatusReflection{Conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get:    "Missing",
		expect: nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.get)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestGetConditionFatFingers(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		get    ConditionType
		expect *Condition
	}{{
		name: "simple",
		status: &TestFatFingers{Conditionals: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get:    ConditionReady,
		expect: nil,
	}, {
		name:   "nil",
		status: nil,
		get:    ConditionReady,
		expect: nil,
	}, {
		name: "missing",
		status: &TestFatFingers{Conditionals: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}}},
		get:    "Missing",
		expect: nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.get)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestSetCondition(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		set    Condition
		expect *Condition
	}{{
		name: "simple",
		status: &TestStatus{c: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}}},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}, {
		name:   "nil",
		status: nil,
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}, {
		name:   "empty",
		status: &TestStatus{},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condSet.Manage(tc.status).SetCondition(tc.set)
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.set.Type)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestSetConditionReflection(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		set    Condition
		expect *Condition
	}{{
		name: "simple",
		status: &TestStatusReflection{Conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}}},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}, {
		name:   "nil",
		status: nil,
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}, {
		name:   "empty",
		status: &TestStatusReflection{},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condSet.Manage(tc.status).SetCondition(tc.set)
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.set.Type)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestSetConditionFatFingers(t *testing.T) {
	condSet := NewLivingConditionSet()
	cases := []struct {
		name   string
		status interface{}
		set    Condition
		expect *Condition
	}{{
		name: "simple",
		status: &TestFatFingers{Conditionals: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}}},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}, {
		name:   "nil",
		status: nil,
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}, {
		name:   "empty",
		status: &TestFatFingers{},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}, {
		name: "invalid accessor struct, should be nil",
		status: &TestFatFingers{
			Conditionals: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		set: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		expect: nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condSet.Manage(tc.status).SetCondition(tc.set)
			e, a := tc.expect, condSet.Manage(tc.status).GetCondition(tc.set.Type)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestIsHappy(t *testing.T) {
	cases := []struct {
		name    string
		status  interface{}
		condSet ConditionSet
		isHappy bool
	}{{
		name: "empty accessor should not be ready",
		status: &TestStatus{
			c: Conditions(nil),
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Different condition type should not be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "False condition accessor should not be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Unknown condition accessor should not be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionUnknown,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Missing condition accessor should not be ready",
		status: &TestStatus{
			c: Conditions{{
				Type: ConditionReady,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "True condition accessor should be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with ready accessor should be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with ready accessor false should not be ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Multiple conditions with mixed ready accessor, some don't matter, ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionFalse,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with mixed ready accessor, some don't matter, not ready",
		status: &TestStatus{
			c: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isHappy, tc.condSet.Manage(tc.status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestIsHappyReflection(t *testing.T) {
	cases := []struct {
		name    string
		status  interface{}
		condSet ConditionSet
		isHappy bool
	}{{
		name: "empty accessor should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions(nil),
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Different condition type should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "False condition accessor should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Unknown condition accessor should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionUnknown,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Missing condition accessor should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type: ConditionReady,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "True condition accessor should be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with ready accessor should be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with ready accessor false should not be ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "Multiple conditions with mixed ready accessor, some don't matter, ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionFalse,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionTrue,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: true,
	}, {
		name: "Multiple conditions with mixed ready accessor, some don't matter, not ready",
		status: &TestStatusReflection{
			Conditions: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}, {
		name: "should not be happy",
		status: &TestFatFingers{
			Conditionals: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isHappy, tc.condSet.Manage(tc.status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestFatFingersHappy(t *testing.T) {
	cases := []struct {
		name     string
		accessor *TestFatFingers
		condSet  ConditionSet
		set      *Condition
		isHappy  bool
	}{{
		name: "should not be happy",
		accessor: &TestFatFingers{
			Conditionals: Conditions{{
				Type:   "Foo",
				Status: corev1.ConditionTrue,
			}, {
				Type:   "Bar",
				Status: corev1.ConditionTrue,
			}, {
				Type:   ConditionReady,
				Status: corev1.ConditionFalse,
			}},
		},
		set: &Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		condSet: NewLivingConditionSet(),
		isHappy: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != nil {
				tc.condSet.Manage(tc.accessor).SetCondition(*tc.set)
			}
			if e, a := tc.isHappy, tc.condSet.Manage(tc.accessor).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestUpdateLastTransitionTime(t *testing.T) {
	condSet := NewLivingConditionSet()

	cases := []struct {
		name       string
		conditions Conditions
		condition  Condition
		update     bool
	}{{
		name: "LastTransitionTime should be set",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}},

		condition: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		update: true,
	}, {
		name: "LastTransitionTime should update",
		conditions: Conditions{{
			Type:               ConditionReady,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: apis.VolatileTime{metav1.NewTime(time.Unix(1337, 0))},
		}},
		condition: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		},
		update: true,
	}, {
		name: "if LastTransitionTime is the only chance, don't do it",
		conditions: Conditions{{
			Type:               ConditionReady,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: apis.VolatileTime{metav1.NewTime(time.Unix(1337, 0))},
		}},

		condition: Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		},
		update: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			conds := &TestStatusReflection{Conditions: tc.conditions}

			was := condSet.Manage(conds).GetCondition(tc.condition.Type)
			condSet.Manage(conds).SetCondition(tc.condition)
			now := condSet.Manage(conds).GetCondition(tc.condition.Type)

			if e, a := tc.condition.Status, now.Status; e != a {
				t.Errorf("%q expected: %v to match %v", tc.name, e, a)
			}

			if tc.update {
				if e, a := was.LastTransitionTime, now.LastTransitionTime; e == a {
					t.Errorf("%q expected: %v to not match %v", tc.name, e, a)
				}
			} else {
				if e, a := was.LastTransitionTime, now.LastTransitionTime; e != a {
					t.Errorf("%q expected: %v to match %v", tc.name, e, a)
				}
			}
		})
	}
}

func TestResourceConditions(t *testing.T) {
	condSet := NewLivingConditionSet()

	status := &TestStatusReflection{}

	foo := Condition{
		Type:   "Foo",
		Status: "True",
	}
	bar := Condition{
		Type:   "Bar",
		Status: "True",
	}

	// Add a new condition.
	condSet.Manage(status).SetCondition(foo)

	if got, want := len(status.Conditions), 1; got != want {
		t.Fatalf("Unexpected Condition length; got %d, want %d", got, want)
	}

	// Add a second condition.
	condSet.Manage(status).SetCondition(bar)

	if got, want := len(status.Conditions), 2; got != want {
		t.Fatalf("Unexpected Condition length; got %d, want %d", got, want)
	}
}

func TestConditionSeverity(t *testing.T) {
	condSet := NewLivingConditionSet("Foo")
	status := &TestStatusReflection{}

	// Add a new condition.
	condSet.Manage(status).InitializeConditions()

	if got, want := len(status.Conditions), 2; got != want {
		t.Errorf("Unexpected number of conditions: %d, wanted %d", got, want)
	}

	condSet.Manage(status).MarkFalse("Bar", "", "")

	if got, want := len(status.Conditions), 3; got != want {
		t.Errorf("Unexpected number of conditions: %d, wanted %d", got, want)
	}

	if got, want := condSet.Manage(status).GetCondition("Ready").Severity, ConditionSeverityError; got != want {
		t.Errorf("GetCondition(%q).Severity = %v, wanted %v", "Ready", got, want)
	}

	if got, want := condSet.Manage(status).GetCondition("Foo").Severity, ConditionSeverityError; got != want {
		t.Errorf("GetCondition(%q).Severity = %v, wanted %v", "Foo", got, want)
	}

	if got, want := condSet.Manage(status).GetCondition("Bar").Severity, ConditionSeverityInfo; got != want {
		t.Errorf("GetCondition(%q).Severity = %v, wanted %v", "Bar", got, want)
	}
}

// getTypes is a small helped to strip out the used ConditionTypes from Conditions
func getTypes(conds Conditions) []ConditionType {
	var types []ConditionType
	for _, c := range conds {
		types = append(types, c.Type)
	}
	return types
}

type ConditionMarkTrueTest struct {
	name       string
	conditions Conditions
	mark       ConditionType
	happy      bool
}

func doTestMarkTrueAccessor(t *testing.T, cases []ConditionMarkTrueTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatus{c: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkTrue(tc.mark)

			if e, a := tc.happy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:   tc.mark,
				Status: corev1.ConditionTrue,
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func doTestMarkTrueReflection(t *testing.T, cases []ConditionMarkTrueTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatusReflection{Conditions: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkTrue(tc.mark)

			if e, a := tc.happy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:   tc.mark,
				Status: corev1.ConditionTrue,
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestMarkTrue(t *testing.T) {
	cases := []ConditionMarkTrueTest{{
		name:  "no deps",
		mark:  ConditionReady,
		happy: true,
	}, {
		name: "existing conditions, turns happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}},
		mark:  ConditionReady,
		happy: true,
	}, {
		name: "with deps, happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}},
		mark:  ConditionReady,
		happy: true,
	}, {
		name: "with deps, not happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:  ConditionReady,
		happy: true,
	}, {
		name: "update dep, turns happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:  "Foo",
		happy: true,
	}, {
		name: "update dep, happy was unknown, turns happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionUnknown,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:  "Foo",
		happy: true,
	}, {
		name: "update dep 1/2, still not happy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Bar",
			Status: corev1.ConditionFalse,
		}},
		mark:  "Foo",
		happy: false,
	}}
	doTestMarkTrueAccessor(t, cases)
	doTestMarkTrueReflection(t, cases)
}

type ConditionMarkFalseTest struct {
	name       string
	conditions Conditions
	mark       ConditionType
	unhappy    bool
}

func doTestMarkFalseAccessor(t *testing.T, cases []ConditionMarkFalseTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatus{c: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkFalse(tc.mark, "UnitTest", "calm down, just testing")

			if e, a := !tc.unhappy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:    tc.mark,
				Status:  corev1.ConditionFalse,
				Reason:  "UnitTest",
				Message: "calm down, just testing",
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func doTestMarkFalseReflection(t *testing.T, cases []ConditionMarkFalseTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatusReflection{Conditions: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkFalse(tc.mark, "UnitTest", "calm down, just testing")

			if e, a := !tc.unhappy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:    tc.mark,
				Status:  corev1.ConditionFalse,
				Reason:  "UnitTest",
				Message: "calm down, just testing",
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestMarkFalse(t *testing.T) {
	cases := []ConditionMarkFalseTest{{
		name:    "no deps",
		mark:    ConditionReady,
		unhappy: true,
	}, {
		name: "existing conditions, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}},
		mark:    ConditionReady,
		unhappy: true,
	}, {
		name: "with deps, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}},
		mark:    ConditionReady,
		unhappy: true,
	}, {
		name: "with deps, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:    ConditionReady,
		unhappy: true,
	}, {
		name: "update dep, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}},
		mark:    "Foo",
		unhappy: true,
	}, {
		name: "update dep, happy was unknown, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionUnknown,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:    "Foo",
		unhappy: true,
	}, {
		name: "update dep 1/2, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Bar",
			Status: corev1.ConditionTrue,
		}},
		mark:    "Foo",
		unhappy: true,
	}}
	doTestMarkFalseAccessor(t, cases)
	doTestMarkFalseReflection(t, cases)
}

type ConditionMarkUnknownTest struct {
	name       string
	conditions Conditions
	mark       ConditionType
	unhappy    bool
	happyIs    corev1.ConditionStatus
}

func doTestMarkUnknownAccessor(t *testing.T, cases []ConditionMarkUnknownTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatus{c: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkUnknown(tc.mark, "UnitTest", "idk, just testing")

			if e, a := !tc.unhappy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected IsHappy: %v got: %v", tc.name, e, a)
			}

			if e, a := tc.happyIs, condSet.Manage(status).GetCondition(ConditionReady).Status; e != a {
				t.Errorf("%q expected ConditionReady: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:    tc.mark,
				Status:  corev1.ConditionUnknown,
				Reason:  "UnitTest",
				Message: "idk, just testing",
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func doTestMarkUnknownReflection(t *testing.T, cases []ConditionMarkUnknownTest) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			condSet := NewLivingConditionSet(getTypes(tc.conditions)...)
			status := &TestStatusReflection{Conditions: tc.conditions}
			condSet.Manage(status).InitializeConditions()

			condSet.Manage(status).MarkUnknown(tc.mark, "UnitTest", "idk, just testing")

			if e, a := !tc.unhappy, condSet.Manage(status).IsHappy(); e != a {
				t.Errorf("%q expected IsHappy: %v got: %v", tc.name, e, a)
			}

			if e, a := tc.happyIs, condSet.Manage(status).GetCondition(ConditionReady).Status; e != a {
				t.Errorf("%q expected ConditionReady: %v got: %v", tc.name, e, a)
			}

			expected := &Condition{
				Type:    tc.mark,
				Status:  corev1.ConditionUnknown,
				Reason:  "UnitTest",
				Message: "idk, just testing",
			}

			e, a := expected, condSet.Manage(status).GetCondition(tc.mark)
			if diff := cmp.Diff(e, a, ignoreFields); diff != "" {
				t.Errorf("%s (-want, +got) = %v", tc.name, diff)
			}
		})
	}
}

func TestMarkUnknown(t *testing.T) {
	cases := []ConditionMarkUnknownTest{{
		name:    "no deps",
		mark:    ConditionReady,
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}, {
		name: "existing conditions, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}},
		mark:    ConditionReady,
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}, {
		name: "with deps, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}},
		mark:    ConditionReady,
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}, {
		name: "with deps that are false, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}, {
			Type:   "Bar",
			Status: corev1.ConditionFalse,
		}},
		mark:    "Foo",
		unhappy: true,
		happyIs: corev1.ConditionFalse,
	}, {
		name: "update dep, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}},
		mark:    "Foo",
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}, {
		name: "update dep, happy was unknown, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionUnknown,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionFalse,
		}},
		mark:    "Foo",
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}, {
		name: "update dep 1/2, turns unhappy",
		conditions: Conditions{{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Foo",
			Status: corev1.ConditionTrue,
		}, {
			Type:   "Bar",
			Status: corev1.ConditionTrue,
		}},
		mark:    "Foo",
		unhappy: true,
		happyIs: corev1.ConditionUnknown,
	}}
	doTestMarkUnknownAccessor(t, cases)
	doTestMarkUnknownReflection(t, cases)
}

func TestInitializeConditions(t *testing.T) {
	condSet := NewLivingConditionSet()

	cases := []struct {
		name       string
		conditions Conditions
		condition  *Condition
	}{{
		name: "initialized",
		condition: &Condition{
			Type:     ConditionReady,
			Status:   corev1.ConditionUnknown,
			Severity: ConditionSeverityError,
		},
	}, {
		name: "already initialized",
		conditions: Conditions{{
			Type:     ConditionReady,
			Status:   corev1.ConditionFalse,
			Severity: ConditionSeverityError,
		}},
		condition: &Condition{
			Type:     ConditionReady,
			Status:   corev1.ConditionFalse,
			Severity: ConditionSeverityError,
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			{
				status := &TestStatus{c: tc.conditions}
				condSet.Manage(status).InitializeConditions()
				if e, a := tc.condition, condSet.Manage(status).GetCondition(ConditionReady); !equality.Semantic.DeepEqual(e, a) {
					t.Errorf("accessor, %q expected: %v got: %v", tc.name, e, a)
				}
			}
			{
				status := &TestStatusReflection{Conditions: tc.conditions}
				condSet.Manage(status).InitializeConditions()
				if e, a := tc.condition, condSet.Manage(status).GetCondition(ConditionReady); !equality.Semantic.DeepEqual(e, a) {
					t.Errorf("reflection, %q expected: %v got: %v", tc.name, e, a)
				}
			}
		})
	}
}
