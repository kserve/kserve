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
	duckv1 "knative.dev/pkg/apis/duck/v1"
	duckv1alpha1 "knative.dev/pkg/apis/duck/v1alpha1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving/v1beta1"
)

// ConvertUp implements apis.Convertible
func (source *Route) ConvertUp(ctx context.Context, obj apis.Convertible) error {
	switch sink := obj.(type) {
	case *v1beta1.Route:
		sink.ObjectMeta = source.ObjectMeta
		source.Status.ConvertUp(apis.WithinStatus(ctx), &sink.Status)
		return source.Spec.ConvertUp(apis.WithinSpec(ctx), &sink.Spec)
	default:
		return fmt.Errorf("unknown version, got: %T", sink)
	}
}

// ConvertUp helps implement apis.Convertible
func (source *RouteSpec) ConvertUp(ctx context.Context, sink *v1.RouteSpec) error {
	sink.Traffic = make([]v1.TrafficTarget, len(source.Traffic))
	for i := range source.Traffic {
		if err := source.Traffic[i].ConvertUp(ctx, &sink.Traffic[i]); err != nil {
			return err
		}
	}
	return nil
}

// ConvertUp helps implement apis.Convertible
func (source *TrafficTarget) ConvertUp(ctx context.Context, sink *v1.TrafficTarget) error {
	*sink = source.TrafficTarget
	switch {
	case source.Tag != "" && source.DeprecatedName != "":
		if apis.IsInSpec(ctx) {
			return apis.ErrMultipleOneOf("name", "tag")
		}
	case source.DeprecatedName != "":
		sink.Tag = source.DeprecatedName
	}
	return nil
}

// ConvertUp helps implement apis.Convertible
func (source *RouteStatus) ConvertUp(ctx context.Context, sink *v1.RouteStatus) {
	source.Status.ConvertTo(ctx, &sink.Status)

	source.RouteStatusFields.ConvertUp(ctx, &sink.RouteStatusFields)
}

// ConvertUp helps implement apis.Convertible
func (source *RouteStatusFields) ConvertUp(ctx context.Context, sink *v1.RouteStatusFields) {
	if source.URL != nil {
		sink.URL = source.URL.DeepCopy()
	}

	if source.Address != nil {
		if sink.Address == nil {
			sink.Address = &duckv1.Addressable{}
		}
		source.Address.ConvertUp(ctx, sink.Address)
	}

	sink.Traffic = make([]v1.TrafficTarget, len(source.Traffic))
	for i := range source.Traffic {
		source.Traffic[i].ConvertUp(ctx, &sink.Traffic[i])
	}
}

// ConvertDown implements apis.Convertible
func (sink *Route) ConvertDown(ctx context.Context, obj apis.Convertible) error {
	switch source := obj.(type) {
	case *v1beta1.Route:
		sink.ObjectMeta = source.ObjectMeta
		sink.Spec.ConvertDown(ctx, source.Spec)
		sink.Status.ConvertDown(ctx, source.Status)
		return nil
	default:
		return fmt.Errorf("unknown version, got: %T", source)
	}
}

// ConvertDown helps implement apis.Convertible
func (sink *RouteSpec) ConvertDown(ctx context.Context, source v1.RouteSpec) {
	sink.Traffic = make([]TrafficTarget, len(source.Traffic))
	for i := range source.Traffic {
		sink.Traffic[i].ConvertDown(ctx, source.Traffic[i])
	}
}

// ConvertDown helps implement apis.Convertible
func (sink *TrafficTarget) ConvertDown(ctx context.Context, source v1.TrafficTarget) {
	sink.TrafficTarget = source
}

// ConvertDown helps implement apis.Convertible
func (sink *RouteStatus) ConvertDown(ctx context.Context, source v1.RouteStatus) {
	source.Status.ConvertTo(ctx, &sink.Status)

	sink.RouteStatusFields.ConvertDown(ctx, source.RouteStatusFields)
}

// ConvertDown helps implement apis.Convertible
func (sink *RouteStatusFields) ConvertDown(ctx context.Context, source v1.RouteStatusFields) {
	if source.URL != nil {
		sink.URL = source.URL.DeepCopy()
	}

	if source.Address != nil {
		if sink.Address == nil {
			sink.Address = &duckv1alpha1.Addressable{}
		}
		sink.Address.ConvertDown(ctx, source.Address)
	}

	sink.Traffic = make([]TrafficTarget, len(source.Traffic))
	for i := range source.Traffic {
		sink.Traffic[i].ConvertDown(ctx, source.Traffic[i])
	}
}
