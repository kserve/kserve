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

	corev1 "k8s.io/api/core/v1"
)

func TestNewLivingConditionSet(t *testing.T) {
	cases := []struct {
		name  string
		types []ConditionType
		count int // count includes the happy condition type.
	}{{
		name:  "empty",
		types: []ConditionType(nil),
		count: 1,
	}, {
		name:  "one",
		types: []ConditionType{"Foo"},
		count: 2,
	}, {
		name:  "duplicate in happy",
		types: []ConditionType{ConditionReady},
		count: 1,
	}, {
		name:  "duplicate in dependents",
		types: []ConditionType{"Foo", "Bar", "Foo"},
		count: 3,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := NewLivingConditionSet(tc.types...)
			if e, a := tc.count, 1+len(set.dependents); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestNewBatchConditionSet(t *testing.T) {
	cases := []struct {
		name  string
		types []ConditionType
		count int // count includes the happy condition type.
	}{{
		name:  "empty",
		types: []ConditionType(nil),
		count: 1,
	}, {
		name:  "one",
		types: []ConditionType{"Foo"},
		count: 2,
	}, {
		name:  "duplicate in happy",
		types: []ConditionType{ConditionSucceeded},
		count: 1,
	}, {
		name:  "duplicate in dependents",
		types: []ConditionType{"Foo", "Bar", "Foo"},
		count: 3,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := NewBatchConditionSet(tc.types...)
			if e, a := tc.count, 1+len(set.dependents); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestNonTerminalCondition(t *testing.T) {
	set := NewLivingConditionSet("Foo")
	status := &Status{}

	manager := set.Manage(status)
	manager.InitializeConditions()

	if got, want := len(status.Conditions), 2; got != want {
		t.Errorf("InitializeConditions() = %v, wanted %v", got, want)
	}

	// Setting the other "terminal" condition makes Ready true.
	manager.MarkTrue("Foo")
	if got, want := manager.GetCondition("Ready").Status, corev1.ConditionTrue; got != want {
		t.Errorf("MarkTrue(Foo) = %v, wanted %v", got, want)
	}

	// Setting a "non-terminal" condition, doesn't change Ready.
	manager.MarkUnknown("Bar", "", "")
	if got, want := manager.GetCondition("Ready").Status, corev1.ConditionTrue; got != want {
		t.Errorf("MarkUnknown(Foo) = %v, wanted %v", got, want)
	}

	// Setting a "non-terminal" condition, doesn't change Ready.
	manager.MarkFalse("Bar", "", "")
	if got, want := manager.GetCondition("Ready").Status, corev1.ConditionTrue; got != want {
		t.Errorf("MarkFalse(Foo) = %v, wanted %v", got, want)
	}
}
