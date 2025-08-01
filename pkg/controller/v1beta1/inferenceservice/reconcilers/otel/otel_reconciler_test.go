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
		metricNames    []string
		otelConfig     v1beta1.OtelCollectorConfig
		expectedConfig map[string]interface{}
	}{
		{
			name: "test with port annotation and single metric",
			componentMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					AnnotationPrometheusPort: "9090",
				},
			},
			metricNames: []string{"request-count"},
			otelConfig: v1beta1.OtelCollectorConfig{
				ScrapeInterval:         "15s",
				MetricReceiverEndpoint: "otel-collector:4317",
			},
			expectedConfig: map[string]interface{}{
				KeyJobName:        JobNameOtelCollector,
				KeyScrapeInterval: "15s",
				KeyStaticConfigs: []interface{}{
					map[string]interface{}{
						KeyTargets: []interface{}{"localhost:9090"},
					},
				},
			},
		},
		{
			name: "test without port annotation and multiple metrics",
			componentMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			metricNames: []string{"metric1", "metric2"},
			otelConfig: v1beta1.OtelCollectorConfig{
				ScrapeInterval: "30s",
			},
			expectedConfig: map[string]interface{}{
				KeyJobName:        JobNameOtelCollector,
				KeyScrapeInterval: "30s",
				KeyStaticConfigs: []interface{}{
					map[string]interface{}{
						KeyTargets: []interface{}{"localhost:8080"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collector := createOtelCollector(tc.componentMeta, tc.metricNames, tc.otelConfig)

			assert.Equal(t, tc.componentMeta.Name, collector.Name)
			assert.Equal(t, tc.componentMeta.Namespace, collector.Namespace)
			assert.Equal(t, otelv1beta1.ModeSidecar, collector.Spec.Mode)

			// Assert config details
			receivers := collector.Spec.Config.Receivers.Object
			prometheusConfig := receivers[PrometheusReceiver].(map[string]interface{})
			config := prometheusConfig[KeyConfig].(map[string]interface{})
			scrapeConfigs := config[KeyScrapeConfigs].([]interface{})
			scrapeConfig := scrapeConfigs[0].(map[string]interface{})

			assert.Equal(t, tc.expectedConfig[KeyJobName], scrapeConfig[KeyJobName])
			assert.Equal(t, tc.expectedConfig[KeyScrapeInterval], scrapeConfig[KeyScrapeInterval])

			staticConfigs := scrapeConfig[KeyStaticConfigs].([]interface{})
			staticConfig := staticConfigs[0].(map[string]interface{})
			targets := staticConfig[KeyTargets].([]interface{})

			assert.Equal(t, tc.expectedConfig[KeyStaticConfigs].([]interface{})[0].(map[string]interface{})[KeyTargets], targets)

			// Verify filter processor if metric names exist
			if len(tc.metricNames) > 0 {
				processors := collector.Spec.Config.Processors.Object
				filterMetrics := processors[ProcessorFilterMetrics].(map[string]interface{})
				metrics := filterMetrics[KeyMetrics].(map[string]interface{})
				include := metrics[KeyInclude].(map[string]interface{})
				metricNames := include[KeyMetricNames].([]string)
				assert.ElementsMatch(t, tc.metricNames, metricNames)
			}

			// Verify processors always include resourcedetection/env and transform
			processors := collector.Spec.Config.Processors.Object
			assert.Contains(t, processors, ProcessorResourcedetectionEnv)
			assert.Contains(t, processors, ProcessorTransform)

			// Verify pipeline processors
			pipeline := collector.Spec.Config.Service.Pipelines[PipelineMetrics].Processors
			if len(tc.metricNames) > 0 {
				assert.Equal(t, []string{ProcessorResourcedetectionEnv, ProcessorTransform, ProcessorFilterMetrics}, pipeline)
				// Verify filter processor config
				filterMetrics := processors[ProcessorFilterMetrics].(map[string]interface{})
				metrics := filterMetrics[KeyMetrics].(map[string]interface{})
				include := metrics[KeyInclude].(map[string]interface{})
				metricNames := include[KeyMetricNames].([]string)
				assert.ElementsMatch(t, tc.metricNames, metricNames)
			} else {
				assert.Equal(t, []string{ProcessorResourcedetectionEnv, ProcessorTransform}, pipeline)
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
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, []string{}, otelConfig)
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
					PrometheusReceiver: map[string]interface{}{
						KeyConfig: map[string]interface{}{
							KeyScrapeConfigs: []interface{}{
								map[string]interface{}{
									KeyJobName:        "old-collector",
									KeyScrapeInterval: "30s",
									KeyStaticConfigs: []interface{}{
										map[string]interface{}{
											KeyTargets: []interface{}{"localhost:8080"},
										},
									},
								},
							},
						},
					},
				}},
				Service: otelv1beta1.Service{
					Pipelines: map[string]*otelv1beta1.Pipeline{
						PipelineMetrics: {
							Receivers:  []string{PrometheusReceiver},
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
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, []string{}, otelConfig)
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
	prometheusConfig := receivers[PrometheusReceiver].(map[string]interface{})
	config := prometheusConfig[KeyConfig].(map[string]interface{})
	scrapeConfigs := config[KeyScrapeConfigs].([]interface{})
	scrapeConfig := scrapeConfigs[0].(map[string]interface{})

	assert.Equal(t, JobNameOtelCollector, scrapeConfig[KeyJobName])
	assert.Equal(t, "15s", scrapeConfig[KeyScrapeInterval])
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
	reconciler, err := NewOtelReconciler(client, scheme, componentMeta, []string{}, otelConfig)
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
