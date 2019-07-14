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

// Multitenant autoscaler executable.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	pkgmetrics "github.com/knative/pkg/metrics"
	"github.com/knative/pkg/signals"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/autoscaler/statserver"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/logging"
	"github.com/knative/serving/pkg/metrics"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/hpa"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	kubeinformers "k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	statsServerAddr = ":8080"
	statsBufferLen  = 1000
	component       = "autoscaler"
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
)

func main() {
	flag.Parse()

	logger, atomicLevel := setupLogger()
	defer flush(logger)

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()
	// statsCh is the main communication channel between the stats channel and multiscaler.
	statsCh := make(chan *autoscaler.StatMessage, statsBufferLen)
	defer close(statsCh)

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalw("Error building kubeconfig", zap.Error(err))
	}

	opt := reconciler.NewOptionsOrDie(cfg, logger, stopCh)

	dynConfig := scalerConfig(logger)

	// Watch the logging config map and dynamically update logging levels.
	opt.ConfigMapWatcher.Watch(logging.ConfigMapName(), logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map and dynamically update metrics exporter.
	opt.ConfigMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))
	// Watch the autoscaler config map and dynamically update autoscaler config.
	opt.ConfigMapWatcher.Watch(autoscaler.ConfigName, dynConfig.Update)

	// Set up informer factories.
	servingInformerFactory := informers.NewSharedInformerFactory(opt.ServingClientSet, time.Second*30)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(opt.KubeClientSet, time.Second*30)

	// Set up informers.
	paInformer := servingInformerFactory.Autoscaling().V1alpha1().PodAutoscalers()
	sksInformer := servingInformerFactory.Networking().V1alpha1().ServerlessServices()
	endpointsInformer := kubeInformerFactory.Core().V1().Endpoints()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	hpaInformer := kubeInformerFactory.Autoscaling().V1().HorizontalPodAutoscalers()

	collector := autoscaler.NewMetricCollector(statsScraperFactoryFunc(endpointsInformer.Lister()), statsCh, logger)

	// Set up scalers.
	// uniScalerFactory depends endpointsInformer to be set.
	multiScaler := autoscaler.NewMultiScaler(dynConfig, stopCh, uniScalerFactoryFunc(endpointsInformer), logger)
	scaler := kpa.NewScaler(opt.ServingClientSet, opt.ScaleClientSet, logger, opt.ConfigMapWatcher)

	controllers := []*controller.Impl{
		kpa.NewController(&opt, paInformer, sksInformer, serviceInformer, endpointsInformer, multiScaler, collector, scaler, dynConfig),
		hpa.NewController(&opt, paInformer, sksInformer, hpaInformer),
	}

	// Set up a statserver.
	statsServer := statserver.New(statsServerAddr, statsCh, logger)
	defer statsServer.Shutdown(time.Second * 5)

	// Start watching the configs.
	if err := opt.ConfigMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("Failed to start watching configs", zap.Error(err))
	}

	// Start all of the informers and wait for them to sync.
	if err := controller.StartInformers(
		stopCh,
		endpointsInformer.Informer(),
		hpaInformer.Informer(),
		paInformer.Informer(),
		serviceInformer.Informer(),
		sksInformer.Informer(),
	); err != nil {
		logger.Fatalf("Failed to start informers: %v", err)
	}

	go controller.StartAll(stopCh, controllers...)

	// Run the controllers and the statserver in a group.
	var eg errgroup.Group
	eg.Go(func() error {
		return statsServer.ListenAndServe()
	})

	go func() {
		for {
			sm, ok := <-statsCh
			if !ok {
				break
			}
			multiScaler.RecordStat(sm.Key, sm.Stat)
		}
	}()

	egCh := make(chan struct{})

	go func() {
		if err := eg.Wait(); err != nil {
			logger.Errorw("Group error.", zap.Error(err))
		}
		close(egCh)
	}()

	select {
	case <-egCh:
	case <-stopCh:
	}
}

func setupLogger() (*zap.SugaredLogger, zap.AtomicLevel) {
	loggingConfigMap, err := configmap.Load("/etc/config-logging")
	if err != nil {
		log.Fatalf("Error loading logging configuration: %v", err)
	}
	loggingConfig, err := logging.NewConfigFromMap(loggingConfigMap)
	if err != nil {
		log.Fatalf("Error parsing logging configuration: %v", err)
	}
	return logging.NewLoggerFromConfig(loggingConfig, component)
}

func scalerConfig(logger *zap.SugaredLogger) *autoscaler.DynamicConfig {
	rawConfig, err := configmap.Load("/etc/config-autoscaler")
	if err != nil {
		logger.Fatalw("Error reading autoscaler configuration", zap.Error(err))
	}
	dynConfig, err := autoscaler.NewDynamicConfigFromMap(rawConfig, logger)
	if err != nil {
		logger.Fatalw("Error parsing autoscaler configuration", zap.Error(err))
	}
	return dynConfig
}

func uniScalerFactoryFunc(endpointsInformer corev1informers.EndpointsInformer) func(decider *autoscaler.Decider, dynamicConfig *autoscaler.DynamicConfig) (autoscaler.UniScaler, error) {
	return func(decider *autoscaler.Decider, dynamicConfig *autoscaler.DynamicConfig) (autoscaler.UniScaler, error) {
		for _, l := range []string{serving.KubernetesServiceLabelKey, serving.ConfigurationLabelKey} {
			if v, ok := decider.Labels[l]; !ok || v == "" {
				return nil, fmt.Errorf("label %q not found or empty in Decider: %v", l, decider)
			}
		}

		serviceName := decider.Labels[serving.ServiceLabelKey] // This can be empty.
		configName := decider.Labels[serving.ConfigurationLabelKey]
		k8SSvcName := decider.Labels[serving.KubernetesServiceLabelKey]

		// Create a stats reporter which tags statistics by PA namespace, configuration name, and PA name.
		reporter, err := autoscaler.NewStatsReporter(decider.Namespace, serviceName, configName, decider.Name)
		if err != nil {
			return nil, err
		}

		return autoscaler.New(dynamicConfig, decider.Namespace,
			k8SSvcName, endpointsInformer,
			decider.Spec.TargetConcurrency, reporter)
	}
}

func statsScraperFactoryFunc(endpointsLister corev1listers.EndpointsLister) func(metric *autoscaler.Metric) (autoscaler.StatsScraper, error) {
	return func(metric *autoscaler.Metric) (autoscaler.StatsScraper, error) {
		return autoscaler.NewServiceScraper(metric, endpointsLister)
	}
}

func flush(logger *zap.SugaredLogger) {
	logger.Sync()
	pkgmetrics.FlushExporter()
}
