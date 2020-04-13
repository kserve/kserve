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
	"knative.dev/serving/pkg/apis/serving"
	"knative.dev/serving/pkg/reconciler/route/config"
)

// Validate makes sure that Route is properly configured.
func (r *Route) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(r.GetObjectMeta()).Also(
		r.validateLabels().ViaField("labels")).ViaField("metadata")
	errs = errs.Also(r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	errs = errs.Also(r.Status.Validate(apis.WithinStatus(ctx)).ViaField("status"))

	if apis.IsInUpdate(ctx) {
		original := apis.GetBaseline(ctx).(*Route)
		// Don't validate annotations(creator and lastModifier) when route owned by service
		// validate only when route created independently.
		if r.OwnerReferences == nil {
			errs = errs.Also(apis.ValidateCreatorAndModifier(original.Spec, r.Spec, original.GetAnnotations(),
				r.GetAnnotations(), serving.GroupName).ViaField("metadata.annotations"))
		}
	}
	return errs
}

// validateLabels function validates route labels.
func (r *Route) validateLabels() (errs *apis.FieldError) {
	for key, val := range r.GetLabels() {
		switch {
		case key == config.VisibilityLabelKey:
			errs = errs.Also(serving.ValidateClusterVisibilityLabel(val))
		case key == serving.ServiceLabelKey:
			errs = errs.Also(verifyLabelOwnerRef(val, serving.ServiceLabelKey, "Service", r.GetOwnerReferences()))
		case strings.HasPrefix(key, serving.GroupNamePrefix):
			errs = errs.Also(apis.ErrInvalidKeyName(key, apis.CurrentField))
		}
	}
	return
}
