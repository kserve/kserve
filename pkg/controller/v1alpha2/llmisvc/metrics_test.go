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

package llmisvc_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

func TestWorkloadMetricsCollector_ReplicaCounts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	tests := []struct {
		name           string
		llmisvcs       []v1alpha2.LLMInferenceService
		expectedMetric map[string]map[string]float64 // component_type -> llmisvc_name -> value
	}{
		{
			name: "single llmisvc with primary workload",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-isvc",
						Namespace: "test-ns",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Workloads: &v1alpha2.WorkloadStatus{
							Primary: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(2)),
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]float64{
				"primary": {"test-isvc": 2},
			},
		},
		{
			name: "llmisvc with disaggregated serving",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "disagg-isvc",
						Namespace: "test-ns",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Workloads: &v1alpha2.WorkloadStatus{
							Primary: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(3)),
							},
							Prefill: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(1)),
							},
							Scheduler: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(1)),
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]float64{
				"primary":   {"disagg-isvc": 3},
				"prefill":   {"disagg-isvc": 1},
				"scheduler": {"disagg-isvc": 1},
			},
		},
		{
			name: "multiple llmisvcs",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "isvc-1",
						Namespace: "ns-1",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Workloads: &v1alpha2.WorkloadStatus{
							Primary: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(1)),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "isvc-2",
						Namespace: "ns-2",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Workloads: &v1alpha2.WorkloadStatus{
							Primary: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: ptr.To(int32(5)),
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]float64{
				"primary": {"isvc-1": 1, "isvc-2": 5},
			},
		},
		{
			name: "nil readyReplicas defaults to 0",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nil-replicas",
						Namespace: "test-ns",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Workloads: &v1alpha2.WorkloadStatus{
							Primary: &v1alpha2.ObservedWorkloadStatus{
								ReadyReplicas: nil,
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]float64{
				"primary": {"nil-replicas": 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test data
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			for i := range tt.llmisvcs {
				clientBuilder.WithObjects(&tt.llmisvcs[i])
			}
			client := clientBuilder.Build()

			// Create collector and collect metrics
			collector := llmisvc.NewWorkloadMetricsCollector(client)
			metricCh := make(chan prometheus.Metric, 100)
			collector.Collect(metricCh)
			close(metricCh)

			// Parse collected metrics
			collectedMetrics := make(map[string]map[string]float64)
			for metric := range metricCh {
				var m dto.Metric
				if err := metric.Write(&m); err != nil {
					t.Fatalf("failed to write metric: %v", err)
				}

				// Extract labels
				var componentType, llmisvName string
				for _, label := range m.GetLabel() {
					if label.GetName() == "component_type" {
						componentType = label.GetValue()
					}
					if label.GetName() == "llmisvc_name" {
						llmisvName = label.GetValue()
					}
				}

				if componentType == "" || llmisvName == "" {
					continue
				}

				if collectedMetrics[componentType] == nil {
					collectedMetrics[componentType] = make(map[string]float64)
				}
				collectedMetrics[componentType][llmisvName] = m.GetGauge().GetValue()
			}

			// Verify expected metrics
			for componentType, expected := range tt.expectedMetric {
				for llmisvName, expectedValue := range expected {
					actualValue, found := collectedMetrics[componentType][llmisvName]
					if !found {
						t.Errorf("missing metric for component_type=%s llmisvc_name=%s", componentType, llmisvName)
						continue
					}
					if actualValue != expectedValue {
						t.Errorf("component_type=%s llmisvc_name=%s: got %v, want %v",
							componentType, llmisvName, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestWorkloadMetricsCollector_ConditionStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	tests := []struct {
		name           string
		llmisvcs       []v1alpha2.LLMInferenceService
		expectedMetric map[string]map[string]bool // condition_type:status -> llmisvc_name -> present
	}{
		{
			name: "all conditions true",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ready-isvc",
						Namespace: "test-ns",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Status: duckv1.Status{
							Conditions: duckv1.Conditions{
								{
									Type:   apis.ConditionReady,
									Status: corev1.ConditionTrue,
								},
								{
									Type:   v1alpha2.WorkloadReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]bool{
				string(apis.ConditionReady) + ":true":    {"ready-isvc": true},
				string(v1alpha2.WorkloadReady) + ":true": {"ready-isvc": true},
			},
		},
		{
			name: "mixed condition states",
			llmisvcs: []v1alpha2.LLMInferenceService{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mixed-isvc",
						Namespace: "test-ns",
					},
					Status: v1alpha2.LLMInferenceServiceStatus{
						Status: duckv1.Status{
							Conditions: duckv1.Conditions{
								{
									Type:   apis.ConditionReady,
									Status: corev1.ConditionFalse,
								},
								{
									Type:   v1alpha2.WorkloadReady,
									Status: corev1.ConditionTrue,
								},
								{
									Type:   v1alpha2.RouterReady,
									Status: corev1.ConditionUnknown,
								},
							},
						},
					},
				},
			},
			expectedMetric: map[string]map[string]bool{
				string(apis.ConditionReady) + ":false":    {"mixed-isvc": true},
				string(v1alpha2.WorkloadReady) + ":true":  {"mixed-isvc": true},
				string(v1alpha2.RouterReady) + ":unknown": {"mixed-isvc": true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test data
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			for i := range tt.llmisvcs {
				clientBuilder.WithObjects(&tt.llmisvcs[i])
			}
			client := clientBuilder.Build()

			// Create collector and collect metrics
			collector := llmisvc.NewWorkloadMetricsCollector(client)
			metricCh := make(chan prometheus.Metric, 100)
			collector.Collect(metricCh)
			close(metricCh)

			// Parse collected metrics
			collectedMetrics := make(map[string]map[string]bool)
			for metric := range metricCh {
				var m dto.Metric
				if err := metric.Write(&m); err != nil {
					t.Fatalf("failed to write metric: %v", err)
				}

				// Extract labels
				var conditionType, llmisvName, status string
				for _, label := range m.GetLabel() {
					switch label.GetName() {
					case "condition_type":
						conditionType = label.GetValue()
					case "llmisvc_name":
						llmisvName = label.GetValue()
					case "status":
						status = label.GetValue()
					}
				}

				if conditionType == "" || llmisvName == "" || status == "" {
					continue
				}

				key := conditionType + ":" + status
				if collectedMetrics[key] == nil {
					collectedMetrics[key] = make(map[string]bool)
				}
				collectedMetrics[key][llmisvName] = true
			}

			// Verify expected metrics
			for key, expected := range tt.expectedMetric {
				for llmisvName := range expected {
					if !collectedMetrics[key][llmisvName] {
						t.Errorf("missing metric for %s llmisvc_name=%s", key, llmisvName)
					}
				}
			}
		})
	}
}

func TestWorkloadMetricsCollector_CardinalityControl(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	// Create 100 LLMISVCs to test cardinality scaling
	llmisvcs := make([]v1alpha2.LLMInferenceService, 100)
	for i := range 100 {
		llmisvcs[i] = v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "isvc-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
				Namespace: "ns-" + string(rune('a'+i%10)),
			},
			Status: v1alpha2.LLMInferenceServiceStatus{
				Workloads: &v1alpha2.WorkloadStatus{
					Primary: &v1alpha2.ObservedWorkloadStatus{
						ReadyReplicas: ptr.To(int32(i % 5)), //nolint:gosec // Test data, no overflow risk
					},
				},
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   apis.ConditionReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		}
	}

	// Create fake client
	clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range llmisvcs {
		clientBuilder.WithObjects(&llmisvcs[i])
	}
	client := clientBuilder.Build()

	// Collect metrics
	collector := llmisvc.NewWorkloadMetricsCollector(client)
	metricCh := make(chan prometheus.Metric, 1000)
	collector.Collect(metricCh)
	close(metricCh)

	// Count total series
	totalSeries := 0
	for range metricCh {
		totalSeries++
	}

	t.Logf("Total metric series for 100 LLMISVCs: %d", totalSeries)

	// Cardinality scales linearly with the number of LLMISVCs.
	// With 100 ISVCs (primary workload + Ready condition, no scheduler):
	// - 100 primary replica series
	// - 300 condition series (true/false/unknown for Ready)
	// = 400 total series
	expectedSeries := 400
	if totalSeries != expectedSeries {
		t.Errorf("cardinality mismatch: got %d series, want %d (scaling: %d ISVCs × 4 series/ISVC)",
			totalSeries, expectedSeries, len(llmisvcs))
	}
}

func TestWorkloadMetricsCollector_ConcurrentCollection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	// Create test data
	llmisvcs := []v1alpha2.LLMInferenceService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-ns",
			},
			Status: v1alpha2.LLMInferenceServiceStatus{
				Workloads: &v1alpha2.WorkloadStatus{
					Primary: &v1alpha2.ObservedWorkloadStatus{
						ReadyReplicas: ptr.To(int32(2)),
					},
				},
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   apis.ConditionReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
	}

	// Create fake client
	clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range llmisvcs {
		clientBuilder.WithObjects(&llmisvcs[i])
	}
	client := clientBuilder.Build()

	// Create collector
	collector := llmisvc.NewWorkloadMetricsCollector(client)

	// Concurrent collection should not panic or produce corrupted data
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			metricCh := make(chan prometheus.Metric, 100)
			collector.Collect(metricCh)
			close(metricCh)

			// Verify we got metrics
			count := 0
			for range metricCh {
				count++
			}
			if count == 0 {
				t.Error("no metrics collected")
			}
		}()
	}

	wg.Wait()
	// If we get here without panic, concurrency is safe
}

func TestWorkloadMetricsCollector_InactiveConditionStatusesAreZero(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	isvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "ready-isvc", Namespace: "test-ns"},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{Type: apis.ConditionReady, Status: corev1.ConditionTrue},
				},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(isvc).Build()
	collector := llmisvc.NewWorkloadMetricsCollector(client)

	got := map[string]float64{}
	for _, m := range collectAll(t, collector) {
		if label(m, "condition_type") != string(apis.ConditionReady) {
			continue
		}
		got[label(m, "status")] = m.GetGauge().GetValue()
	}

	if got["true"] != 1 {
		t.Errorf("status=true: got %v, want 1", got["true"])
	}
	if got["false"] != 0 {
		t.Errorf("status=false: got %v, want 0", got["false"])
	}
	if got["unknown"] != 0 {
		t.Errorf("status=unknown: got %v, want 0", got["unknown"])
	}
}

func TestWorkloadMetricsCollector_ListFailureKeepsLastScrape(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	isvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-isvc", Namespace: "test-ns"},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Workloads: &v1alpha2.WorkloadStatus{
				Primary: &v1alpha2.ObservedWorkloadStatus{
					ReadyReplicas: ptr.To(int32(2)),
				},
			},
		},
	}
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(isvc).Build()
	fc := &failingListClient{Client: inner}
	collector := llmisvc.NewWorkloadMetricsCollector(fc)

	first := collectAll(t, collector)
	if len(first) == 0 {
		t.Fatal("expected metrics from successful list")
	}

	fc.fail = true
	second := collectAll(t, collector)
	if len(second) != len(first) {
		t.Fatalf("list failure dropped series: got %d, want %d", len(second), len(first))
	}
}

