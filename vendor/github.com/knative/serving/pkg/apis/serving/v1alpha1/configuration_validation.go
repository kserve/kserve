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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving"
)

// Validate makes sure that Configuration is properly configured.
func (c *Configuration) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(c.GetObjectMeta()).ViaField("metadata")
	ctx = apis.WithinParent(ctx, c.ObjectMeta)
	errs = errs.Also(c.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))

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

	errs := CheckDeprecated(ctx, map[string]interface{}{
		"generation": cs.DeprecatedGeneration,
	})

	if cs.Build == nil {
		// No build was specified.
	} else if err := cs.Build.As(&buildv1alpha1.BuildSpec{}); err == nil {
		// It is a BuildSpec, this is the legacy path.
	} else if err = cs.Build.As(&unstructured.Unstructured{}); err == nil {
		// It is an unstructured.Unstructured.
	} else {
		errs = errs.Also(apis.ErrInvalidValue(err, "build"))
	}

	var templateField string
	switch {
	case cs.RevisionTemplate != nil && cs.Template != nil:
		return apis.ErrMultipleOneOf("revisionTemplate", "template")
	case cs.RevisionTemplate != nil:
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
