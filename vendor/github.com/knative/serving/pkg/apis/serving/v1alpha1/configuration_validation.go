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

	"k8s.io/apimachinery/pkg/api/equality"

	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving"
)

// Validate makes sure that Configuration is properly configured.
func (c *Configuration) Validate(ctx context.Context) (errs *apis.FieldError) {
	// If we are in a status sub resource update, the metadata and spec cannot change.
	// So, to avoid rejecting controller status updates due to validations that may
	// have changed (i.e. due to config-defaults changes), we elide the metadata and
	// spec validation.
	if !apis.IsInStatusUpdate(ctx) {
		errs = errs.Also(serving.ValidateObjectMetadata(c.GetObjectMeta()).ViaField("metadata"))
		ctx = apis.WithinParent(ctx, c.ObjectMeta)
		errs = errs.Also(c.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	}

	if apis.IsInUpdate(ctx) {
		original := apis.GetBaseline(ctx).(*Configuration)

		err := c.Spec.GetTemplate().VerifyNameChange(ctx,
			original.Spec.GetTemplate())
		errs = errs.Also(err.ViaField("spec.revisionTemplate"))
	}

	return errs
}

// Validate makes sure that ConfigurationSpec is properly configured.
func (cs *ConfigurationSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(cs, &ConfigurationSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := apis.CheckDeprecated(ctx, cs)

	// Build support is now disabled.
	if cs.DeprecatedBuild != nil {
		errs = errs.Also(apis.ErrDisallowedFields("build"))
	}

	var templateField string
	switch {
	case cs.DeprecatedRevisionTemplate != nil && cs.Template != nil:
		return apis.ErrMultipleOneOf("revisionTemplate", "template")
	case cs.DeprecatedRevisionTemplate != nil:
		templateField = "revisionTemplate"
	case cs.Template != nil:
		templateField = "template"
		// Disallow the use of deprecated fields under "template".
		ctx = apis.DisallowDeprecated(ctx)
	default:
		return apis.ErrMissingOneOf("revisionTemplate", "template")
	}

	return errs.Also(cs.GetTemplate().Validate(ctx).ViaField(templateField))
}
