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
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

// Validate validates the fields belonging to Service
func (s *Service) Validate(ctx context.Context) (errs *apis.FieldError) {
	// If we are in a status sub resource update, the metadata and spec cannot change.
	// So, to avoid rejecting controller status updates due to validations that may
	// have changed (i.e. due to config-defaults changes), we elide the metadata and
	// spec validation.
	if !apis.IsInStatusUpdate(ctx) {
		errs = errs.Also(serving.ValidateObjectMetadata(s.GetObjectMeta()).ViaField("metadata"))
		ctx = apis.WithinParent(ctx, s.ObjectMeta)
		errs = errs.Also(s.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	}

	if apis.IsInUpdate(ctx) {
		original := apis.GetBaseline(ctx).(*Service)

		field, currentConfig := s.Spec.getConfigurationSpec()
		_, originalConfig := original.Spec.getConfigurationSpec()

		if currentConfig != nil && originalConfig != nil {
			templateField := "template"
			if currentConfig.GetTemplate() == currentConfig.DeprecatedRevisionTemplate {
				templateField = "revisionTemplate"
			}
			err := currentConfig.GetTemplate().VerifyNameChange(ctx,
				originalConfig.GetTemplate())
			errs = errs.Also(err.ViaField("spec", field, "configuration", templateField))
		}
	}

	return errs
}

func (ss *ServiceSpec) getConfigurationSpec() (string, *ConfigurationSpec) {
	switch {
	case ss.DeprecatedRunLatest != nil:
		return "runLatest", &ss.DeprecatedRunLatest.Configuration
	case ss.DeprecatedRelease != nil:
		return "release", &ss.DeprecatedRelease.Configuration
	case ss.DeprecatedPinned != nil:
		return "pinned", &ss.DeprecatedPinned.Configuration
	default:
		return "", &ss.ConfigurationSpec
	}
}

// Validate validates the fields belonging to ServiceSpec recursively
func (ss *ServiceSpec) Validate(ctx context.Context) *apis.FieldError {
	// We would do this semantic DeepEqual, but the spec is comprised
	// entirely of a oneof, the validation for which produces a clearer
	// error message.
	// if equality.Semantic.DeepEqual(ss, &ServiceSpec{}) {
	// 	return apis.ErrMissingField(currentField)
	// }

	errs := apis.CheckDeprecated(ctx, ss)

	set := []string{}

	if ss.DeprecatedRunLatest != nil {
		set = append(set, "runLatest")
		errs = errs.Also(ss.DeprecatedRunLatest.Validate(ctx).ViaField("runLatest"))
	}
	if ss.DeprecatedRelease != nil {
		set = append(set, "release")
		errs = errs.Also(ss.DeprecatedRelease.Validate(ctx).ViaField("release"))
	}
	if ss.DeprecatedManual != nil {
		set = append(set, "manual")
		errs = errs.Also(apis.ErrDisallowedFields("manual"))
	}
	if ss.DeprecatedPinned != nil {
		set = append(set, "pinned")
		errs = errs.Also(ss.DeprecatedPinned.Validate(ctx).ViaField("pinned"))
	}

	// Before checking ConfigurationSpec, check RouteSpec.
	if len(set) > 0 && len(ss.RouteSpec.Traffic) > 0 {
		errs = errs.Also(apis.ErrMultipleOneOf(
			append([]string{"traffic"}, set...)...))
	}

	if !equality.Semantic.DeepEqual(ss.ConfigurationSpec, ConfigurationSpec{}) {
		set = append(set, "template")

		// Disallow the use of deprecated fields within our inlined
		// Configuration and Route specs.
		ctx = apis.DisallowDeprecated(ctx)

		errs = errs.Also(ss.ConfigurationSpec.Validate(ctx))
		errs = errs.Also(ss.RouteSpec.Validate(
			// Within the context of Service, the RouteSpec has a default
			// configurationName.
			v1beta1.WithDefaultConfigurationName(ctx)))
	}

	if len(set) > 1 {
		errs = errs.Also(apis.ErrMultipleOneOf(set...))
	} else if len(set) == 0 {
		errs = errs.Also(apis.ErrMissingOneOf("runLatest", "release", "pinned", "template"))
	}
	return errs
}

// Validate validates the fields belonging to PinnedType
func (pt *PinnedType) Validate(ctx context.Context) *apis.FieldError {
	var errs *apis.FieldError
	if pt.RevisionName == "" {
		errs = apis.ErrMissingField("revisionName")
	}
	return errs.Also(pt.Configuration.Validate(ctx).ViaField("configuration"))
}

// Validate validates the fields belonging to RunLatestType
func (rlt *RunLatestType) Validate(ctx context.Context) *apis.FieldError {
	return rlt.Configuration.Validate(ctx).ViaField("configuration")
}

// Validate validates the fields belonging to ReleaseType
func (rt *ReleaseType) Validate(ctx context.Context) *apis.FieldError {
	var errs *apis.FieldError

	numRevisions := len(rt.Revisions)
	if numRevisions == 0 {
		errs = errs.Also(apis.ErrMissingField("revisions"))
	}
	if numRevisions > 2 {
		errs = errs.Also(apis.ErrOutOfBoundsValue(numRevisions, 1, 2, "revisions"))
	}
	for i, r := range rt.Revisions {
		// Skip over the last revision special keyword.
		if r == ReleaseLatestRevisionKeyword {
			continue
		}
		if msgs := validation.IsDNS1035Label(r); len(msgs) > 0 {
			errs = errs.Also(apis.ErrInvalidArrayValue(
				fmt.Sprintf("not a DNS 1035 label: %v", msgs),
				"revisions", i))
		}
	}

	if numRevisions < 2 && rt.RolloutPercent != 0 {
		errs = errs.Also(apis.ErrInvalidValue(rt.RolloutPercent, "rolloutPercent"))
	}

	if rt.RolloutPercent < 0 || rt.RolloutPercent > 99 {
		errs = errs.Also(apis.ErrOutOfBoundsValue(
			rt.RolloutPercent, 0, 99,
			"rolloutPercent"))
	}

	return errs.Also(rt.Configuration.Validate(ctx).ViaField("configuration"))
}
