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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// observeWorkloadStatus populates status.workloads with references to the
// workload resources created during this reconciliation. It uses the same
// deterministic naming functions the reconciler uses to create the resources,
// so refs are set without additional API calls.
//
// This function must only be called after reconcileWorkload and
// reconcileRouter return without error, which guarantees the named
// resources exist on the API server.
func observeWorkloadStatus(llmSvc *v1alpha2.LLMInferenceService) {
	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.Status.Workloads = nil
		return
	}

	ws := &v1alpha2.WorkloadStatus{}

	if llmSvc.Spec.Worker != nil {
		ws.Primary = &corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
			Kind:     "LeaderWorkerSet",
			Name:     mainLWSName(llmSvc),
		}
		if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Worker != nil {
			ws.Prefill = &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
				Kind:     "LeaderWorkerSet",
				Name:     prefillLWSName(llmSvc),
			}
		}
	} else {
		ws.Primary = &corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("apps"),
			Kind:     "Deployment",
			Name:     mainDeploymentName(llmSvc),
		}
		if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Worker == nil {
			ws.Prefill = &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To("apps"),
				Kind:     "Deployment",
				Name:     prefillDeploymentName(llmSvc),
			}
		}
	}

	ws.Service = &corev1.TypedLocalObjectReference{
		Kind: "Service",
		Name: workloadServiceName(llmSvc),
	}

	if hasManagedScheduler(llmSvc) {
		ws.Scheduler = &corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("apps"),
			Kind:     "Deployment",
			Name:     schedulerDeploymentName(llmSvc),
		}
	}

	llmSvc.Status.Workloads = ws
}

// hasManagedScheduler returns true when the spec defines a managed scheduler
// deployment (template present, no external pool ref). Mirrors the predicate
// in reconcileSchedulerDeployment.
func hasManagedScheduler(llmSvc *v1alpha2.LLMInferenceService) bool {
	return llmSvc.Spec.Router != nil &&
		llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Template != nil &&
		!llmSvc.Spec.Router.Scheduler.Pool.HasRef()
}
