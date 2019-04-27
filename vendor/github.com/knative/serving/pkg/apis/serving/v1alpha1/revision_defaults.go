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

	"github.com/knative/pkg/apis"
	corev1 "k8s.io/api/core/v1"
)

func (r *Revision) SetDefaults(ctx context.Context) {
	r.Spec.SetDefaults(apis.WithinSpec(ctx))
}

func (rs *RevisionSpec) SetDefaults(ctx context.Context) {
	// When ConcurrencyModel is specified but ContainerConcurrency
	// is not (0), use the ConcurrencyModel value.
	if rs.DeprecatedConcurrencyModel == RevisionRequestConcurrencyModelSingle && rs.ContainerConcurrency == 0 {
		rs.ContainerConcurrency = 1
	}

	// When the PodSpec has no containers, move the single Container
	// into the PodSpec for the scope of defaulting and then move
	// it back as we return.
	if len(rs.Containers) == 0 {
		if rs.Container == nil {
			rs.Container = &corev1.Container{}
		}
		rs.Containers = []corev1.Container{*rs.Container}
		defer func() {
			rs.Container = &rs.Containers[0]
			rs.Containers = nil
		}()
	}
	rs.RevisionSpec.SetDefaults(ctx)
}
