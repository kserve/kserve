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

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
)

func TestBuildImplementsConditions(t *testing.T) {
	if err := duck.VerifyType(&Build{}, &duckv1alpha1.Conditions{}); err != nil {
		t.Errorf("Expect Build to implement duck verify type: err %#v", err)
	}
}

func TestBuildConditions(t *testing.T) {
	b := &Build{}
	foo := &duckv1alpha1.Condition{
		Type:   "Foo",
		Status: "True",
	}
	bar := &duckv1alpha1.Condition{
		Type:   "Bar",
		Status: "True",
	}

	var ignoreVolatileTime = cmp.Comparer(func(_, _ apis.VolatileTime) bool {
		return true
	})

	// Add a new condition.
	b.Status.SetCondition(foo)

	want := duckv1alpha1.Conditions([]duckv1alpha1.Condition{*foo})
	if cmp.Diff(b.Status.GetConditions(), want, ignoreVolatileTime) != "" {
		t.Errorf("Unexpected build condition type; want %v got %v", want, b.Status.GetConditions())
	}

	fooStatus := b.Status.GetCondition(foo.Type)
	if cmp.Diff(fooStatus, foo, ignoreVolatileTime) != "" {
		t.Errorf("Unexpected build condition type; want %v got %v", fooStatus, foo)
	}

	// Add a second condition.
	b.Status.SetCondition(bar)

	want = duckv1alpha1.Conditions([]duckv1alpha1.Condition{*bar, *foo})

	if d := cmp.Diff(b.Status.GetConditions(), want, ignoreVolatileTime); d != "" {
		t.Fatalf("Unexpected build condition type; want %v got %v; diff %s", want, b.Status.GetConditions(), d)
	}
}

func TestBuildGroupVersionKind(t *testing.T) {
	b := Build{}

	expectedKind := "Build"
	if b.GetGroupVersionKind().Kind != expectedKind {
		t.Errorf("GetGroupVersionKind mismatch; expected: %v got: %v", expectedKind, b.GetGroupVersionKind().Kind)
	}
}
