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

	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/ptr"
)

// SetDefaults implements apis.Defaultable
func (r *Route) SetDefaults(ctx context.Context) {
	r.Spec.SetDefaults(apis.WithinSpec(ctx))
}

// SetDefaults implements apis.Defaultable
func (rs *RouteSpec) SetDefaults(ctx context.Context) {
	if len(rs.Traffic) == 0 && HasDefaultConfigurationName(ctx) {
		rs.Traffic = []TrafficTarget{{
			Percent:        100,
			LatestRevision: ptr.Bool(true),
		}}
	}

	for idx := range rs.Traffic {
		rs.Traffic[idx].SetDefaults(ctx)
	}
}

// SetDefaults implements apis.Defaultable
func (tt *TrafficTarget) SetDefaults(ctx context.Context) {
	if tt.LatestRevision == nil {
		sense := (tt.RevisionName == "")
		tt.LatestRevision = &sense
	}
}
