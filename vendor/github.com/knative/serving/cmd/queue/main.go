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
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/knative/pkg/logging/logkey"
	"github.com/knative/pkg/metrics"
	"github.com/knative/pkg/signals"
	"github.com/knative/serving/cmd/util"
	"github.com/knative/serving/pkg/activator"
	activatorutil "github.com/knative/serving/pkg/activator/util"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/http/h2c"
	"github.com/knative/serving/pkg/logging"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/queue"
	"github.com/knative/serving/pkg/queue/health"
	queuestats "github.com/knative/serving/pkg/queue/stats"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// Add a little buffer space between request handling and stat
	// reporting so that latency in the stat pipeline doesn't
	// interfere with request handling.
	statReportingQueueLength = 10

	// Add enough buffer to not block request serving on stats collection
	requestCountingQueueLength = 100

	// Duration the /quitquitquit handler should wait before returning.
	// This is to give Istio a little bit more time to remove the pod
	// from its configuration and propagate that to all istio-proxies
	// in the mesh.
	quitSleepDuration = 20 * time.Second

	// commonMetricsPort is the port where common metrics, e.g. request metrics
	// are exposed in Prometheus. This is different from the metrics used
	// for autoscaling, which are exposed in 9090.
	commonMetricsPort = 9091
)

var (
	servingService         string
	servingConfig          string
	servingNamespace       string
	servingRevision        string
	servingRevisionKey     string
	servingPodIP           string
	servingPodName         string
	userTargetPort         int
	userTargetAddress      string
	containerConcurrency   int
	revisionTimeoutSeconds int
	reqChan                = make(chan queue.ReqEvent, requestCountingQueueLength)
	logger                 *zap.SugaredLogger
	breaker                *queue.Breaker

	h2cProxy  *httputil.ReverseProxy
	httpProxy *httputil.ReverseProxy

	server           *http.Server
	healthState      = &health.State{}
	promStatReporter *queue.PrometheusStatsReporter // Prometheus stats reporter.
)

func initEnv() {
	servingService = os.Getenv("SERVING_SERVICE") // KService is optional
	servingConfig = util.GetRequiredEnvOrFatal("SERVING_CONFIGURATION", logger)
	servingNamespace = util.GetRequiredEnvOrFatal("SERVING_NAMESPACE", logger)
	servingRevision = util.GetRequiredEnvOrFatal("SERVING_REVISION", logger)
	servingPodIP = util.GetRequiredEnvOrFatal("SERVING_POD_IP", logger)
	servingPodName = util.GetRequiredEnvOrFatal("SERVING_POD", logger)
	containerConcurrency = util.MustParseIntEnvOrFatal("CONTAINER_CONCURRENCY", logger)
	revisionTimeoutSeconds = util.MustParseIntEnvOrFatal("REVISION_TIMEOUT_SECONDS", logger)
	userTargetPort = util.MustParseIntEnvOrFatal("USER_PORT", logger)
	userTargetAddress = fmt.Sprintf("127.0.0.1:%d", userTargetPort)

	// TODO(mattmoor): Move this key to be in terms of the KPA.
	servingRevisionKey = autoscaler.NewMetricKey(servingNamespace, servingRevision)
	_psr, err := queue.NewPrometheusStatsReporter(servingNamespace, servingConfig, servingRevision, servingPodName)
	if err != nil {
		logger.Fatalw("Failed to create stats reporter", zap.Error(err))
	}
	promStatReporter = _psr
}

func reportStats(statChan chan *autoscaler.Stat) {
	for {
		s := <-statChan
		if err := promStatReporter.Report(s); err != nil {
			logger.Errorw("Error while sending stat", zap.Error(err))
		}
	}
}

func knativeProbeHeader(r *http.Request) string {
	return r.Header.Get(network.ProbeHeaderName)
}

func knativeProxyHeader(r *http.Request) string {
	return r.Header.Get(network.ProxyHeaderName)
}

func probeUserContainer() bool {
	var err error
	wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (bool, error) {
		logger.Debug("TCP probing the user-container.")
		err = health.TCPProbe(userTargetAddress, 100*time.Millisecond)
		return err == nil, nil
	})

	if err == nil {
		logger.Info("User-container successfully probed.")
	} else {
		logger.Errorw("User-container could not be probed successfully.", zap.Error(err))
	}

	return err == nil
}

