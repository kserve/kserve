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

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving/v1beta1"
)

// ConvertUp implements apis.Convertible
func (source *Revision) ConvertUp(ctx context.Context, obj apis.Convertible) error {
	switch sink := obj.(type) {
	case *v1beta1.Revision:
		sink.ObjectMeta = source.ObjectMeta
		source.Status.ConvertUp(ctx, &sink.Status)
		return source.Spec.ConvertUp(ctx, &sink.Spec)
	default:
		return fmt.Errorf("unknown version, got: %T", sink)
	}
}

// ConvertUp helps implement apis.Convertible
func (source *RevisionTemplateSpec) ConvertUp(ctx context.Context, sink *v1.RevisionTemplateSpec) error {
	sink.ObjectMeta = source.ObjectMeta
	return source.Spec.ConvertUp(ctx, &sink.Spec)
}

// ConvertUp helps implement apis.Convertible
func (source *RevisionSpec) ConvertUp(ctx context.Context, sink *v1.RevisionSpec) error {
	if source.TimeoutSeconds != nil {
		sink.TimeoutSeconds = ptr.Int64(*source.TimeoutSeconds)
	}
	if source.ContainerConcurrency != nil {
		sink.ContainerConcurrency = ptr.Int64(*source.ContainerConcurrency)
	}
	switch {
	case source.DeprecatedContainer != nil && len(source.Containers) > 0:
		return apis.ErrMultipleOneOf("container", "containers")
	case source.DeprecatedContainer != nil:
		sink.PodSpec = corev1.PodSpec{
			ServiceAccountName: source.ServiceAccountName,
			Containers:         []corev1.Container{*source.DeprecatedContainer},
			Volumes:            source.Volumes,
		}
	case len(source.Containers) == 1:
		sink.PodSpec = source.PodSpec
	case len(source.Containers) > 1:
		return apis.ErrMultipleOneOf("containers")
	default:
		return apis.ErrMissingOneOf("container", "containers")
	}
	if source.DeprecatedBuildRef != nil {
		return ConvertErrorf("buildRef",
			"buildRef cannot be migrated forward, got: %#v", source.DeprecatedBuildRef)
	}
	return nil
}

// ConvertUp helps implement apis.Convertible
func (source *RevisionStatus) ConvertUp(ctx context.Context, sink *v1.RevisionStatus) {
	source.Status.ConvertTo(ctx, &sink.Status)

	sink.ServiceName = source.ServiceName
	sink.LogURL = source.LogURL
	// TODO(mattmoor): ImageDigest?
}

// ConvertDown implements apis.Convertible
func (sink *Revision) ConvertDown(ctx context.Context, obj apis.Convertible) error {
	switch source := obj.(type) {
	case *v1beta1.Revision:
		sink.ObjectMeta = source.ObjectMeta
		sink.Status.ConvertDown(ctx, source.Status)
		return sink.Spec.ConvertDown(ctx, source.Spec)
	default:
		return fmt.Errorf("unknown version, got: %T", source)
	}
}

// ConvertDown helps implement apis.Convertible
func (sink *RevisionTemplateSpec) ConvertDown(ctx context.Context, source v1.RevisionTemplateSpec) error {
	sink.ObjectMeta = source.ObjectMeta
	return sink.Spec.ConvertDown(ctx, source.Spec)
}

// ConvertDown helps implement apis.Convertible
func (sink *RevisionSpec) ConvertDown(ctx context.Context, source v1.RevisionSpec) error {
	sink.RevisionSpec = *source.DeepCopy()
	return nil
}

// ConvertDown helps implement apis.Convertible
func (sink *RevisionStatus) ConvertDown(ctx context.Context, source v1.RevisionStatus) {
	source.Status.ConvertTo(ctx, &sink.Status)

	sink.ServiceName = source.ServiceName
	sink.LogURL = source.LogURL
	// TODO(mattmoor): ImageDigest?
}
