/*
Copyright 2026 The KServe Authors.

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

package llmisvc

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// Multiple target ports cannot be represented by the legacy pool's single
// targetPortNumber. Such pools must never fall back to the legacy API.
func inferencePoolRequiresV1(spec *v1alpha2.LLMInferenceServiceSpec) bool {
	return spec.Router != nil && spec.Router.Scheduler != nil &&
		spec.Router.Scheduler.Pool != nil && spec.Router.Scheduler.Pool.Spec != nil &&
		len(spec.Router.Scheduler.Pool.Spec.TargetPorts) > 1
}

// Select the pool watched by the sidecar after merging presets and expanding DP
// ports. This also covers asymmetric P/D, where only prefill has multiple ranks.
// Explicit pool refs are validated as v1 resources by the router.
func applyInferencePoolSidecarAPI(spec *v1alpha2.LLMInferenceServiceSpec) {
	referencedPool := spec.Router != nil && spec.Router.Scheduler != nil && spec.Router.Scheduler.Pool.HasRef()
	if !inferencePoolRequiresV1(spec) && !referencedPool {
		return
	}
	for _, workload := range []*v1alpha2.WorkloadSpec{&spec.WorkloadSpec, spec.Prefill} {
		if workload == nil {
			continue
		}
		for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
			if sidecar := routingSidecar(pod); sidecar != nil {
				for _, args := range [][]string{sidecar.Command, sidecar.Args} {
					for i, arg := range args {
						if arg == "--pool-group="+constants.InferencePoolV1Alpha2APIGroupName {
							args[i] = "--pool-group=" + constants.InferencePoolV1APIGroupName
						} else if arg == "--pool-group" && i+1 < len(args) && args[i+1] == constants.InferencePoolV1Alpha2APIGroupName {
							args[i+1] = constants.InferencePoolV1APIGroupName
						}
					}
				}
			}
		}
	}
}
