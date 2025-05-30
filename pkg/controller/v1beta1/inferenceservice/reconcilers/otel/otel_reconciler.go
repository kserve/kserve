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
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/utils"

	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const ModeSidecar = "sidecar"

var log = logf.Log.WithName("OTelReconciler")

type OtelReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	OTelCollector *otelv1beta1.OpenTelemetryCollector
	metric        v1beta1.MetricsSpec
}

func NewOtelReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	metric v1beta1.MetricsSpec,
	otelConfig v1beta1.OtelCollectorConfig,
) (*OtelReconciler, error) {
	return &OtelReconciler{
		client:        client,
		scheme:        scheme,
		OTelCollector: createOtelCollector(componentMeta, metric, otelConfig),
		metric:        metric,
	}, nil
}

func createOtelCollector(componentMeta metav1.ObjectMeta,
	metric v1beta1.MetricsSpec,
	otelConfig v1beta1.OtelCollectorConfig,
) *otelv1beta1.OpenTelemetryCollector {
	metricNames := metric.PodMetric.Metric.MetricNames
	port, ok := componentMeta.Annotations["prometheus.kserve.io/port"]
	if !ok {
		log.Info("Annotation prometheus.kserve.io/port is missing, using default value 8080 to configure OTel Collector")
		port = "8080"
	}

	otelCollector := &otelv1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Annotations: componentMeta.Annotations,
		},
		Spec: otelv1beta1.OpenTelemetryCollectorSpec{
			Mode: otelv1beta1.ModeSidecar,
			Config: otelv1beta1.Config{
				Receivers: otelv1beta1.AnyConfig{Object: map[string]interface{}{
					"prometheus": map[string]interface{}{
						"config": map[string]interface{}{
							"scrape_configs": []interface{}{
								map[string]interface{}{
									"job_name":        "otel-collector",
									"scrape_interval": otelConfig.ScrapeInterval,
									"static_configs": []interface{}{
										map[string]interface{}{
											"targets": []interface{}{"localhost:" + port},
										},
									},
								},
							},
						},
					},
				}},
				Processors: &otelv1beta1.AnyConfig{Object: map[string]interface{}{}},
				Exporters: otelv1beta1.AnyConfig{Object: map[string]interface{}{
					"otlp": map[string]interface{}{
						"endpoint":    otelConfig.MetricReceiverEndpoint,
						"compression": "none",
						"tls": map[string]interface{}{
							"insecure": true,
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

	// Add filter processor to exclude the metric that is used for scaling if it is specified.
	// otherwise, all metrics will be sent to the OTel backend.
	if len(metricNames) > 0 {
		metricFilters := []interface{}{}
		for _, name := range metricNames {
			metricFilters = append(metricFilters, fmt.Sprintf(`name != "%s"`, name))
		}
		otelCollector.Spec.Config.Processors = &otelv1beta1.AnyConfig{Object: map[string]interface{}{
			"filter/ottl": map[string]interface{}{
				"error_mode": "ignore",
				"metrics": map[string]interface{}{
					"metric": metricFilters,
				},
			},
		}}
		otelCollector.Spec.Config.Service.Pipelines["metrics"].Processors = []string{"filter/ottl"}
	}

	return otelCollector
}

func semanticOtelCollectorEquals(desired, existing *otelv1beta1.OpenTelemetryCollector) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

func (o *OtelReconciler) Reconcile(ctx context.Context) error {
	desired := o.OTelCollector

	existing := &otelv1beta1.OpenTelemetryCollector{}
	getExistingErr := o.client.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)
	otelIsNotFound := apierr.IsNotFound(getExistingErr)
	if getExistingErr != nil && !otelIsNotFound {
		return fmt.Errorf("failed to get existing OTel Collector resource: %w", getExistingErr)
	}

	// ISVC is stopped, delete the httproute if it exists, otherwise, do nothing
	forceStopRuntime := utils.GetForceStopRuntime(desired)
	if (getExistingErr != nil && otelIsNotFound) && forceStopRuntime {
		return nil
	}

	if forceStopRuntime {
		if existing.GetDeletionTimestamp() == nil { // check if the otel was already deleted
			log.Info("Deleting OpenTelemetry Collector", "namespace", existing.Namespace, "name", existing.Name)
			if err := o.client.Delete(ctx, existing); err != nil {
				return err
			}
		}
		return nil
	}

	// Create or update the otel to match the desired state
	if getExistingErr != nil && otelIsNotFound {
		log.Info("Creating OTel Collector resource", "name", desired.Name)
		if err := o.client.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create OTel Collector resource", "name", desired.Name)
			return err
		}
		return nil
	}

	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existing.ResourceVersion
	if !semanticOtelCollectorEquals(desired, existing) {
		log.Info("Updating OTel Collector resource", "name", desired.Name)
		if err := o.client.Update(ctx, desired); err != nil {
			log.Error(err, "Failed to update OTel Collector", "name", desired.Name)
		}
	}
	return nil
}

func (o *OtelReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, o.OTelCollector, scheme)
}
