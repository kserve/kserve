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
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	acceleratorCPU = "cpu"
	acceleratorGPU = "gpu"
)

var infoDesc = prometheus.NewDesc(
	"kserve_llminferenceservice_info",
	"Info metric (value=1) for each active LLMInferenceService, labeled by accelerator type and model.",
	[]string{"namespace", "name", "accelerator", "model_name", "model_uri"},
	nil,
)

// InfoMetricsCollector implements prometheus.Collector to expose
// deployment-inventory metrics for LLMInferenceServices.
// Metrics are pulled on-demand during Prometheus scrape rather than pushed during reconciliation.
type InfoMetricsCollector struct {
	client client.Client
	mu     sync.Mutex
	last   []prometheus.Metric
}

// NewInfoMetricsCollector creates a new collector for LLMInferenceService info metrics.
func NewInfoMetricsCollector(c client.Client) *InfoMetricsCollector {
	return &InfoMetricsCollector{client: c}
}

// Describe implements prometheus.Collector.
func (c *InfoMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- infoDesc
}

// Collect implements prometheus.Collector.
// Lists LLMInferenceServices from the controller-runtime cache and emits const metrics.
// A failed list re-emits the last successful scrape so series do not disappear.
func (c *InfoMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logger := log.FromContext(ctx).WithName("metrics")

	llmisvcs := &v1alpha2.LLMInferenceServiceList{}
	if err := c.client.List(ctx, llmisvcs); err != nil {
		logger.Error(err, "failed to list LLMInferenceServices for info metrics collection")
		c.emitLast(ch)
		return
	}

	next := collectInfoMetrics(llmisvcs.Items)
	c.mu.Lock()
	c.last = next
	c.mu.Unlock()

	for _, m := range next {
		ch <- m
	}
}

func (c *InfoMetricsCollector) emitLast(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	for _, m := range last {
		ch <- m
	}
}

func collectInfoMetrics(items []v1alpha2.LLMInferenceService) []prometheus.Metric {
	out := make([]prometheus.Metric, 0, len(items))
	for i := range items {
		isvc := &items[i]
		out = append(out, prometheus.MustNewConstMetric(
			infoDesc,
			prometheus.GaugeValue,
			1,
			isvc.Namespace,
			isvc.Name,
			resolveAccelerator(isvc.Spec.Template),
			resolveModelName(isvc),
			isvc.Spec.Model.URI.String(),
		))
	}
	return out
}

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
