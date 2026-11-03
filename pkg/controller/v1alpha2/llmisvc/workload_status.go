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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	apiGroupApps = "apps"
	apiGroupLWS  = "leaderworkerset.x-k8s.io"
)

func observedDeployment(name string) *v1alpha2.ObservedWorkloadStatus {
	return &v1alpha2.ObservedWorkloadStatus{
		TypedLocalObjectReference: corev1.TypedLocalObjectReference{
			APIGroup: ptr.To(apiGroupApps), Kind: "Deployment", Name: name,
		},
	}
}

func observedLWS(name string) *v1alpha2.ObservedWorkloadStatus {
	return &v1alpha2.ObservedWorkloadStatus{
		TypedLocalObjectReference: corev1.TypedLocalObjectReference{
			APIGroup: ptr.To(apiGroupLWS), Kind: "LeaderWorkerSet", Name: name,
		},
	}
}

// observeWorkloadStatus populates status.workloads with references to the
// workload resources and their observed replica counts.
//
// This function must only be called after reconcileWorkload and
// reconcileRouter return without error, which guarantees the named
// resources exist on the API server.
func (r *LLMISVCReconciler) observeWorkloadStatus(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.Status.Workloads = nil
		return nil
	}

	ws := &v1alpha2.WorkloadStatus{}

	if llmSvc.Spec.Worker != nil {
		ws.Primary = observedLWS(mainLWSName(llmSvc))
	} else {
		ws.Primary = observedDeployment(mainDeploymentName(llmSvc))
	}

	if llmSvc.Spec.Prefill != nil {
		if llmSvc.Spec.Prefill.Worker != nil {
			ws.Prefill = observedLWS(prefillLWSName(llmSvc))
		} else {
			ws.Prefill = observedDeployment(prefillDeploymentName(llmSvc))
		}
	}

	ws.Service = &corev1.TypedLocalObjectReference{
		Kind: "Service",
		Name: workloadServiceName(llmSvc),
	}

	if hasManagedScheduler(llmSvc) {
		ws.Scheduler = observedDeployment(schedulerDeploymentName(llmSvc))
	}

	llmSvc.Status.Workloads = ws

	for _, w := range []*v1alpha2.ObservedWorkloadStatus{ws.Primary, ws.Prefill, ws.Scheduler} {
		if w == nil {
			continue
		}
		if w.Kind == "LeaderWorkerSet" {
			if err := r.observeLWSReplicas(ctx, llmSvc, w); err != nil {
				return err
			}
		} else {
			if err := r.observeDeploymentReplicas(ctx, llmSvc, w); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *LLMISVCReconciler) observeDeploymentReplicas(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, obs *v1alpha2.ObservedWorkloadStatus) error {
	deploy := &appsv1.Deployment{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKey{Namespace: llmSvc.Namespace, Name: obs.Name}, deploy)
	})
	if err != nil {
		return fmt.Errorf("failed to read deployment %s for replica count: %w", obs.Name, err)
	}
	obs.ReadyReplicas = ptr.To(deploy.Status.AvailableReplicas)
	return nil
}

func (r *LLMISVCReconciler) observeLWSReplicas(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, obs *v1alpha2.ObservedWorkloadStatus) error {
	lws := &lwsapi.LeaderWorkerSet{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKey{Namespace: llmSvc.Namespace, Name: obs.Name}, lws)
	})
	if err != nil {
		return fmt.Errorf("failed to read LWS %s for replica count: %w", obs.Name, err)
	}
	obs.ReadyReplicas = ptr.To(lws.Status.ReadyReplicas)
	return nil
}