func TestWorkloadMetricsCollector_RemovesStaleSeries(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kservescheme.AddLLMISVCAPIs(scheme)

	isvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "stale-isvc", Namespace: "test-ns"},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Workloads: &v1alpha2.WorkloadStatus{
				Primary: &v1alpha2.ObservedWorkloadStatus{
					ReadyReplicas: ptr.To(int32(1)),
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(isvc).Build()
	collector := llmisvc.NewWorkloadMetricsCollector(c)

	if got := len(collectAll(t, collector)); got == 0 {
		t.Fatal("expected metrics before delete")
	}

	if err := c.Delete(context.Background(), isvc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if got := len(collectAll(t, collector)); got != 0 {
		t.Errorf("stale series after delete: got %d, want 0", got)
	}
}

type failingListClient struct {
	client.Client
	fail bool
}

func (f *failingListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.fail {
		return errors.New("list failed")
	}
	return f.Client.List(ctx, list, opts...)
}

func collectAll(t *testing.T, collector *llmisvc.WorkloadMetricsCollector) []*dto.Metric {
	t.Helper()
	metricCh := make(chan prometheus.Metric, 1000)
	collector.Collect(metricCh)
	close(metricCh)

	var out []*dto.Metric
	for metric := range metricCh {
		m := &dto.Metric{}
		if err := metric.Write(m); err != nil {
			t.Fatalf("failed to write metric: %v", err)
		}
		out = append(out, m)
	}
	return out
}

func label(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}
