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

package otel

import (
	"testing"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"

	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateOtelCollector(t *testing.T) {
	// Using assert library instead of gomega

	testCases := []struct {
		name           string
		componentMeta  metav1.ObjectMeta
		metric         v1beta1.MetricsSpec
		otelConfig     v1beta1.OtelCollectorConfig
		expectedConfig map[string]interface{}
	}{
		{
			name: "test with port annotation",
			componentMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					"prometheus.kserve.io/port": "9090",
				},
			},
			metric: v1beta1.MetricsSpec{
				PodMetric: &v1beta1.PodMetricSource{
					Metric: v1beta1.PodMetrics{
						MetricNames: []string{"request-count"},
					},
				},
			},
			otelConfig: v1beta1.OtelCollectorConfig{
				ScrapeInterval:         "15s",
				MetricReceiverEndpoint: "otel-collector:4317",
			},
			expectedConfig: map[string]interface{}{
				"job_name":        "otel-collector",
				"scrape_interval": "15s",
				"static_configs": []interface{}{
					map[string]interface{}{
						"targets": []interface{}{"localhost:9090"},
					},
				},
			},
		},
		{
			name: "test without port annotation",
			componentMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			metric: v1beta1.MetricsSpec{
				PodMetric: &v1beta1.PodMetricSource{
					Metric: v1beta1.PodMetrics{
						MetricNames: []string{"request-count"},
					},
				},
			},
			otelConfig: v1beta1.OtelCollectorConfig{
				ScrapeInterval: "30s",
			},
			expectedConfig: map[string]interface{}{
				"job_name":        "otel-collector",
				"scrape_interval": "30s",
				"static_configs": []interface{}{
					map[string]interface{}{
						"targets": []interface{}{"localhost:8080"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collector := createOtelCollector(tc.componentMeta, tc.metric, tc.otelConfig)

			assert.Equal(t, tc.componentMeta.Name, collector.Name)
			assert.Equal(t, tc.componentMeta.Namespace, collector.Namespace)
			assert.Equal(t, otelv1beta1.ModeSidecar, collector.Spec.Mode)

			// Assert config details
			receivers := collector.Spec.Config.Receivers.Object
			prometheusConfig := receivers["prometheus"].(map[string]interface{})
			config := prometheusConfig["config"].(map[string]interface{})
			scrapeConfigs := config["scrape_configs"].([]interface{})
			scrapeConfig := scrapeConfigs[0].(map[string]interface{})

			assert.Equal(t, tc.expectedConfig["job_name"], scrapeConfig["job_name"])
			assert.Equal(t, tc.expectedConfig["scrape_interval"], scrapeConfig["scrape_interval"])

			staticConfigs := scrapeConfig["static_configs"].([]interface{})
			staticConfig := staticConfigs[0].(map[string]interface{})
			targets := staticConfig["targets"].([]interface{})

			assert.Equal(t, tc.expectedConfig["static_configs"].([]interface{})[0].(map[string]interface{})["targets"], targets)

			// Verify filter processor if metric names exist
			if len(tc.metric.PodMetric.Metric.MetricNames) > 0 {
				processors := collector.Spec.Config.Processors.Object
				filterOttl := processors["filter/ottl"].(map[string]interface{})
				metrics := filterOttl["metrics"].(map[string]interface{})
				metricFilters := metrics["metric"].([]interface{})

				assert.Len(t, metricFilters, len(tc.metric.PodMetric.Metric.MetricNames))
				// Verify processors in pipeline
				assert.Equal(t, []string{"filter/ottl"}, collector.Spec.Config.Service.Pipelines["metrics"].Processors)
			} else {
				assert.Empty(t, collector.Spec.Config.Service.Pipelines["metrics"].Processors)
			}
		})
	}
}

func TestReconcileCreate(t *testing.T) {
	// Using assert library instead of gomega

	// Setup test data
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
	}

	metric := v1beta1.MetricsSpec{
		PodMetric: &v1beta1.PodMetricSource{
			Metric: v1beta1.PodMetrics{},
		},
	}

	otelConfig := v1beta1.OtelCollectorConfig{
		ScrapeInterval:         "15s",
		MetricReceiverEndpoint: "otel-collector:4317",
	}

	// Create the test scheme
	scheme := runtime.NewScheme()
	_ = otelv1beta1.AddToScheme(scheme)

	// Create fake client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	// Create reconciler
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, metric, otelConfig)
	require.NoError(t, err)

	// Test reconcile - should create a new resource
	err = reconciler.Reconcile(t.Context())
	require.NoError(t, err)
	assert.NoError(t, err)
	// Verify collector was created
	collector := &otelv1beta1.OpenTelemetryCollector{}
	err = client.Get(t.Context(), types.NamespacedName{Name: componentMeta.Name, Namespace: componentMeta.Namespace}, collector)
	require.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, componentMeta.Name, collector.Name)
	assert.Equal(t, componentMeta.Namespace, collector.Namespace)
}

