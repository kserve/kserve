/*
Copyright 2021 The KServe Authors.

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

package raw

import (
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/autoscaler"
	deployment "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/deployment"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/otel"
	service "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/service"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	knapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("RawKubeReconciler")

// RawKubeReconciler reconciles the Native K8S Resources
type RawKubeReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	Deployment    *deployment.DeploymentReconciler
	Service       *service.ServiceReconciler
	Scaler        *autoscaler.AutoscalerReconciler
	OtelCollector *otel.OtelReconciler
	URL           *knapis.URL
}

// NewRawKubeReconciler creates raw kubernetes resource reconciler.
func NewRawKubeReconciler(ctx context.Context, client client.Client,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec,
) (*RawKubeReconciler, error) {
	var otelCollector *otel.OtelReconciler
	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if err != nil {
		log.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return nil, err
	}
	otelConfig, err := v1beta1.NewOtelCollectorConfig(configMap)
	if err != nil {
		return nil, err
	}

	if componentExt.AutoScaling != nil {
		metrics := componentExt.AutoScaling.Metrics
		for _, metric := range metrics {
			if metric.Type == v1beta1.ExternalMetricSourceType {
				if *metric.External.Metric.Backend == v1beta1.MetricsBackend(constants.OTelBackend) {
					var err error
					otelCollector, err = otel.NewOtelReconciler(client, scheme, componentMeta, metric, *otelConfig)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	as, err := autoscaler.NewAutoscalerReconciler(client, scheme, componentMeta, componentExt)
	if err != nil {
		return nil, err
	}
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if err != nil {
		return nil, err
	}
	ingressConfig, err := v1beta1.NewIngressConfig(isvcConfigMap)
	if err != nil {
		return nil, err
	}
	url, err := createRawURL(ingressConfig, componentMeta)
	if err != nil {
		return nil, err
	}

	var multiNodeEnabled bool
	if workerPodSpec != nil {
		multiNodeEnabled = true
	}

	// do not return error as service config is optional
	serviceConfig, err1 := v1beta1.NewServiceConfig(isvcConfigMap)
	if err1 != nil {
		log.Error(err1, "failed to get service config")
	}

	return &RawKubeReconciler{
		client:        client,
		scheme:        scheme,
		Deployment:    deployment.NewDeploymentReconciler(client, scheme, componentMeta, workerComponentMeta, componentExt, podSpec, workerPodSpec),
		Service:       service.NewServiceReconciler(client, scheme, componentMeta, componentExt, podSpec, multiNodeEnabled, serviceConfig),
		Scaler:        as,
		OtelCollector: otelCollector,
		URL:           url,
	}, nil
}

func createRawURL(ingressConfig *v1beta1.IngressConfig, metadata metav1.ObjectMeta) (*knapis.URL, error) {
	url := &knapis.URL{}
	url.Scheme = "http"
	var err error
	if url.Host, err = ingress.GenerateDomainName(metadata.Name, metadata, ingressConfig); err != nil {
		return nil, fmt.Errorf("failed creating host name: %w", err)
	}
	return url, nil
}

// Reconcile ...
func (r *RawKubeReconciler) Reconcile(ctx context.Context) ([]*appsv1.Deployment, error) {
	// reconcile Deployment
	deploymentList, err := r.Deployment.Reconcile(ctx)
	if err != nil {
		return nil, err
	}
	// reconcile Service
	_, err = r.Service.Reconcile(ctx)
	if err != nil {
		return nil, err
	}
	// reconcile OTel Collector
	if r.OtelCollector != nil {
		err = r.OtelCollector.Reconcile(ctx)
		if err != nil {
			return nil, err
		}
	}
	// reconcile HPA
	err = r.Scaler.Reconcile(ctx)
	if err != nil {
		return nil, err
	}

	return deploymentList, nil
}
