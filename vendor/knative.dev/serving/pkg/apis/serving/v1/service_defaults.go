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

package v1

import (
	"context"

	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/serving"
)

// SetDefaults implements apis.Defaultable
func (s *Service) SetDefaults(ctx context.Context) {
	ctx = apis.WithinParent(ctx, s.ObjectMeta)
	s.Spec.SetDefaults(apis.WithinSpec(ctx))

	if apis.IsInUpdate(ctx) {
		serving.SetUserInfo(ctx, apis.GetBaseline(ctx).(*Service).Spec, s.Spec, s)
	} else {
		serving.SetUserInfo(ctx, nil, s.Spec, s)
	}
}

// SetDefaults implements apis.Defaultable
func (ss *ServiceSpec) SetDefaults(ctx context.Context) {
	ss.ConfigurationSpec.SetDefaults(ctx)
	ss.RouteSpec.SetDefaults(WithDefaultConfigurationName(ctx))
}
