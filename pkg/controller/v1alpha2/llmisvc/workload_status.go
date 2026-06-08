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

const (
	apiGroupApps = "apps"
	apiGroupLWS  = "leaderworkerset.x-k8s.io"
)

func lwsRef(name string) *corev1.TypedLocalObjectReference {
	return &corev1.TypedLocalObjectReference{APIGroup: ptr.To(apiGroupLWS), Kind: "LeaderWorkerSet", Name: name}
}

func deploymentRef(name string) *corev1.TypedLocalObjectReference {
	return &corev1.TypedLocalObjectReference{APIGroup: ptr.To(apiGroupApps), Kind: "Deployment", Name: name}
}

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
		ws.Primary = lwsRef(mainLWSName(llmSvc))
	} else {
		ws.Primary = deploymentRef(mainDeploymentName(llmSvc))
	}

	if llmSvc.Spec.Prefill != nil {
		if llmSvc.Spec.Prefill.Worker != nil {
			ws.Prefill = lwsRef(prefillLWSName(llmSvc))
		} else {
			ws.Prefill = deploymentRef(prefillDeploymentName(llmSvc))
		}
	}

	ws.Service = &corev1.TypedLocalObjectReference{
		Kind: "Service",
		Name: workloadServiceName(llmSvc),
	}

	if hasManagedScheduler(llmSvc) {
		ws.Scheduler = deploymentRef(schedulerDeploymentName(llmSvc))
	}

	llmSvc.Status.Workloads = ws
}
