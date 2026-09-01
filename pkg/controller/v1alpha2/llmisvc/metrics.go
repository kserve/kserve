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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	replicaMetricName   = "llmisvc_workload_replicas_ready"
	conditionMetricName = "llmisvc_condition_status"

	labelLLMIsvcName   = "llmisvc_name"
	labelNamespace     = "namespace"
	labelComponentType = "component_type"
	labelConditionType = "condition_type"
	labelStatus        = "status"
)

var (
	replicaDesc = prometheus.NewDesc(
		replicaMetricName,
		"Number of ready replicas for LLMInferenceService workload components (primary, prefill, scheduler)",
		[]string{labelLLMIsvcName, labelNamespace, labelComponentType},
		nil,
	)
	conditionDesc = prometheus.NewDesc(
		conditionMetricName,
		"Status of LLMInferenceService conditions. Value is 1 for the active status and 0 for the others (true, false, unknown).",
		[]string{labelLLMIsvcName, labelNamespace, labelConditionType, labelStatus},
		nil,
	)

	conditionStatuses = []string{"true", "false", "unknown"}
)

// WorkloadMetricsCollector implements prometheus.Collector to expose
// model-level operational metrics for LLMInferenceServices.
// Metrics are pulled on-demand during Prometheus scrape rather than pushed during reconciliation.
type WorkloadMetricsCollector struct {
	client client.Client
	mu     sync.Mutex
	last   []prometheus.Metric
}

// NewWorkloadMetricsCollector creates a new metrics collector for LLMInferenceService workloads.
func NewWorkloadMetricsCollector(c client.Client) *WorkloadMetricsCollector {
	return &WorkloadMetricsCollector{client: c}
}

// Describe implements prometheus.Collector.
func (c *WorkloadMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- replicaDesc
	ch <- conditionDesc
}

// Collect implements prometheus.Collector.
// Lists LLMInferenceServices from the controller-runtime cache and emits const metrics.
// A failed list re-emits the last successful scrape so series do not disappear.
func (c *WorkloadMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logger := log.FromContext(ctx).WithName("metrics")

	llmisvcs := &v1alpha2.LLMInferenceServiceList{}
	if err := c.client.List(ctx, llmisvcs); err != nil {
		logger.Error(err, "failed to list LLMInferenceServices for metrics collection")
		c.emitLast(ch)
		return
	}

	next := collectMetrics(llmisvcs.Items)
	c.mu.Lock()
	c.last = next
	c.mu.Unlock()

	for _, m := range next {
		ch <- m
	}
}

func (c *WorkloadMetricsCollector) emitLast(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	for _, m := range last {
		ch <- m
	}
}

func collectMetrics(items []v1alpha2.LLMInferenceService) []prometheus.Metric {
	out := make([]prometheus.Metric, 0, len(items)*6)
	for i := range items {
		isvc := &items[i]
		out = append(out, replicaMetrics(isvc)...)
		out = append(out, conditionMetrics(isvc)...)
	}
	return out
}

func replicaMetrics(isvc *v1alpha2.LLMInferenceService) []prometheus.Metric {
	if isvc.Status.Workloads == nil {
		return nil
	}

	var out []prometheus.Metric
	if isvc.Status.Workloads.Primary != nil {
		out = append(out, mustGauge(replicaDesc, readyReplicas(isvc.Status.Workloads.Primary),
			isvc.Name, isvc.Namespace, "primary"))
	}
	if isvc.Status.Workloads.Prefill != nil {
		out = append(out, mustGauge(replicaDesc, readyReplicas(isvc.Status.Workloads.Prefill),
			isvc.Name, isvc.Namespace, "prefill"))
	}
	if isvc.Status.Workloads.Scheduler != nil {
		out = append(out, mustGauge(replicaDesc, readyReplicas(isvc.Status.Workloads.Scheduler),
			isvc.Name, isvc.Namespace, "scheduler"))
	}
	return out
}

func conditionMetrics(isvc *v1alpha2.LLMInferenceService) []prometheus.Metric {
	out := make([]prometheus.Metric, 0, len(isvc.Status.Conditions)*len(conditionStatuses))
	for _, condition := range isvc.Status.Conditions {
		active, ok := conditionStatusValue(condition.Status)
		if !ok {
			continue
		}
		for _, status := range conditionStatuses {
			value := 0.0
			if status == active {
				value = 1
			}
			out = append(out, mustGauge(conditionDesc, value,
				isvc.Name, isvc.Namespace, string(condition.Type), status))
		}
	}
	return out
}

func conditionStatusValue(status corev1.ConditionStatus) (string, bool) {
	switch status {
	case corev1.ConditionTrue:
		return "true", true
	case corev1.ConditionFalse:
		return "false", true
	case corev1.ConditionUnknown:
		return "unknown", true
	default:
		return "", false
	}
}

func readyReplicas(w *v1alpha2.ObservedWorkloadStatus) float64 {
	if w == nil || w.ReadyReplicas == nil {
		return 0
	}
	return float64(*w.ReadyReplicas)
}

func mustGauge(desc *prometheus.Desc, value float64, labelValues ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
}
