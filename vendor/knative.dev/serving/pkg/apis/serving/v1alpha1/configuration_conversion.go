/*
Copyright 2019 The Knative Authors.

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

	"knative.dev/pkg/apis"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving/v1beta1"
)

// ConvertUp implements apis.Convertible
func (source *Configuration) ConvertUp(ctx context.Context, obj apis.Convertible) error {
	switch sink := obj.(type) {
	case *v1beta1.Configuration:
		sink.ObjectMeta = source.ObjectMeta
		if err := source.Spec.ConvertUp(ctx, &sink.Spec); err != nil {
			return err
		}
		return source.Status.ConvertUp(ctx, &sink.Status)
	case *v1.Configuration:
		sink.ObjectMeta = source.ObjectMeta
		if err := source.Spec.ConvertUp(ctx, &sink.Spec); err != nil {
			return err
		}
		return source.Status.ConvertUp(ctx, &sink.Status)
	default:
		return fmt.Errorf("unknown version, got: %T", sink)
	}
}

// ConvertUp helps implement apis.Convertible
func (source *ConfigurationSpec) ConvertUp(ctx context.Context, sink *v1.ConfigurationSpec) error {
	if source.DeprecatedBuild != nil {
		return ConvertErrorf("build", "build cannot be migrated forward.")
	}
	switch {
	case source.DeprecatedRevisionTemplate != nil && source.Template != nil:
		return apis.ErrMultipleOneOf("revisionTemplate", "template")
	case source.DeprecatedRevisionTemplate != nil:
		return source.DeprecatedRevisionTemplate.ConvertUp(ctx, &sink.Template)
	case source.Template != nil:
		return source.Template.ConvertUp(ctx, &sink.Template)
	default:
		return apis.ErrMissingOneOf("revisionTemplate", "template")
	}
}

// ConvertUp helps implement apis.Convertible
func (source *ConfigurationStatus) ConvertUp(ctx context.Context, sink *v1.ConfigurationStatus) error {
	source.Status.ConvertTo(ctx, &sink.Status)

	return source.ConfigurationStatusFields.ConvertUp(ctx, &sink.ConfigurationStatusFields)
}

// ConvertUp helps implement apis.Convertible
func (source *ConfigurationStatusFields) ConvertUp(ctx context.Context, sink *v1.ConfigurationStatusFields) error {
	sink.LatestReadyRevisionName = source.LatestReadyRevisionName
	sink.LatestCreatedRevisionName = source.LatestCreatedRevisionName
	return nil
}

// ConvertDown implements apis.Convertible
func (sink *Configuration) ConvertDown(ctx context.Context, obj apis.Convertible) error {
	switch source := obj.(type) {
	case *v1beta1.Configuration:
		sink.ObjectMeta = source.ObjectMeta
		if err := sink.Spec.ConvertDown(ctx, source.Spec); err != nil {
			return err
		}
		return sink.Status.ConvertDown(ctx, source.Status)
	case *v1.Configuration:
		sink.ObjectMeta = source.ObjectMeta
		if err := sink.Spec.ConvertDown(ctx, source.Spec); err != nil {
			return err
		}
		return sink.Status.ConvertDown(ctx, source.Status)
	default:
		return fmt.Errorf("unknown version, got: %T", source)
	}
}

// ConvertDown helps implement apis.Convertible
func (sink *ConfigurationSpec) ConvertDown(ctx context.Context, source v1.ConfigurationSpec) error {
	sink.Template = &RevisionTemplateSpec{}
	return sink.Template.ConvertDown(ctx, source.Template)
}

// ConvertDown helps implement apis.Convertible
func (sink *ConfigurationStatus) ConvertDown(ctx context.Context, source v1.ConfigurationStatus) error {
	source.Status.ConvertTo(ctx, &sink.Status)

	return sink.ConfigurationStatusFields.ConvertDown(ctx, source.ConfigurationStatusFields)
}

// ConvertDown helps implement apis.Convertible
func (sink *ConfigurationStatusFields) ConvertDown(ctx context.Context, source v1.ConfigurationStatusFields) error {
	sink.LatestReadyRevisionName = source.LatestReadyRevisionName
	sink.LatestCreatedRevisionName = source.LatestCreatedRevisionName
	return nil
}
