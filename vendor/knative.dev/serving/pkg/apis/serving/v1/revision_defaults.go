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

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	"knative.dev/serving/pkg/apis/config"
)

// SetDefaults implements apis.Defaultable
func (r *Revision) SetDefaults(ctx context.Context) {
	r.Spec.SetDefaults(apis.WithinSpec(ctx))
}

// SetDefaults implements apis.Defaultable
func (rts *RevisionTemplateSpec) SetDefaults(ctx context.Context) {
	rts.Spec.SetDefaults(apis.WithinSpec(ctx))
}

// SetDefaults implements apis.Defaultable
func (rs *RevisionSpec) SetDefaults(ctx context.Context) {
	cfg := config.FromContextOrDefaults(ctx)

	// Default TimeoutSeconds based on our configmap
	if rs.TimeoutSeconds == nil || *rs.TimeoutSeconds == 0 {
		rs.TimeoutSeconds = ptr.Int64(cfg.Defaults.RevisionTimeoutSeconds)
	}

	// Default ContainerConcurrency based on our configmap
	if rs.ContainerConcurrency == nil {
		rs.ContainerConcurrency = ptr.Int64(cfg.Defaults.ContainerConcurrency)
	}

	for idx := range rs.PodSpec.Containers {
		if rs.PodSpec.Containers[idx].Name == "" {
			rs.PodSpec.Containers[idx].Name = cfg.Defaults.UserContainerName(ctx)
		}

		if rs.PodSpec.Containers[idx].Resources.Requests == nil {
			rs.PodSpec.Containers[idx].Resources.Requests = corev1.ResourceList{}
		}
		if _, ok := rs.PodSpec.Containers[idx].Resources.Requests[corev1.ResourceCPU]; !ok {
			if rsrc := cfg.Defaults.RevisionCPURequest; rsrc != nil {
				rs.PodSpec.Containers[idx].Resources.Requests[corev1.ResourceCPU] = *rsrc
			}
		}
		if _, ok := rs.PodSpec.Containers[idx].Resources.Requests[corev1.ResourceMemory]; !ok {
			if rsrc := cfg.Defaults.RevisionMemoryRequest; rsrc != nil {
				rs.PodSpec.Containers[idx].Resources.Requests[corev1.ResourceMemory] = *rsrc
			}
		}

		if rs.PodSpec.Containers[idx].Resources.Limits == nil {
			rs.PodSpec.Containers[idx].Resources.Limits = corev1.ResourceList{}
		}
		if _, ok := rs.PodSpec.Containers[idx].Resources.Limits[corev1.ResourceCPU]; !ok {
			if rsrc := cfg.Defaults.RevisionCPULimit; rsrc != nil {
				rs.PodSpec.Containers[idx].Resources.Limits[corev1.ResourceCPU] = *rsrc
			}
		}
		if _, ok := rs.PodSpec.Containers[idx].Resources.Limits[corev1.ResourceMemory]; !ok {
			if rsrc := cfg.Defaults.RevisionMemoryLimit; rsrc != nil {
				rs.PodSpec.Containers[idx].Resources.Limits[corev1.ResourceMemory] = *rsrc
			}
		}
		if rs.PodSpec.Containers[idx].ReadinessProbe == nil {
			rs.PodSpec.Containers[idx].ReadinessProbe = &corev1.Probe{}
		}
		if rs.PodSpec.Containers[idx].ReadinessProbe.TCPSocket == nil &&
			rs.PodSpec.Containers[idx].ReadinessProbe.HTTPGet == nil &&
			rs.PodSpec.Containers[idx].ReadinessProbe.Exec == nil {
			rs.PodSpec.Containers[idx].ReadinessProbe.TCPSocket = &corev1.TCPSocketAction{}
		}

		if rs.PodSpec.Containers[idx].ReadinessProbe.SuccessThreshold == 0 {
			rs.PodSpec.Containers[idx].ReadinessProbe.SuccessThreshold = 1
		}

		vms := rs.PodSpec.Containers[idx].VolumeMounts
		for i := range vms {
			vms[i].ReadOnly = true
		}
	}
}
