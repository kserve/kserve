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
	"fmt"

	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Validate makes sure that Route is properly configured.
func (r *Route) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(r.GetObjectMeta()).ViaField("metadata")
	errs = errs.Also(r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	errs = errs.Also(r.Status.Validate(apis.WithinStatus(ctx)).ViaField("status"))
	return errs
}

func validateTrafficList(ctx context.Context, traffic []TrafficTarget) *apis.FieldError {
	var errs *apis.FieldError

	// Track the targets of named TrafficTarget entries (to detect duplicates).
	trafficMap := make(map[string]int)

	sum := 0
	for i, tt := range traffic {
		errs = errs.Also(tt.Validate(ctx).ViaIndex(i))

		if idx, ok := trafficMap[tt.Tag]; ok {
			// We want only single definition of the route, even if it points
			// to the same config or revision.
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("Multiple definitions for %q", tt.Tag),
				Paths: []string{
					fmt.Sprintf("[%d].tag", i),
					fmt.Sprintf("[%d].tag", idx),
				},
			})
		} else {
			trafficMap[tt.Tag] = i
		}
		sum += tt.Percent
	}

	if sum != 100 {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("Traffic targets sum to %d, want 100", sum),
			Paths:   []string{apis.CurrentField},
		})
	}
	return errs
}

// Validate implements apis.Validatable
func (rs *RouteSpec) Validate(ctx context.Context) *apis.FieldError {
	return validateTrafficList(ctx, rs.Traffic).ViaField("traffic")
}

// Validate verifies that TrafficTarget is properly configured.
func (tt *TrafficTarget) Validate(ctx context.Context) *apis.FieldError {
	var errs *apis.FieldError

	// We only validate the sense of latestRevision in the context of a Spec,
	// and only when it is specified.
	if apis.IsInSpec(ctx) && tt.LatestRevision != nil {
		lr := *tt.LatestRevision
		pinned := tt.RevisionName != ""
		if pinned == lr {
			// The senses for whether to pin to a particular revision or
			// float forward to the latest revision must match.
			errs = errs.Also(apis.ErrInvalidValue(lr, "latestRevision"))
		}
	}

	switch {
	// When we have a default configurationName, we don't
	// allow one to be specified.
	case HasDefaultConfigurationName(ctx) && tt.ConfigurationName != "":
		errs = errs.Also(apis.ErrDisallowedFields("configurationName"))

	// Both revisionName and configurationName are never allowed to
	// appear concurrently.
	case tt.RevisionName != "" && tt.ConfigurationName != "":
		errs = errs.Also(apis.ErrMultipleOneOf(
			"revisionName", "configurationName"))

	// When a revisionName appears, we must check that the name is valid.
	case tt.RevisionName != "":
		if el := validation.IsQualifiedName(tt.RevisionName); len(el) > 0 {
			errs = errs.Also(apis.ErrInvalidKeyName(
				tt.RevisionName, "revisionName", el...))
		}

	// When revisionName is missing in Status report an error.
	case apis.IsInStatus(ctx):
		errs = errs.Also(apis.ErrMissingField("revisionName"))

	// When configurationName is specified, we must check that the name is valid.
	case tt.ConfigurationName != "":
		if el := validation.IsQualifiedName(tt.ConfigurationName); len(el) > 0 {
			errs = errs.Also(apis.ErrInvalidKeyName(
				tt.ConfigurationName, "configurationName", el...))
		}

	// When we are using a default configurationName, it must be a valid name already.
	case HasDefaultConfigurationName(ctx):

	// All other cases are missing one of revisionName or configurationName.
	default:
		errs = errs.Also(apis.ErrMissingOneOf(
			"revisionName", "configurationName"))
	}

	// Check that the traffic Percentage is within bounds.
	if tt.Percent < 0 || tt.Percent > 100 {
		errs = errs.Also(apis.ErrOutOfBoundsValue(
			tt.Percent, 0, 100, "percent"))
	}

	// Check that we set the URL appropriately.
	if tt.URL.String() != "" {
		// URL is not allowed in traffic under spec.
		if apis.IsInSpec(ctx) {
			errs = errs.Also(apis.ErrDisallowedFields("url"))
		}

		// URL is not allowed in any traffic target without a name.
		if tt.Tag == "" {
			errs = errs.Also(apis.ErrDisallowedFields("url"))
		}
	} else if tt.Tag != "" {
		// URL must be specified in status when name is specified.
		if apis.IsInStatus(ctx) {
			errs = errs.Also(apis.ErrMissingField("url"))
		}
	}

	return errs
}

// Validate implements apis.Validatable
func (rs *RouteStatus) Validate(ctx context.Context) *apis.FieldError {
	return rs.RouteStatusFields.Validate(ctx)
}

// Validate implements apis.Validatable
func (rsf *RouteStatusFields) Validate(ctx context.Context) *apis.FieldError {
	// TODO(mattmoor): Validate other status fields.

	if len(rsf.Traffic) != 0 {
		return validateTrafficList(ctx, rsf.Traffic).ViaField("traffic")
	}
	return nil
}
