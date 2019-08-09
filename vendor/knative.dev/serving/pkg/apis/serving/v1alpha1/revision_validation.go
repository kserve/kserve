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
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
)

func (r *Revision) checkImmutableFields(ctx context.Context, original *Revision) *apis.FieldError {
	if diff, err := kmp.ShortDiff(original.Spec, r.Spec); err != nil {
		return &apis.FieldError{
			Message: "Failed to diff Revision",
			Paths:   []string{"spec"},
			Details: err.Error(),
		}
	} else if diff != "" {
		return &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: diff,
		}
	}
	return nil
}

// Validate ensures Revision is properly configured.
func (r *Revision) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(r.GetObjectMeta()).ViaField("metadata")
	if apis.IsInUpdate(ctx) {
		old := apis.GetBaseline(ctx).(*Revision)
		errs = errs.Also(r.checkImmutableFields(ctx, old))
	} else {
		errs = errs.Also(r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	}
	return errs
}

// Validate ensures RevisionTemplateSpec is properly configured.
func (rt *RevisionTemplateSpec) Validate(ctx context.Context) *apis.FieldError {
	errs := rt.Spec.Validate(ctx).ViaField("spec")
	errs = errs.Also(autoscaling.ValidateAnnotations(rt.GetAnnotations()).ViaField("metadata.annotations"))

	// If the DeprecatedRevisionTemplate has a name specified, then check that
	// it follows the requirements on the name.
	if rt.Name != "" {
		om := apis.ParentMeta(ctx)
		prefix := om.Name + "-"
		if om.Name != "" {
			// Even if there is GenerateName, allow the use
			// of Name post-creation.
		} else if om.GenerateName != "" {
			// We disallow bringing your own name when the parent
			// resource uses generateName (at creation).
			return apis.ErrDisallowedFields("metadata.name")
		}

		if !strings.HasPrefix(rt.Name, prefix) {
			errs = errs.Also(apis.ErrInvalidValue(
				fmt.Sprintf("%q must have prefix %q", rt.Name, prefix),
				"metadata.name"))
		}
	}

	errs = errs.Also(serving.ValidateQueueSidecarAnnotation(rt.Annotations).ViaField("metadata.annotations"))
	return errs
}

// VerifyNameChange checks that if a user brought their own name previously that it
// changes at the appropriate times.
func (current *RevisionTemplateSpec) VerifyNameChange(ctx context.Context, og *RevisionTemplateSpec) *apis.FieldError {
	if current.Name == "" {
		// We only check that Name changes when the DeprecatedRevisionTemplate changes.
		return nil
	}
	if current.Name != og.Name {
		// The name changed, so we're good.
		return nil
	}

	if diff, err := kmp.ShortDiff(og, current); err != nil {
		return &apis.FieldError{
			Message: "Failed to diff DeprecatedRevisionTemplate",
			Paths:   []string{apis.CurrentField},
			Details: err.Error(),
		}
	} else if diff != "" {
		return &apis.FieldError{
			Message: "Saw the following changes without a name change (-old +new)",
			Paths:   []string{apis.CurrentField},
			Details: diff,
		}
	}
	return nil
}

// Validate ensures RevisionSpec is properly configured.
func (rs *RevisionSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(rs, &RevisionSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := apis.CheckDeprecated(ctx, rs)

	switch {
	case len(rs.PodSpec.Containers) > 0 && rs.DeprecatedContainer != nil:
		errs = errs.Also(apis.ErrMultipleOneOf("container", "containers"))
	case len(rs.PodSpec.Containers) > 0:
		errs = errs.Also(rs.RevisionSpec.Validate(ctx))
	case rs.DeprecatedContainer != nil:
		volumes, err := serving.ValidateVolumes(rs.Volumes)
		if err != nil {
			errs = errs.Also(err.ViaField("volumes"))
		}
		errs = errs.Also(serving.ValidateContainer(
			*rs.DeprecatedContainer, volumes).ViaField("container"))
	default:
		errs = errs.Also(apis.ErrMissingOneOf("container", "containers"))
	}

	if rs.DeprecatedBuildRef != nil {
		errs = errs.Also(apis.ErrDisallowedFields("buildRef"))
	}

	if err := rs.DeprecatedConcurrencyModel.Validate(ctx).ViaField("concurrencyModel"); err != nil {
		errs = errs.Also(err)
	} else {
		errs = errs.Also(rs.ContainerConcurrency.Validate(ctx).ViaField("containerConcurrency"))
	}

	if rs.TimeoutSeconds != nil {
		errs = errs.Also(serving.ValidateTimeoutSeconds(ctx, *rs.TimeoutSeconds))
	}
	return errs
}

// Validate ensures DeprecatedRevisionServingStateType is properly configured.
func (ss DeprecatedRevisionServingStateType) Validate(ctx context.Context) *apis.FieldError {
	switch ss {
	case DeprecatedRevisionServingStateType(""),
		DeprecatedRevisionServingStateRetired,
		DeprecatedRevisionServingStateReserve,
		DeprecatedRevisionServingStateActive:
		return nil
	default:
		return apis.ErrInvalidValue(ss, apis.CurrentField)
	}
}

// Validate ensures RevisionRequestConcurrencyModelType is properly configured.
func (cm DeprecatedRevisionRequestConcurrencyModelType) Validate(ctx context.Context) *apis.FieldError {
	switch cm {
	case DeprecatedRevisionRequestConcurrencyModelType(""),
		DeprecatedRevisionRequestConcurrencyModelMulti,
		DeprecatedRevisionRequestConcurrencyModelSingle:
		return nil
	default:
		return apis.ErrInvalidValue(cm, apis.CurrentField)
	}
}
