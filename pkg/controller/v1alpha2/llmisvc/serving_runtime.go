/*
Copyright 2025 The KServe Authors.

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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// resolveRuntimeSpec looks up spec.runtime as a ServingRuntime in the LLMInferenceService's
// namespace first, then falls back to ClusterServingRuntime. When found, it returns a
// synthetic LLMInferenceServiceSpec whose Template.Containers carry the runtime's containers.
//
// This synthetic spec becomes the lowest-priority layer in the merge chain, so
// LLMInferenceServiceConfig baseRefs and the user's spec.template still override it.
// For now the runtime primarily contributes the container image; longer term the launch
// command and engine-specific args should also live here once the well-known infra
// templates are made engine-agnostic.
//
// Returns (nil, nil) when spec.runtime is nil/empty or the referenced runtime does not
// exist — falling back silently lets services opt out of the ServingRuntime layer without
// needing a special marker value.
func (r *LLMISVCReconciler) resolveRuntimeSpec(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*v1alpha2.LLMInferenceServiceSpec, error) {
	if llmSvc.Spec.Runtime == nil || *llmSvc.Spec.Runtime == "" {
		return nil, nil
	}
	name := *llmSvc.Spec.Runtime

	// Namespace-scoped ServingRuntime takes precedence over ClusterServingRuntime,
	// mirroring the InferenceService controller (see pkg/controller/v1beta1/inferenceservice/utils.GetServingRuntime).
	sr := &v1alpha1.ServingRuntime{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: llmSvc.Namespace}, sr); err == nil {
		return runtimeToSpec(sr.Spec.Containers), nil
	} else if !apierrors.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return nil, fmt.Errorf("failed to get ServingRuntime %s/%s: %w", llmSvc.Namespace, name, err)
	}

	csr := &v1alpha1.ClusterServingRuntime{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, csr); err == nil {
		return runtimeToSpec(csr.Spec.Containers), nil
	} else if !apierrors.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return nil, fmt.Errorf("failed to get ClusterServingRuntime %q: %w", name, err)
	}

	// Runtime not found: fall through silently. The controller's existing well-known configs
	// still provide a working image (kserve-config-llm-template) so the workload can start.
	return nil, nil
}

// runtimeToSpec wraps a ServingRuntime container list in the smallest LLMInferenceServiceSpec
// that the strategic-merge patch will treat as the container base. Only Template.Containers
// is populated; every other field is left at its zero value so this layer never blocks
// higher-precedence overrides.
func runtimeToSpec(containers []corev1.Container) *v1alpha2.LLMInferenceServiceSpec {
	if len(containers) == 0 {
		return nil
	}
	out := make([]corev1.Container, len(containers))
	for i := range containers {
		containers[i].DeepCopyInto(&out[i])
	}
	return &v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: out,
			},
		},
	}
}