func TestReconcileUpdate(t *testing.T) {
	// Using assert library instead of gomega

	// Setup test data
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
	}

	metric := v1beta1.MetricsSpec{
		PodMetric: &v1beta1.PodMetricSource{
			Metric: v1beta1.PodMetrics{},
		},
	}

	otelConfig := v1beta1.OtelCollectorConfig{
		ScrapeInterval:         "15s",
		MetricReceiverEndpoint: "otel-collector:4317",
	}

	// Create the test scheme
	scheme := runtime.NewScheme()
	_ = otelv1beta1.AddToScheme(scheme)

	// Create existing collector with different config
	existingCollector := &otelv1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentMeta.Name,
			Namespace: componentMeta.Namespace,
		},
		Spec: otelv1beta1.OpenTelemetryCollectorSpec{
			Mode: otelv1beta1.ModeSidecar,
			Config: otelv1beta1.Config{
				Receivers: otelv1beta1.AnyConfig{Object: map[string]interface{}{
					"prometheus": map[string]interface{}{
						"config": map[string]interface{}{
							"scrape_configs": []interface{}{
								map[string]interface{}{
									"job_name":        "old-collector",
									"scrape_interval": "30s",
									"static_configs": []interface{}{
										map[string]interface{}{
											"targets": []interface{}{"localhost:8080"},
										},
									},
								},
							},
						},
					},
				}},
				Service: otelv1beta1.Service{
					Pipelines: map[string]*otelv1beta1.Pipeline{
						"metrics": {
							Receivers:  []string{"prometheus"},
							Processors: []string{},
							Exporters:  []string{"otlp"},
						},
					},
				},
			},
		},
	}

	// Create fake client with existing collector
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCollector).Build()
	// Create reconciler
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, metric, otelConfig)
	require.NoError(t, err)

	// Test reconcile - should update existing resource
	err = reconciler.Reconcile(t.Context())
	require.NoError(t, err)
	assert.NoError(t, err)
	// Verify collector was updated
	updatedCollector := &otelv1beta1.OpenTelemetryCollector{}
	err = client.Get(t.Context(), types.NamespacedName{Name: componentMeta.Name, Namespace: componentMeta.Namespace}, updatedCollector)
	require.NoError(t, err)
	assert.NoError(t, err)

	// Verify updated config
	receivers := updatedCollector.Spec.Config.Receivers.Object
	prometheusConfig := receivers["prometheus"].(map[string]interface{})
	config := prometheusConfig["config"].(map[string]interface{})
	scrapeConfigs := config["scrape_configs"].([]interface{})
	scrapeConfig := scrapeConfigs[0].(map[string]interface{})

	assert.Equal(t, "otel-collector", scrapeConfig["job_name"])
	assert.Equal(t, "15s", scrapeConfig["scrape_interval"])
}

func TestSetControllerReferences(t *testing.T) {
	// Using assert library instead of gomega

	// Setup test data
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
	}

	// Create a proper runtime.Object for owner reference
	owner := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-resource",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
	}

	metric := v1beta1.MetricsSpec{
		PodMetric: &v1beta1.PodMetricSource{
			Metric: v1beta1.PodMetrics{},
		},
	}

	otelConfig := v1beta1.OtelCollectorConfig{
		ScrapeInterval:         "15s",
		MetricReceiverEndpoint: "otel-collector:4317",
	}

	// Create the test scheme
	scheme := runtime.NewScheme()
	_ = otelv1beta1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	// Create fake client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	// Create reconciler
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, metric, otelConfig)
	require.NoError(t, err)

	// Test set controller reference
	err = reconciler.SetControllerReferences(owner, scheme)
	require.NoError(t, err)
	assert.NoError(t, err)

	// Verify owner reference was set
	ownerRefs := reconciler.OTelCollector.OwnerReferences
	assert.Len(t, ownerRefs, 1)
	assert.Equal(t, owner.Name, ownerRefs[0].Name)
	assert.Equal(t, owner.UID, ownerRefs[0].UID)
}
