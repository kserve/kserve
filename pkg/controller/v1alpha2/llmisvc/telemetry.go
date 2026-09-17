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
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	acceleratorCPU = "cpu"
	acceleratorGPU = "gpu"
)

var llmInferenceServiceInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "kserve_llminferenceservice_info",
		Help: "Info metric (value=1) for each active LLMInferenceService, labeled by accelerator type and model.",
	},
	[]string{"namespace", "name", "accelerator", "model_name", "model_uri"},
)

func init() {
	metrics.Registry.MustRegister(llmInferenceServiceInfo)
}

// recordLLMInferenceServiceInfo sets the info metric for an active LLMInferenceService.
// It deletes any previous series for this service first to avoid stale metrics
// when labels change (e.g., model URI update).
func recordLLMInferenceServiceInfo(llmSvc *v1alpha2.LLMInferenceService) {
	accelerator := resolveAccelerator(llmSvc.Spec.Template)
	modelName := resolveModelName(llmSvc)
	modelURI := llmSvc.Spec.Model.URI.String()

	llmInferenceServiceInfo.DeletePartialMatch(prometheus.Labels{
		"namespace": llmSvc.Namespace,
		"name":      llmSvc.Name,
	})
	llmInferenceServiceInfo.With(prometheus.Labels{
		"namespace":   llmSvc.Namespace,
		"name":        llmSvc.Name,
		"accelerator": accelerator,
		"model_name":  modelName,
		"model_uri":   modelURI,
	}).Set(1)
}

// deleteLLMInferenceServiceInfo removes the info metric for a deleted LLMInferenceService.
func deleteLLMInferenceServiceInfo(llmSvc *v1alpha2.LLMInferenceService) {
	llmInferenceServiceInfo.DeletePartialMatch(prometheus.Labels{
		"namespace": llmSvc.Namespace,
		"name":      llmSvc.Name,
	})
}

// resolveAccelerator inspects the pod template for GPU resource requests/limits.
// Returns "gpu" if any container requests a known GPU resource, "cpu" otherwise.
func resolveAccelerator(podSpec *corev1.PodSpec) string {
	if podSpec == nil {
		return acceleratorCPU
	}
	for i := range podSpec.Containers {
		if hasGPUResources(&podSpec.Containers[i]) {
			return acceleratorGPU
		}
	}
	return acceleratorCPU
}

var gpuResourcePrefixes = []string{
	"nvidia.com/gpu",
	"amd.com/gpu",
	"intel.com/gpu",
	"gpu.intel.com/i915",
	"gpu.intel.com/xe",
	"habana.ai/gaudi",
}

func hasGPUResources(container *corev1.Container) bool {
	for name := range container.Resources.Requests {
		if isGPUResource(string(name)) {
			return true
		}
	}
	for name := range container.Resources.Limits {
		if isGPUResource(string(name)) {
			return true
		}
	}
	return false
}

func isGPUResource(name string) bool {
	for _, prefix := range gpuResourcePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func resolveModelName(llmSvc *v1alpha2.LLMInferenceService) string {
	if llmSvc.Spec.Model.Name != nil && *llmSvc.Spec.Model.Name != "" {
		return *llmSvc.Spec.Model.Name
	}
	return llmSvc.Name
}
