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
}

func NewOtelReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
) (*OtelReconciler, error) {
	return &OtelReconciler{
		client:        client,
		scheme:        scheme,
		OTelCollector: createOtelCollector(componentMeta, componentExt),
		componentExt:  componentExt,
	}, nil
}

func createOtelCollector(componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
) *otelv1alpha1.OpenTelemetryCollector {
	annotations := componentMeta.GetAnnotations()

	metricFilter := "process_cpu_seconds_total" // Set dynamically based on the metrics to be collected

	otelCollector := &otelv1alpha1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: annotations,
		},
		Spec: otelv1alpha1.OpenTelemetryCollectorSpec{
			Mode: otelv1alpha1.ModeSidecar,
			Config: fmt.Sprintf(`
				receivers:
				prometheus:
					config:
					scrape_configs:
						- job_name: 'otel-collector'
						scrape_interval: 5s
						static_configs:
							- targets: ['0.0.0.0:8080']
				
				processors:
				filter/ottl:
					error_mode: ignore
					metrics:
					metric:
						- 'type == %q'
				
				exporters:
				otlp:
					endpoint: keda-otel-scaler.keda.svc:4317
					compression: "none"
					tls:
					insecure: true
				
				service:
				pipelines:
					metrics:
					receivers: [prometheus]
					processors: [filter/ottl]
					exporters: [otlp]
				`, metricFilter),
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
