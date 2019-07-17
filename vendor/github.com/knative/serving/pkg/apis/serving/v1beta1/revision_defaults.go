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

	corev1 "k8s.io/api/core/v1"

	"github.com/knative/serving/pkg/apis/config"
)

// SetDefaults implements apis.Defaultable
func (r *Revision) SetDefaults(ctx context.Context) {
	r.Spec.SetDefaults(ctx)
}

// SetDefaults implements apis.Defaultable
func (rts *RevisionTemplateSpec) SetDefaults(ctx context.Context) {
	rts.Spec.SetDefaults(ctx)
}

// SetDefaults implements apis.Defaultable
func (rs *RevisionSpec) SetDefaults(ctx context.Context) {
	cfg := config.FromContextOrDefaults(ctx)

	// Default TimeoutSeconds based on our configmap
	if rs.TimeoutSeconds == nil {
		ts := cfg.Defaults.RevisionTimeoutSeconds
		rs.TimeoutSeconds = &ts
	}

	var container corev1.Container
	if len(rs.PodSpec.Containers) == 1 {
		container = rs.PodSpec.Containers[0]
	}
	defer func() {
		rs.PodSpec.Containers = []corev1.Container{container}
	}()

	if container.Name == "" {
		container.Name = cfg.Defaults.UserContainerName(ctx)
	}

	if container.Resources.Requests == nil {
		container.Resources.Requests = corev1.ResourceList{}
	}
	if _, ok := container.Resources.Requests[corev1.ResourceCPU]; !ok {
		if rsrc := cfg.Defaults.RevisionCPURequest; rsrc != nil {
			container.Resources.Requests[corev1.ResourceCPU] = *rsrc
		}
	}
	if _, ok := container.Resources.Requests[corev1.ResourceMemory]; !ok {
		if rsrc := cfg.Defaults.RevisionMemoryRequest; rsrc != nil {
			container.Resources.Requests[corev1.ResourceMemory] = *rsrc
		}
	}

	if container.Resources.Limits == nil {
		container.Resources.Limits = corev1.ResourceList{}
	}
	if _, ok := container.Resources.Limits[corev1.ResourceCPU]; !ok {
		if rsrc := cfg.Defaults.RevisionCPULimit; rsrc != nil {
			container.Resources.Limits[corev1.ResourceCPU] = *rsrc
		}
	}
	if _, ok := container.Resources.Limits[corev1.ResourceMemory]; !ok {
		if rsrc := cfg.Defaults.RevisionMemoryLimit; rsrc != nil {
			container.Resources.Limits[corev1.ResourceMemory] = *rsrc
		}
	}

	vms := container.VolumeMounts
	for i := range vms {
		vms[i].ReadOnly = true
	}
}
