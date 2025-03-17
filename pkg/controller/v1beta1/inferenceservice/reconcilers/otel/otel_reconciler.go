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
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const ModeSidecar = "sidecar"

var log = logf.Log.WithName("OTelReconciler")

type OtelReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	OTelCollector *otelv1alpha1.OpenTelemetryCollector
	componentExt  *v1beta1.ComponentExtensionSpec
	metric        v1beta1.MetricsSpec
}

func NewOtelReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	metric v1beta1.MetricsSpec,
	otelConfig v1beta1.OtelCollectorConfig,
) (*OtelReconciler, error) {
	return &OtelReconciler{
		client:        client,
		scheme:        scheme,
		OTelCollector: createOtelCollector(componentMeta, componentExt, metric, otelConfig),
		componentExt:  componentExt,
		metric:        metric,
	}, nil
}

func getOtelConfig(metricFilter string, otelConfig v1beta1.OtelCollectorConfig) (string, error) {
	config := map[string]interface{}{
		"receivers": map[string]interface{}{
			"prometheus": map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{
						{
							"job_name":        "otel-collector",
							"scrape_interval": otelConfig.ScrapeInterval,
							"static_configs": []map[string]interface{}{
								{"targets": []string{"localhost:8080"}},
							},
						},
					},
				},
			},
		},
		"processors": map[string]interface{}{
			"filter/ottl": map[string]interface{}{
				"error_mode": "ignore",
				"metrics": map[string]interface{}{
					"metric": []string{
						fmt.Sprintf(`name == "%s"`, metricFilter),
					},
				},
			},
		},
		"exporters": map[string]interface{}{
			"otlp": map[string]interface{}{
				"endpoint":    otelConfig.OTelExporterEndpoint,
				"compression": "none",
				"tls": map[string]interface{}{
					"insecure": true,
				},
			},
		},
		"service": map[string]interface{}{
			"pipelines": map[string]interface{}{
				"metrics": map[string]interface{}{
					"receivers":  []string{"prometheus"},
					"processors": []string{"filter/ottl"},
					"exporters":  []string{"otlp"},
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(yamlBytes), nil
}

func createOtelCollector(componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
	metric v1beta1.MetricsSpec,
	otelConfig v1beta1.OtelCollectorConfig,
) *otelv1alpha1.OpenTelemetryCollector {
	annotations := componentMeta.GetAnnotations()
	metricQuery := metric.External.Metric.Query
	configStr, err := getOtelConfig(metricQuery, otelConfig)
	if err != nil {
		return nil
	}

	otelCollector := &otelv1alpha1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: annotations,
		},
		Spec: otelv1alpha1.OpenTelemetryCollectorSpec{
			Mode:   otelv1alpha1.ModeSidecar,
			Config: configStr,
		},
	}

	return otelCollector
}

func (o *OtelReconciler) Reconcile(ctx context.Context) error {
	desired := o.OTelCollector
	existing := &otelv1alpha1.OpenTelemetryCollector{}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating OTelCollector", "namespace", desired.Namespace, "name", desired.Name)
		if err := o.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}

		return o.client.Update(ctx, existing)
	})
	if err != nil {
		// Create OTelCollector if it does not exist
		if apierr.IsNotFound(err) {
			log.Info("Creating OTelCollector", "namespace", desired.Namespace, "name", desired.Name)
			return o.client.Create(ctx, desired)
		}
		return errors.Wrapf(err, "fails to reconcile OTelCollector")
	}

	return nil
}

func (o *OtelReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, o.OTelCollector, scheme)
}