// Make handler a closure for testing.
func handler(reqChan chan queue.ReqEvent, breaker *queue.Breaker, httpProxy, h2cProxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy := httpProxy
		if r.ProtoMajor == 2 {
			proxy = h2cProxy
		}

		ph := knativeProbeHeader(r)
		switch {
		case ph != "":
			if ph != queue.Name {
				http.Error(w, fmt.Sprintf("unexpected probe header value: %q", ph), http.StatusBadRequest)
				return
			}
			if probeUserContainer() {
				// Respond with the name of the component handling the request.
				w.Write([]byte(queue.Name))
			} else {
				http.Error(w, "container not ready", http.StatusServiceUnavailable)
			}
			return
		case network.IsKubeletProbe(r):
			// Do not count health checks for concurrency metrics
			proxy.ServeHTTP(w, r)
			return
		}

		// Metrics for autoscaling
		h := knativeProxyHeader(r)
		in, out := queue.ReqIn, queue.ReqOut
		if activator.Name == h {
			in, out = queue.ProxiedIn, queue.ProxiedOut
		}
		reqChan <- queue.ReqEvent{Time: time.Now(), EventType: in}
		defer func() {
			reqChan <- queue.ReqEvent{Time: time.Now(), EventType: out}
		}()

		// Enforce queuing and concurrency limits
		if breaker != nil {
			ok := breaker.Maybe(func() {
				proxy.ServeHTTP(w, r)
			})
			if !ok {
				http.Error(w, "overload", http.StatusServiceUnavailable)
			}
		} else {
			proxy.ServeHTTP(w, r)
		}
	}
}

// Sets up /health and /wait-for-drain endpoints.
func createAdminHandlers() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc(queue.RequestQueueHealthPath, healthState.HealthHandler(probeUserContainer))
	mux.HandleFunc(queue.RequestQueueDrainPath, healthState.DrainHandler())

	return mux
}

func main() {
	flag.Parse()
	logger, _ = logging.NewLogger(os.Getenv("SERVING_LOGGING_CONFIG"), os.Getenv("SERVING_LOGGING_LEVEL"))
	logger = logger.Named("queueproxy")
	defer flush(logger)

	initEnv()
	logger = logger.With(
		zap.String(logkey.Key, servingRevisionKey),
		zap.String(logkey.Pod, servingPodName))

	target, err := url.Parse(fmt.Sprintf("http://%s", userTargetAddress))
	if err != nil {
		logger.Fatalw("Failed to parse localhost url", zap.Error(err))
	}

	httpProxy = httputil.NewSingleHostReverseProxy(target)
	httpProxy.FlushInterval = -1
	h2cProxy = httputil.NewSingleHostReverseProxy(target)
	h2cProxy.Transport = h2c.DefaultTransport
	h2cProxy.FlushInterval = -1

	activatorutil.SetupHeaderPruning(httpProxy)
	activatorutil.SetupHeaderPruning(h2cProxy)

	// If containerConcurrency == 0 then concurrency is unlimited.
	if containerConcurrency > 0 {
		// We set the queue depth to be equal to the container concurrency but at least 10 to
		// allow the autoscaler to get a strong enough signal.
		queueDepth := containerConcurrency
		if queueDepth < 10 {
			queueDepth = 10
		}
		params := queue.BreakerParams{QueueDepth: queueDepth, MaxConcurrency: containerConcurrency, InitialCapacity: containerConcurrency}
		breaker = queue.NewBreaker(params)
		logger.Infof("Queue container is starting with %#v", params)
	}

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promStatReporter.Handler())
		http.ListenAndServe(fmt.Sprintf(":%d", v1alpha1.RequestQueueMetricsPort), mux)
	}()

	statChan := make(chan *autoscaler.Stat, statReportingQueueLength)
	defer close(statChan)
	go reportStats(statChan)

	reportTicker := time.NewTicker(queue.ReporterReportingPeriod)
	defer reportTicker.Stop()
	queue.NewStats(servingPodName, queue.Channels{
		ReqChan:    reqChan,
		ReportChan: reportTicker.C,
		StatChan:   statChan,
	}, time.Now())

	adminServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", v1alpha1.RequestQueueAdminPort),
		Handler: createAdminHandlers(),
	}

	timeoutHandler := queue.TimeToFirstByteTimeoutHandler(http.HandlerFunc(handler(reqChan, breaker, httpProxy, h2cProxy)),
		time.Duration(revisionTimeoutSeconds)*time.Second, "request timeout")
	composedHandler := pushRequestMetricHandler(pushRequestLogHandler(timeoutHandler))
	server = h2c.NewServer(fmt.Sprintf(":%d", v1alpha1.RequestQueuePort), composedHandler)

	errChan := make(chan error, 2)
	defer close(errChan)
	// Runs a server created by creator and sends fatal errors to the errChan.
	// Does not act on the ErrServerClosed error since that indicates we're
	// already shutting everything down.
	catchServerError := func(creator func() error) {
		if err := creator(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}

	go catchServerError(server.ListenAndServe)
	go catchServerError(adminServer.ListenAndServe)

	// Blocks until we actually receive a TERM signal or one of the servers
	// exit unexpectedly. We fold both signals together because we only want
	// to act on the first of those to reach here.
	select {
	case err := <-errChan:
		logger.Errorw("Failed to bring up queue-proxy, shutting down.", zap.Error(err))
		flush(logger)
		os.Exit(1)
	case <-signals.SetupSignalHandler():
		logger.Info("Received TERM signal, attempting to gracefully shutdown servers.")
		healthState.Shutdown(func() {
			// Give istio time to sync our "not ready" state
			time.Sleep(quitSleepDuration)

			// Calling server.Shutdown() allows pending requests to
			// complete, while no new work is accepted.
			if err := server.Shutdown(context.Background()); err != nil {
				logger.Errorf("Failed to shutdown proxy server", zap.Error(err))
			}
		})

		flush(logger)
		if err := adminServer.Shutdown(context.Background()); err != nil {
			logger.Errorw("Failed to shutdown admin-server", zap.Error(err))
		}
	}
}

