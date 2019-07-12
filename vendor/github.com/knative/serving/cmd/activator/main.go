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
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging/logkey"
	pkgmetrics "github.com/knative/pkg/metrics"
	"github.com/knative/pkg/signals"
	"github.com/knative/pkg/system"
	"github.com/knative/pkg/version"
	"github.com/knative/pkg/websocket"
	"github.com/knative/serving/cmd/util"
	"github.com/knative/serving/pkg/activator"
	activatorhandler "github.com/knative/serving/pkg/activator/handler"
	activatorutil "github.com/knative/serving/pkg/activator/util"
	"github.com/knative/serving/pkg/apis/networking"
	nv1a1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	clientset "github.com/knative/serving/pkg/client/clientset/versioned"
	servinginformers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/goversion"
	"github.com/knative/serving/pkg/http/h2c"
	"github.com/knative/serving/pkg/logging"
	"github.com/knative/serving/pkg/metrics"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/queue"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// Fail if using unsupported go version.
var _ = goversion.IsSupported()

const (
	component = "activator"

	// This is the number of times we will perform network probes to
	// see if the Revision is accessible before forwarding the actual
	// request.
	maxRetries = 18

	// Add a little buffer space between request handling and stat
	// reporting so that latency in the stat pipeline doesn't
	// interfere with request handling.
	statReportingQueueLength = 10

	// Add enough buffer to not block request serving on stats collection
	requestCountingQueueLength = 100

	// The number of requests that are queued on the breaker before the 503s are sent.
	// The value must be adjusted depending on the actual production requirements.
	breakerQueueDepth = 10000

	// The upper bound for concurrent requests sent to the revision.
	// As new endpoints show up, the Breakers concurrency increases up to this value.
	breakerMaxConcurrency = 1000

	// The port on which autoscaler WebSocket server listens.
	autoscalerPort = 8080

	defaultResyncInterval = 10 * time.Hour
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
)

func statReporter(statSink *websocket.ManagedConnection, stopCh <-chan struct{}, statChan <-chan *autoscaler.StatMessage, logger *zap.SugaredLogger) {
	for {
		select {
		case sm := <-statChan:
			if statSink == nil {
				logger.Error("Stat sink is not connected")
				continue
			}
			if err := statSink.Send(sm); err != nil {
				logger.Errorw("Error while sending stat", zap.Error(err))
			}
		case <-stopCh:
			// It's a sending connection, so no drainage required.
			statSink.Shutdown()
			return
		}
	}
}

