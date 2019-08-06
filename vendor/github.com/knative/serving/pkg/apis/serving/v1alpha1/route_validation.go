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
	"context"
	"fmt"

	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (r *Route) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(r.GetObjectMeta()).ViaField("metadata")
	errs = errs.Also(r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	return errs
}

func (rs *RouteSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(rs, &RouteSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := apis.CheckDeprecated(ctx, rs)

	type diagnostic struct {
		index int
		field string
	}
	// Track the targets of named TrafficTarget entries (to detect duplicates).
	trafficMap := make(map[string]diagnostic)

	percentSum := 0
	for i, tt := range rs.Traffic {
		// Delegate to the v1beta1 validation.
		errs = errs.Also(tt.TrafficTarget.Validate(ctx).ViaFieldIndex("traffic", i))

		percentSum += tt.Percent

		if tt.DeprecatedName != "" && tt.Tag != "" {
			errs = errs.Also(apis.ErrMultipleOneOf("name", "tag").
				ViaFieldIndex("traffic", i))
		} else if tt.DeprecatedName == "" && tt.Tag == "" {
			// No Name field, so skip the uniqueness check.
			continue
		}

		errs = errs.Also(apis.CheckDeprecated(ctx, tt).ViaFieldIndex("traffic", i))

		name := tt.DeprecatedName
		field := "name"
		if name == "" {
			name = tt.Tag
			field = "tag"
		}

		if d, ok := trafficMap[name]; !ok {
			// No entry exists, so add ours
			trafficMap[name] = diagnostic{i, field}
		} else {
			// We want only single definition of the route, even if it points
			// to the same config or revision.
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("Multiple definitions for %q", name),
				Paths: []string{
					fmt.Sprintf("traffic[%d].%s", d.index, d.field),
					fmt.Sprintf("traffic[%d].%s", i, field),
				},
			})
		}
	}

	if percentSum != 100 {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("Traffic targets sum to %d, want 100", percentSum),
			Paths:   []string{"traffic"},
		})
	}
	return errs
}