func pushRequestLogHandler(currentHandler http.Handler) http.Handler {
	templ := os.Getenv("SERVING_REQUEST_LOG_TEMPLATE")
	if templ == "" {
		return currentHandler
	}

	revInfo := &queue.RequestLogRevInfo{
		Name:          servingRevision,
		Namespace:     servingNamespace,
		Service:       servingService,
		Configuration: servingConfig,
		PodName:       servingPodName,
		PodIP:         servingPodIP,
	}
	handler, err := queue.NewRequestLogHandler(currentHandler, logging.NewSyncFileWriter(os.Stdout), templ, revInfo)

	if err != nil {
		logger.Errorw("Error setting up request logger. Request logs will be unavailable.", zap.Error(err))
		return currentHandler
	}
	return handler
}

func pushRequestMetricHandler(currentHandler http.Handler) http.Handler {
	backend := os.Getenv("SERVING_REQUEST_METRICS_BACKEND")
	logger.Infof("SERVING_REQUEST_METRICS_BACKEND=%v", backend)
	if backend == "" {
		return currentHandler
	}

	r, err := queuestats.NewStatsReporter(servingNamespace, servingService, servingConfig, servingRevision)
	if err != nil {
		logger.Errorw("Error setting up request metrics reporter. Request metrics will be unavailable.", zap.Error(err))
		return currentHandler
	}

	// Set up OpenCensus exporter.
	// NOTE: We use revision as the component instead of queue because queue is
	// implementation specific. The current metrics are request relative. Using
	// revision is reasonable.
	// TODO(yanweiguo): add the ability to emit metrics with names not combined
	// to component.
	ops := metrics.ExporterOptions{
		Domain:         "knative.dev/serving",
		Component:      "revision",
		PrometheusPort: commonMetricsPort,
		ConfigMap: map[string]string{
			metrics.BackendDestinationKey: backend,
		},
	}
	err = metrics.UpdateExporter(ops, logger)
	if err != nil {
		logger.Errorw("Error setting up request metrics exporter. Request metrics will be unavailable.", zap.Error(err))
		return currentHandler
	}

	handler, err := queue.NewRequestMetricHandler(currentHandler, r)
	if err != nil {
		logger.Errorw("Error setting up request metrics handler. Request metrics will be unavailable.", zap.Error(err))
		return currentHandler
	}
	return handler
}

func flush(logger *zap.SugaredLogger) {
	logger.Sync()
	os.Stdout.Sync()
	os.Stderr.Sync()
	metrics.FlushExporter()
}
