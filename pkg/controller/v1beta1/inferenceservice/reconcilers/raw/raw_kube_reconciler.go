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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/autoscaler"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/otel"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"

	"github.com/pkg/errors"

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
	Workload      reconcilers.WorkloadReconciler
	Service       reconcilers.ServiceReconciler
	Scaler        *autoscaler.AutoscalerReconciler
	OtelCollector *otel.OtelReconciler
	URL           *knapis.URL
}

// NewRawKubeReconciler creates raw kubernetes resource reconciler.
func NewRawKubeReconciler(ctx context.Context,
	client client.Client,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
	workerPodSpec *corev1.PodSpec,
	storageUris *[]v1beta1.StorageUri,
	storageInitializerConfig *kserveTypes.StorageInitializerConfig,
	storageSpec *v1beta1.StorageSpec,
	credentialBuilder *credentials.CredentialBuilder,
	storageContainerSpec *v1alpha1.StorageContainerSpec,
	configMap *corev1.ConfigMap,
) (*RawKubeReconciler, error) {
	// Parse configs from the ConfigMap
	otelCollectorConfig, err := v1beta1.NewOtelCollectorConfig(configMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse OtelCollector config")
	}

	var otelCollector *otel.OtelReconciler
	// create OTel Collector if pod metrics is enabled for auto-scaling
	if componentExt != nil && componentExt.AutoScaling != nil {
		var metricNames []string
		metrics := componentExt.AutoScaling.Metrics
		for _, metric := range metrics {
			if metric.Type == v1beta1.PodMetricSourceType {
				if metric.PodMetric.Metric.Backend == v1beta1.PodsMetricsBackend(constants.OTelBackend) {
					metricNames = append(metricNames, metric.PodMetric.Metric.MetricNames...)
				}
			}
		}
		if len(metricNames) > 0 {
			otelCollector, err = otel.NewOtelReconciler(client, scheme, componentMeta, metricNames, *otelCollectorConfig)
			if err != nil {
				return nil, err
			}
		}
	}

	autoscalerConfig, err := v1beta1.NewAutoscalerConfig(configMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse Autoscaler config")
	}

	as, err := autoscaler.NewAutoscalerReconciler(client, scheme, componentMeta, componentExt, autoscalerConfig, otelCollectorConfig)
	if err != nil {
		return nil, err
	}

	ingressConfig, err := v1beta1.NewIngressConfig(configMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse Ingress config")
	}
	url, err := createRawURL(ingressConfig, componentMeta)
	if err != nil {
		return nil, err
	}

	var multiNodeEnabled bool
	if workerPodSpec != nil {
		multiNodeEnabled = true
	}

	serviceConfig, err := v1beta1.NewServiceConfig(configMap)
	if err != nil {
		// do not return error as service config is optional
		log.Error(err, "failed to parse service config")
	}

	if storageUris != nil && len(*storageUris) > 0 {
		isvcReadonlyStringFlag := pod.GetStorageInitializerReadOnlyFlag(componentMeta.Annotations)

		storageInitializerParams := &pod.StorageInitializerParams{
			Namespace:            componentMeta.Namespace,
			StorageURIs:          *storageUris,
			IsReadOnly:           isvcReadonlyStringFlag,
			PodSpec:              podSpec,
			CredentialBuilder:    credentialBuilder,
			Client:               client,
			Config:               storageInitializerConfig,
			IsvcAnnotations:      componentMeta.Annotations,
			StorageSpec:          storageSpec,
			StorageContainerSpec: storageContainerSpec,
			IsLegacyURI:          false,
		}

		err := pod.CommonStorageInitialization(ctx, storageInitializerParams)
		if err != nil {
			return nil, err
		}

		if workerPodSpec != nil {
			workerStorageInitializerParams := &pod.StorageInitializerParams{
				Namespace:            workerComponentMeta.Namespace,
				StorageURIs:          *storageUris,
				IsReadOnly:           isvcReadonlyStringFlag,
				PodSpec:              workerPodSpec,
				CredentialBuilder:    credentialBuilder,
				Client:               client,
				Config:               storageInitializerConfig,
				IsvcAnnotations:      workerComponentMeta.Annotations,
				StorageSpec:          storageSpec,
				StorageContainerSpec: storageContainerSpec,
				IsLegacyURI:          false,
			}
			err := pod.CommonStorageInitialization(ctx, workerStorageInitializerParams)
			if err != nil {
				return nil, err
			}
		}
	}

	// Parse deploy config from ConfigMap
	deployConfig, err := v1beta1.NewDeployConfig(configMap)
	if err != nil {
		log.Error(err, "failed to parse deploy config")
		deployConfig = nil // Use nil if config is not available
	}

	// Parse deployment mode
	deploymentMode := constants.ParseDeploymentMode(componentMeta.Annotations[constants.DeploymentMode])

	// Use factory to create reconcilers
	factory := reconcilers.NewReconcilerFactory()

	workloadRec, err := factory.CreateWorkloadReconciler(
		ctx,
		deploymentMode,
		reconcilers.WorkloadReconcilerParams{
			Client:              client,
			Scheme:              scheme,
			ComponentMeta:       componentMeta,
			WorkerComponentMeta: workerComponentMeta,
			ComponentExt:        componentExt,
			PodSpec:             podSpec,
			WorkerPodSpec:       workerPodSpec,
			DeployConfig:        deployConfig,
		},
	)
	if err != nil {
		return nil, err
	}

	serviceRec, err := factory.CreateServiceReconciler(
		deploymentMode,
		reconcilers.ServiceReconcilerParams{
			Client:           client,
			Scheme:           scheme,
			ComponentMeta:    componentMeta,
			ComponentExt:     componentExt,
			PodSpec:          podSpec,
			MultiNodeEnabled: multiNodeEnabled,
			ServiceConfig:    serviceConfig,
		},
	)
	if err != nil {
		return nil, err
	}

	return &RawKubeReconciler{
		client:        client,
		scheme:        scheme,
		Workload:      workloadRec,
		Service:       serviceRec,
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
	// reconcile OTel Collector
	if r.OtelCollector != nil {
		err := r.OtelCollector.Reconcile(ctx)
		if err != nil {
			return nil, err
		}
	}
	// reconcile Workload (Deployment)
	deploymentList, err := r.Workload.Reconcile(ctx)
	if err != nil {
		return nil, err
	}

	// reconcile Service
	_, err = r.Service.Reconcile(ctx)
	if err != nil {
		return nil, err
	}

	// reconcile HPA
	err = r.Scaler.Reconcile(ctx)
	if err != nil {
		return nil, err
	}

	return deploymentList, nil
}
