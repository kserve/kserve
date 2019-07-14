/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kmp

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCompareKcmpDefault(t *testing.T) {
	a := resource.MustParse("50m")
	b := resource.MustParse("100m")

	want := cmp.Diff(a, b, defaultOpts...)

	if got, err := SafeDiff(a, b); err != nil {
		t.Errorf("unexpected SafeDiff err: %v", err)
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SafeDiff (-want, +got): %v", diff)
	}

	if got, err := SafeEqual(a, b); err != nil {
		t.Fatalf("unexpected SafeEqual err: %v", err)
	} else if diff := cmp.Diff(false, got); diff != "" {
		t.Errorf("SafeEqual(-want, +got): %v", diff)
	}
}

func TestRecovery(t *testing.T) {
	type foo struct {
		bar string
	}

	a := foo{"a"}
	b := foo{"b"}

	if _, err := SafeDiff(a, b); err == nil {
		t.Error("expected err, got nil")
	}

	if _, err := SafeEqual(a, b); err == nil {
		t.Error("expected err, got nil")
	}

	if _, err := ShortDiff(a, b); err == nil {
		t.Error("expected err, got nil")
	}

	if _, err := CompareSetFields(a, b); err == nil {
		t.Error("expected err, got nil")
	}
}

func TestFieldDiff(t *testing.T) {
	type foo struct {
		Bar string `json:"stringField"`
		Baz int    `json:"intField"`
	}

	a := foo{
		Bar: "a",
		Baz: 1,
	}
	b := foo{
		Bar: "b",
		Baz: 1,
	}

	want := []string{"stringField"}

	got, err := CompareSetFields(a, b)

	if err != nil {
		t.Errorf("unexpected FieldDiff err: %v", err)
	} else if !cmp.Equal(got, want) {
		t.Errorf("FieldDiff() = %v, want: %s", got, want)
	}

}

func TestImmutableDiff(t *testing.T) {
	a := resource.MustParse("50m")
	b := resource.MustParse("100m")

	want := `{resource.Quantity}:
	-: resource.Quantity: "{i:{value:50 scale:-3} d:{Dec:<nil>} s:50m Format:DecimalSI}"
	+: resource.Quantity: "{i:{value:100 scale:-3} d:{Dec:<nil>} s:100m Format:DecimalSI}"
`

	if got, err := ShortDiff(a, b); err != nil {
		t.Errorf("unexpected ShortDiff err: %v", err)
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SafeDiff (-want, +got): %v", diff)
	}
}
