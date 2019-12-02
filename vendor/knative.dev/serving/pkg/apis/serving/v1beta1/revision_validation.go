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

	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/serving"
)

// Validate ensures Revision is properly configured.
func (r *Revision) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(r.GetObjectMeta()).Also(
		r.ValidateLabels().ViaField("labels")).ViaField("metadata")
	errs = errs.Also(r.Status.Validate(apis.WithinStatus(ctx)).ViaField("status"))

	if apis.IsInUpdate(ctx) {
		original := apis.GetBaseline(ctx).(*Revision)
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
	} else {
		errs = errs.Also(r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	}

	return errs
}

// ValidateLabels function validates service labels
func (r *Revision) ValidateLabels() (errs *apis.FieldError) {
	for key, val := range r.GetLabels() {
		switch {
		case key == serving.RouteLabelKey || key == serving.ServiceLabelKey || key == serving.ConfigurationGenerationLabelKey:
		case key == serving.ConfigurationLabelKey:
			errs = errs.Also(verifyLabelOwnerRef(val, serving.ConfigurationLabelKey, "Configuration", r.GetOwnerReferences()))
		case strings.HasPrefix(key, serving.GroupNamePrefix):
			errs = errs.Also(apis.ErrInvalidKeyName(key, ""))
		}
	}
	return
}
