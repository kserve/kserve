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

	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

// ConvertUp implements apis.Convertible
func (source *Service) ConvertUp(ctx context.Context, obj apis.Convertible) error {
	switch sink := obj.(type) {
	case *v1beta1.Service:
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
func (source *ServiceSpec) ConvertUp(ctx context.Context, sink *v1beta1.ServiceSpec) error {
	switch {
	case source.DeprecatedRunLatest != nil:
		sink.RouteSpec = v1beta1.RouteSpec{
			Traffic: []v1beta1.TrafficTarget{{
				Percent:        100,
				LatestRevision: ptr.Bool(true),
			}},
		}
		return source.DeprecatedRunLatest.Configuration.ConvertUp(ctx, &sink.ConfigurationSpec)

	case source.DeprecatedRelease != nil:
		if len(source.DeprecatedRelease.Revisions) == 2 {
			sink.RouteSpec = v1beta1.RouteSpec{
				Traffic: []v1beta1.TrafficTarget{{
					RevisionName: source.DeprecatedRelease.Revisions[0],
					Percent:      100 - source.DeprecatedRelease.RolloutPercent,
					Tag:          "current",
				}, {
					RevisionName: source.DeprecatedRelease.Revisions[1],
					Percent:      source.DeprecatedRelease.RolloutPercent,
					Tag:          "candidate",
				}, {
					Percent:        0,
					Tag:            "latest",
					LatestRevision: ptr.Bool(true),
				}},
			}
		} else {
			sink.RouteSpec = v1beta1.RouteSpec{
				Traffic: []v1beta1.TrafficTarget{{
					RevisionName: source.DeprecatedRelease.Revisions[0],
					Percent:      100,
					Tag:          "current",
				}, {
					Percent:        0,
					Tag:            "latest",
					LatestRevision: ptr.Bool(true),
				}},
			}
		}
		for i, tt := range sink.RouteSpec.Traffic {
			if tt.RevisionName == "@latest" {
				sink.RouteSpec.Traffic[i].RevisionName = ""
				sink.RouteSpec.Traffic[i].LatestRevision = ptr.Bool(true)
			}
		}
		return source.DeprecatedRelease.Configuration.ConvertUp(ctx, &sink.ConfigurationSpec)

	case source.DeprecatedPinned != nil:
		sink.RouteSpec = v1beta1.RouteSpec{
			Traffic: []v1beta1.TrafficTarget{{
				RevisionName: source.DeprecatedPinned.RevisionName,
				Percent:      100,
			}},
		}
		return source.DeprecatedPinned.Configuration.ConvertUp(ctx, &sink.ConfigurationSpec)

	case source.DeprecatedManual != nil:
		return ConvertErrorf("manual", "manual mode cannot be migrated forward.")

	default:
		source.RouteSpec.ConvertUp(ctx, &sink.RouteSpec)
		return source.ConfigurationSpec.ConvertUp(ctx, &sink.ConfigurationSpec)
	}
}

// ConvertUp helps implement apis.Convertible
func (source *ServiceStatus) ConvertUp(ctx context.Context, sink *v1beta1.ServiceStatus) error {
	source.Status.ConvertTo(ctx, &sink.Status)

	source.RouteStatusFields.ConvertUp(ctx, &sink.RouteStatusFields)
	return source.ConfigurationStatusFields.ConvertUp(ctx, &sink.ConfigurationStatusFields)
}

// ConvertDown implements apis.Convertible
func (sink *Service) ConvertDown(ctx context.Context, obj apis.Convertible) error {
	switch source := obj.(type) {
	case *v1beta1.Service:
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
func (sink *ServiceSpec) ConvertDown(ctx context.Context, source v1beta1.ServiceSpec) error {
	sink.RouteSpec.ConvertDown(ctx, source.RouteSpec)
	return sink.ConfigurationSpec.ConvertDown(ctx, source.ConfigurationSpec)
}

// ConvertDown helps implement apis.Convertible
func (sink *ServiceStatus) ConvertDown(ctx context.Context, source v1beta1.ServiceStatus) error {
	source.Status.ConvertTo(ctx, &sink.Status)

	sink.RouteStatusFields.ConvertDown(ctx, source.RouteStatusFields)
	return sink.ConfigurationStatusFields.ConvertDown(ctx, source.ConfigurationStatusFields)
}
