/*
Copyright 2018 The Knative Authors

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

package main

import (
	"flag"
	"log"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	cachinginformers "github.com/knative/caching/pkg/client/informers/externalversions"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	pkgmetrics "github.com/knative/pkg/metrics"
	"github.com/knative/pkg/signals"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/logging"
	"github.com/knative/serving/pkg/metrics"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/configuration"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/labeler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/route"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/serverlessservice"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/service"
	"go.uber.org/zap"
)

const (
	component = "controller"
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
)

func main() {
	flag.Parse()

	// Set up our logger.
	loggingConfigMap, err := configmap.Load("/etc/config-logging")
	if err != nil {
		log.Fatalf("Error loading logging configuration: %v", err)
	}
	loggingConfig, err := logging.NewConfigFromMap(loggingConfigMap)
	if err != nil {
		log.Fatalf("Error parsing logging configuration: %v", err)
	}
	logger, atomicLevel := logging.NewLoggerFromConfig(loggingConfig, component)
	defer flush(logger)

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalw("Error building kubeconfig", zap.Error(err))
	}

	const numControllers = 6
	cfg.QPS = numControllers * rest.DefaultQPS
	cfg.Burst = numControllers * rest.DefaultBurst
	opt := reconciler.NewOptionsOrDie(cfg, logger, stopCh)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(opt.KubeClientSet, opt.ResyncPeriod)
	servingInformerFactory := informers.NewSharedInformerFactory(opt.ServingClientSet, opt.ResyncPeriod)
	cachingInformerFactory := cachinginformers.NewSharedInformerFactory(opt.CachingClientSet, opt.ResyncPeriod)
	buildInformerFactory := revision.KResourceTypedInformerFactory(opt)

	serviceInformer := servingInformerFactory.Serving().V1alpha1().Services()
	routeInformer := servingInformerFactory.Serving().V1alpha1().Routes()
	configurationInformer := servingInformerFactory.Serving().V1alpha1().Configurations()
	revisionInformer := servingInformerFactory.Serving().V1alpha1().Revisions()
	kpaInformer := servingInformerFactory.Autoscaling().V1alpha1().PodAutoscalers()
	clusterIngressInformer := servingInformerFactory.Networking().V1alpha1().ClusterIngresses()
	sksInformer := servingInformerFactory.Networking().V1alpha1().ServerlessServices()
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	coreServiceInformer := kubeInformerFactory.Core().V1().Services()
	endpointsInformer := kubeInformerFactory.Core().V1().Endpoints()
	configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	imageInformer := cachingInformerFactory.Caching().V1alpha1().Images()

	// Build all of our controllers, with the clients constructed above.
	// Add new controllers to this array.
	controllers := []*controller.Impl{
		configuration.NewController(
			opt,
			configurationInformer,
			revisionInformer,
		),
		revision.NewController(
			opt,
			revisionInformer,
			kpaInformer,
			imageInformer,
			deploymentInformer,
			coreServiceInformer,
			endpointsInformer,
			configMapInformer,
			buildInformerFactory,
		),
		route.NewController(
			opt,
			routeInformer,
			configurationInformer,
			revisionInformer,
			coreServiceInformer,
			clusterIngressInformer,
		),
		labeler.NewRouteToConfigurationController(
			opt,
			routeInformer,
			configurationInformer,
			revisionInformer,
		),
		service.NewController(
			opt,
			serviceInformer,
			configurationInformer,
			revisionInformer,
			routeInformer,
		),
		serverlessservice.NewController(
			opt,
			sksInformer,
			coreServiceInformer,
			endpointsInformer,
		),
	}
	if len(controllers) != numControllers {
		logger.Fatalf("Number of controllers and QPS settings mismatch: %d != %d", len(controllers), numControllers)
	}

	// Watch the logging config map and dynamically update logging levels.
	opt.ConfigMapWatcher.Watch(logging.ConfigMapName(), logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map and dynamically update metrics exporter.
	opt.ConfigMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))
	if err := opt.ConfigMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("failed to start configuration manager", zap.Error(err))
	}

	// Start all of the informers and wait for them to sync.
	logger.Info("Starting informers.")
	if err := controller.StartInformers(
		stopCh,
		clusterIngressInformer.Informer(),
		configMapInformer.Informer(),
		configurationInformer.Informer(),
		coreServiceInformer.Informer(),
		deploymentInformer.Informer(),
		endpointsInformer.Informer(),
		imageInformer.Informer(),
		kpaInformer.Informer(),
		revisionInformer.Informer(),
		routeInformer.Informer(),
		serviceInformer.Informer(),
		sksInformer.Informer(),
	); err != nil {
		logger.Fatalf("Failed to start informers: %v", err)
	}

	// Start all of the controllers.
	logger.Info("Starting controllers.")
	go controller.StartAll(stopCh, controllers...)
	<-stopCh
}

func flush(logger *zap.SugaredLogger) {
	logger.Sync()
	pkgmetrics.FlushExporter()
}
