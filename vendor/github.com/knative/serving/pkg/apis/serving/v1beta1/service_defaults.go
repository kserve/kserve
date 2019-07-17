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

	"k8s.io/apimachinery/pkg/api/equality"

	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving"
)

// SetDefaults implements apis.Defaultable
func (s *Service) SetDefaults(ctx context.Context) {
	ctx = apis.WithinParent(ctx, s.ObjectMeta)
	s.Spec.SetDefaults(apis.WithinSpec(ctx))

	if ui := apis.GetUserInfo(ctx); ui != nil {
		ans := s.GetAnnotations()
		if ans == nil {
			ans = map[string]string{}
			defer s.SetAnnotations(ans)
		}

		if apis.IsInUpdate(ctx) {
			old := apis.GetBaseline(ctx).(*Service)
			if equality.Semantic.DeepEqual(old.Spec, s.Spec) {
				return
			}
			ans[serving.UpdaterAnnotation] = ui.Username
		} else {
			ans[serving.CreatorAnnotation] = ui.Username
			ans[serving.UpdaterAnnotation] = ui.Username
		}
	}
}

// SetDefaults implements apis.Defaultable
func (ss *ServiceSpec) SetDefaults(ctx context.Context) {
	ss.ConfigurationSpec.SetDefaults(ctx)
	ss.RouteSpec.SetDefaults(WithDefaultConfigurationName(ctx))
}
