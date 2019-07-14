/*
Copyright 2019 The Knative Authors

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

package v1beta1

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/ptr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTrafficTargetValidation(t *testing.T) {
	tests := []struct {
		name string
		tt   *TrafficTarget
		want *apis.FieldError
		wc   func(context.Context) context.Context
	}{{
		name: "valid with revisionName",
		tt: &TrafficTarget{
			RevisionName: "bar",
			Percent:      12,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "valid with revisionName and name (spec)",
		tt: &TrafficTarget{
			Tag:          "foo",
			RevisionName: "bar",
			Percent:      12,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "valid with revisionName and name (status)",
		tt: &TrafficTarget{
			Tag:          "foo",
			RevisionName: "bar",
			Percent:      12,
			URL:          "http://foo.bar.com",
		},
		wc:   apis.WithinStatus,
		want: nil,
	}, {
		name: "invalid with revisionName and name (status)",
		tt: &TrafficTarget{
			Tag:          "foo",
			RevisionName: "bar",
			Percent:      12,
		},
		wc:   apis.WithinStatus,
		want: apis.ErrMissingField("url"),
	}, {
		name: "invalid with bad revisionName",
		tt: &TrafficTarget{
			RevisionName: "b ar",
			Percent:      12,
		},
		wc: apis.WithinSpec,
		want: apis.ErrInvalidKeyName(
			"b ar", "revisionName", "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')"),
	}, {
		name: "valid with revisionName and latestRevision",
		tt: &TrafficTarget{
			RevisionName:   "bar",
			LatestRevision: ptr.Bool(false),
			Percent:        12,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "invalid with revisionName and latestRevision (spec)",
		tt: &TrafficTarget{
			RevisionName:   "bar",
			LatestRevision: ptr.Bool(true),
			Percent:        12,
		},
		wc:   apis.WithinSpec,
		want: apis.ErrInvalidValue(true, "latestRevision"),
	}, {
		name: "valid with revisionName and latestRevision (status)",
		tt: &TrafficTarget{
			RevisionName:   "bar",
			LatestRevision: ptr.Bool(true),
			Percent:        12,
		},
		wc:   apis.WithinStatus,
		want: nil,
	}, {
		name: "valid with configurationName",
		tt: &TrafficTarget{
			ConfigurationName: "bar",
			Percent:           37,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "valid with configurationName and name (spec)",
		tt: &TrafficTarget{
			Tag:               "foo",
			ConfigurationName: "bar",
			Percent:           37,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "invalid with bad configurationName",
		tt: &TrafficTarget{
			ConfigurationName: "b ar",
			Percent:           37,
		},
		wc: apis.WithinSpec,
		want: apis.ErrInvalidKeyName(
			"b ar", "configurationName", "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')"),
	}, {
		name: "valid with configurationName and latestRevision",
		tt: &TrafficTarget{
			ConfigurationName: "blah",
			LatestRevision:    ptr.Bool(true),
			Percent:           37,
		},
		wc:   apis.WithinSpec,
		want: nil,
	}, {
		name: "invalid with configurationName and latestRevision",
		tt: &TrafficTarget{
			ConfigurationName: "blah",
			LatestRevision:    ptr.Bool(false),
			Percent:           37,
		},
		wc:   apis.WithinSpec,
		want: apis.ErrInvalidValue(false, "latestRevision"),
	}, {
		name: "invalid with configurationName and default configurationName",
		tt: &TrafficTarget{
			ConfigurationName: "blah",
			Percent:           37,
		},
		wc:   WithDefaultConfigurationName,
		want: apis.ErrDisallowedFields("configurationName"),
	}, {
		name: "valid with only default configurationName",
		tt: &TrafficTarget{
			Percent: 37,
		},
		wc: func(ctx context.Context) context.Context {
			return WithDefaultConfigurationName(apis.WithinSpec(ctx))
		},
		want: nil,
	}, {
		name: "valid with default configurationName and latestRevision",
		tt: &TrafficTarget{
			LatestRevision: ptr.Bool(true),
			Percent:        37,
		},
		wc: func(ctx context.Context) context.Context {
			return WithDefaultConfigurationName(apis.WithinSpec(ctx))
		},
		want: nil,
	}, {
		name: "invalid with default configurationName and latestRevision",
		tt: &TrafficTarget{
			LatestRevision: ptr.Bool(false),
			Percent:        37,
		},
		wc: func(ctx context.Context) context.Context {
			return WithDefaultConfigurationName(apis.WithinSpec(ctx))
		},
		want: apis.ErrInvalidValue(false, "latestRevision"),
	}, {
		name: "invalid without revisionName in status",
		tt: &TrafficTarget{
			ConfigurationName: "blah",
			Percent:           37,
		},
		wc:   apis.WithinStatus,
		want: apis.ErrMissingField("revisionName"),
	}, {
		name: "valid with revisionName and default configurationName",
		tt: &TrafficTarget{
			RevisionName: "bar",
			Percent:      12,
		},
		wc:   WithDefaultConfigurationName,
		want: nil,
	}, {
		name: "valid with no percent",
		tt: &TrafficTarget{
			ConfigurationName: "booga",
		},
		want: nil,
	}, {
		name: "valid with no name",
		tt: &TrafficTarget{
			ConfigurationName: "booga",
			Percent:           100,
		},
		want: nil,
	}, {
		name: "invalid with both",
		tt: &TrafficTarget{
			RevisionName:      "foo",
			ConfigurationName: "bar",
		},
		want: &apis.FieldError{
			Message: "expected exactly one, got both",
			Paths:   []string{"revisionName", "configurationName"},
		},
	}, {
		name: "invalid with neither",
		tt: &TrafficTarget{
			Percent: 100,
		},
		want: &apis.FieldError{
			Message: "expected exactly one, got neither",
			Paths:   []string{"revisionName", "configurationName"},
		},
	}, {
		name: "invalid percent too low",
		tt: &TrafficTarget{
			RevisionName: "foo",
			Percent:      -5,
		},
		want: apis.ErrOutOfBoundsValue("-5", "0", "100", "percent"),
	}, {
		name: "invalid percent too high",
		tt: &TrafficTarget{
			RevisionName: "foo",
			Percent:      101,
		},
		want: apis.ErrOutOfBoundsValue("101", "0", "100", "percent"),
	}, {
		name: "disallowed url set",
		tt: &TrafficTarget{
			ConfigurationName: "foo",
			Percent:           100,
			URL:               "ShouldNotBeSet",
		},
		wc:   apis.WithinSpec,
		want: apis.ErrDisallowedFields("url"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.wc != nil {
				ctx = test.wc(ctx)
			}
			got := test.tt.Validate(ctx)
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestRouteValidation(t *testing.T) {
	tests := []struct {
		name string
		r    *Route
		want *apis.FieldError
	}{{
		name: "valid",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					Tag:          "bar",
					RevisionName: "foo",
					Percent:      100,
				}},
			},
			Status: RouteStatus{
				RouteStatusFields: RouteStatusFields{
					Traffic: []TrafficTarget{{
						Tag:          "bar",
						RevisionName: "foo",
						Percent:      100,
						URL:          "http://bar.blah.com",
					}},
				},
			},
		},
		want: nil,
	}, {
		name: "valid split",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					Tag:          "prod",
					RevisionName: "foo",
					Percent:      90,
				}, {
					Tag:               "experiment",
					ConfigurationName: "bar",
					Percent:           10,
				}},
			},
		},
		want: nil,
	}, {
		name: "missing url in status",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					Tag:          "bar",
					RevisionName: "foo",
					Percent:      100,
				}},
			},
			Status: RouteStatus{
				RouteStatusFields: RouteStatusFields{
					Traffic: []TrafficTarget{{
						Tag:          "bar",
						RevisionName: "foo",
						Percent:      100,
					}},
				},
			},
		},
		want: &apis.FieldError{
			Message: "missing field(s)",
			Paths: []string{
				"status.traffic[0].url",
			},
		},
	}, {
		name: "invalid traffic entry (missing oneof)",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					Tag:     "foo",
					Percent: 100,
				}},
			},
		},
		want: &apis.FieldError{
			Message: "expected exactly one, got neither",
			Paths: []string{
				"spec.traffic[0].configurationName",
				"spec.traffic[0].revisionName",
			},
		},
	}, {
		name: "invalid traffic entry (multiple names)",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					Tag:          "foo",
					RevisionName: "bar",
					Percent:      50,
				}, {
					Tag:          "foo",
					RevisionName: "bar",
					Percent:      50,
				}},
			},
		},
		want: &apis.FieldError{
			Message: `Multiple definitions for "foo"`,
			Paths: []string{
				"spec.traffic[0].tag",
				"spec.traffic[1].tag",
			},
		},
	}, {
		name: "invalid name - dots",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "do.not.use.dots",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					RevisionName: "foo",
					Percent:      100,
				}},
			},
		},
		want: &apis.FieldError{
			Message: "not a DNS 1035 label: [a DNS-1035 label must consist of lower case alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character (e.g. 'my-name',  or 'abc-123', regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')]",
			Paths:   []string{"metadata.name"},
		},
	}, {
		name: "invalid name - dots and spec percent is not 100",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "do.not.use.dots",
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					RevisionName: "foo",
					Percent:      90,
				}},
			},
		},
		want: (&apis.FieldError{
			Message: "not a DNS 1035 label: [a DNS-1035 label must consist of lower case alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character (e.g. 'my-name',  or 'abc-123', regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')]",
			Paths:   []string{"metadata.name"},
		}).Also(&apis.FieldError{
			Message: "Traffic targets sum to 90, want 100",
			Paths:   []string{"spec.traffic"},
		}),
	}, {
		name: "invalid name - too long",
		r: &Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: strings.Repeat("a", 64),
			},
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					RevisionName: "foo",
					Percent:      100,
				}},
			},
		},
		want: &apis.FieldError{
			Message: "not a DNS 1035 label: [must be no more than 63 characters]",
			Paths:   []string{"metadata.name"},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.r.Validate(context.Background())
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}