func main() {
	flag.Parse()
	cm, err := configmap.Load("/etc/config-logging")
	if err != nil {
		log.Fatalf("Error loading logging configuration: %v", err)
	}
	config, err := logging.NewConfigFromMap(cm)
	if err != nil {
		log.Fatalf("Error parsing logging configuration: %v", err)
	}
	createdLogger, atomicLevel := logging.NewLoggerFromConfig(config, component)
	logger := createdLogger.With(zap.String(logkey.ControllerType, "activator"))
	defer flush(logger)

	logger.Info("Starting the knative activator")

	clusterConfig, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalw("Error getting cluster configuration", zap.Error(err))
	}
	kubeClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.Fatalw("Error building new kubernetes client", zap.Error(err))
	}
	servingClient, err := clientset.NewForConfig(clusterConfig)
	if err != nil {
		logger.Fatalw("Error building serving clientset", zap.Error(err))
	}

	// We sometimes startup faster than we can reach kube-api. Poll on failure to prevent us terminating
	if perr := wait.PollImmediate(time.Second, 60*time.Second, func() (bool, error) {
		if err = version.CheckMinimumVersion(kubeClient.Discovery()); err != nil {
			logger.Errorw("Failed to get k8s version", zap.Error(err))
		}
		return err == nil, nil
	}); perr != nil {
		logger.Fatalw("Timed out attempting to get k8s version", zap.Error(err))
	}

	reporter, err := activator.NewStatsReporter()
	if err != nil {
		logger.Fatalw("Failed to create stats reporter", zap.Error(err))
	}

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()
	statChan := make(chan *autoscaler.StatMessage, statReportingQueueLength)
	defer close(statChan)

	reqChan := make(chan activatorhandler.ReqEvent, requestCountingQueueLength)
	defer close(reqChan)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, defaultResyncInterval)
	servingInformerFactory := servinginformers.NewSharedInformerFactory(servingClient, defaultResyncInterval)
	endpointInformer := kubeInformerFactory.Core().V1().Endpoints()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	revisionInformer := servingInformerFactory.Serving().V1alpha1().Revisions()
	sksInformer := servingInformerFactory.Networking().V1alpha1().ServerlessServices()

	// Run informers instead of starting them from the factory to prevent the sync hanging because of empty handler.
	if err := controller.StartInformers(
		stopCh,
		revisionInformer.Informer(),
		endpointInformer.Informer(),
		serviceInformer.Informer(),
		sksInformer.Informer()); err != nil {
		logger.Fatalf("Failed to start informers: %v", err)
	}

	params := queue.BreakerParams{QueueDepth: breakerQueueDepth, MaxConcurrency: breakerMaxConcurrency, InitialCapacity: 0}

	// Return the number of endpoints, 0 if no endpoints are found.
	endpointsCountGetter := func(sks *nv1a1.ServerlessService) (int, error) {
		count, err := resources.FetchReadyAddressCount(endpointInformer.Lister(), sks.Namespace, sks.Status.PrivateServiceName)
		return count, err
	}

	// Returns the revision from the observer.
	revisionGetter := func(revID activator.RevisionID) (*v1alpha1.Revision, error) {
		return revisionInformer.Lister().Revisions(revID.Namespace).Get(revID.Name)
	}

	// Returns the SKS from the observer.
	sksGetter := func(ns, n string) (*nv1a1.ServerlessService, error) {
		return sksInformer.Lister().ServerlessServices(ns).Get(n)
	}

	serviceGetter := func(namespace, name string) (*corev1.Service, error) {
		return serviceInformer.Lister().Services(namespace).Get(name)
	}

	throttlerParams := activator.ThrottlerParams{
		BreakerParams: params,
		Logger:        logger,
		GetEndpoints:  endpointsCountGetter,
		GetRevision:   revisionGetter,
		GetSKS:        sksGetter,
	}
	throttler := activator.NewThrottler(throttlerParams)

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    throttler.UpdateEndpoints,
		UpdateFunc: controller.PassNew(throttler.UpdateEndpoints),
		DeleteFunc: throttler.DeleteBreaker,
	}

	// Update/create the breaker in the throttler when the number of endpoints changes.
	// Pass only the endpoints created by revisions.
	// TODO(greghaynes) we have to allow unset and use the old RevisionUID filter for backwards compat.
	// When we can assume our ServiceTypeKey label is present in all services we can filter all but
	// networking.ServiceTypeKey == networking.ServiceTypePublic
	epFilter := reconciler.ChainFilterFuncs(
		reconciler.LabelExistsFilterFunc(serving.RevisionUID),
		// We are only interested in the private services, since that is
		// what is populated by the actual revision backends.
		reconciler.LabelFilterFunc(networking.ServiceTypeKey, string(networking.ServiceTypePrivate), true),
	)
	endpointInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: epFilter,
		Handler:    handler,
	})

	// Open a websocket connection to the autoscaler
	autoscalerEndpoint := fmt.Sprintf("ws://%s.%s.svc.%s:%d", "autoscaler", system.Namespace(), network.GetClusterDomainName(), autoscalerPort)
	logger.Info("Connecting to autoscaler at", autoscalerEndpoint)
	statSink := websocket.NewDurableSendingConnection(autoscalerEndpoint, logger)
	go statReporter(statSink, stopCh, statChan, logger)

	podName := util.GetRequiredEnvOrFatal("POD_NAME", logger)

	// Create and run our concurrency reporter
	reportTicker := time.NewTicker(time.Second)
	defer reportTicker.Stop()
	cr := activatorhandler.NewConcurrencyReporter(podName, reqChan, reportTicker.C, statChan)
	go cr.Run(stopCh)

	// Create activation handler chain
	// Note: innermost handlers are specified first, ie. the last handler in the chain will be executed first
	var ah http.Handler = &activatorhandler.ActivationHandler{
		Transport:     activatorutil.AutoTransport,
		Logger:        logger,
		Reporter:      reporter,
		Throttler:     throttler,
		GetProbeCount: maxRetries,
		GetRevision:   revisionGetter,
		GetSKS:        sksGetter,
		GetService:    serviceGetter,
	}
	ah = activatorhandler.NewRequestEventHandler(reqChan, ah)
	ah = &activatorhandler.HealthHandler{HealthCheck: statSink.Status, NextHandler: ah}
	ah = &activatorhandler.ProbeHandler{NextHandler: ah}

	// Watch the logging config map and dynamically update logging levels.
	configMapWatcher := configmap.NewInformedWatcher(kubeClient, system.Namespace())
	configMapWatcher.Watch(logging.ConfigMapName(), logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map and dynamically update metrics exporter.
	configMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))
	if err = configMapWatcher.Start(stopCh); err != nil {
		logger.Fatalw("Failed to start configuration manager", zap.Error(err))
	}

	http1Srv := h2c.NewServer(":8080", ah)
	go func() {
		if err := http1Srv.ListenAndServe(); err != nil {
			logger.Errorw("Error running HTTP server", zap.Error(err))
		}
	}()

	h2cSrv := h2c.NewServer(":8081", ah)
	go func() {
		if err := h2cSrv.ListenAndServe(); err != nil {
			logger.Errorw("Error running HTTP server", zap.Error(err))
		}
	}()

	<-stopCh
	http1Srv.Shutdown(context.Background())
	h2cSrv.Shutdown(context.Background())
}

func flush(logger *zap.SugaredLogger) {
	logger.Sync()
	pkgmetrics.FlushExporter()
}
