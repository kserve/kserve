/*
Copyright 2023 The KServe Authors.

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
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/zapr"
	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	"go.uber.org/zap"
	"knative.dev/networking/pkg/http/header"
	proxy "knative.dev/networking/pkg/http/proxy"
	pkglogging "knative.dev/pkg/logging"
	pkgnet "knative.dev/pkg/network"
	pkghandler "knative.dev/pkg/network/handlers"
	"knative.dev/pkg/signals"
	"knative.dev/serving/pkg/queue"
	"knative.dev/serving/pkg/queue/health"
	"knative.dev/serving/pkg/queue/readiness"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/agent"
	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/batcher"
	kfslogger "github.com/kserve/kserve/pkg/logger"
)

var (
	port          = flag.String("port", "9081", "Agent port")
	componentPort = flag.Int("component-port", 8080, "Component port")
	// model puller flags
	enablePuller = flag.Bool("enable-puller", false, "Enable model puller")
	configDir    = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir     = flag.String("model-dir", "/mnt/models", "directory for model files")
	// logger flags
	logUrl              = flag.String("log-url", "", "The URL to send request/response logs to")
	workers             = flag.Int("workers", 5, "Number of workers")
	sourceUri           = flag.String("source-uri", "", "The source URI to use when publishing cloudevents")
	logMode             = flag.String("log-mode", string(v1beta1.LogAll), "Whether to log 'request', 'response' or 'all'")
	inferenceService    = flag.String("inference-service", "", "The InferenceService name to add as header to log events")
	namespace           = flag.String("namespace", "", "The namespace to add as header to log events")
	endpoint            = flag.String("endpoint", "", "The endpoint name to add as header to log events")
	component           = flag.String("component", "", "The component name (predictor, explainer, transformer) to add as header to log events")
	metadataHeaders     = flag.StringSlice("metadata-headers", nil, "Allow list of headers that will be passed down as metadata")
	metadataAnnotations = flag.StringSlice("metadata-annotations", nil, "Allow list of metadata annotation to be passed with payload logging")
	// batcher flags
	enableBatcher = flag.Bool("enable-batcher", false, "Enable request batcher")
	maxBatchSize  = flag.String("max-batchsize", "32", "Max Batch Size")
	maxLatency    = flag.String("max-latency", "5000", "Max Latency in milliseconds")
	// probing flags
	readinessProbeTimeout = flag.Duration("probe-period", -1, "run readiness probe with given timeout") //nolint: unused
	// This creates an abstract socket instead of an actual file.
	unixSocketPath = "@/kserve/agent.sock"
	CaCertFile     = flag.String("logger-ca-cert-file", "service-ca.crt", "The logger CA certificate file")
	TlsSkipVerify  = flag.Bool("logger-tls-skip-verify", false, "Skip verification of TLS certificate")
)

const (
	// reportingPeriod is the interval of time between reporting stats by queue proxy.
	reportingPeriod = 1 * time.Second //nolint: unused

	// Duration the /wait-for-drain handler should wait before returning.
	// This is to give networking a little bit more time to remove the pod
	// from its configuration and propagate that to all loadbalancers and nodes.
	drainSleepDuration = 30 * time.Second
)

type config struct {
	// Making the below fields optional since raw deployment won't have them
	ContainerConcurrency   int    `split_words:"true"`
	QueueServingPort       int    `split_words:"true"`
	UserPort               int    `split_words:"true"`
	RevisionTimeoutSeconds int    `split_words:"true"`
	ServingReadinessProbe  string `split_words:"true" required:"true"`
	// See https://github.com/knative/serving/issues/12387
	EnableHTTP2AutoDetection   bool `envconfig:"ENABLE_HTTP2_AUTO_DETECTION"` // optional
	EnableMultiContainerProbes bool `split_words:"true"`
	// Logging configuration
	ServingLoggingConfig         string `split_words:"true"`
	ServingLoggingLevel          string `split_words:"true"`
	ServingRequestLogTemplate    string `split_words:"true"` // optional
	ServingEnableRequestLog      bool   `split_words:"true"` // optional
	ServingEnableProbeRequestLog bool   `split_words:"true"` // optional
}

type loggerArgs struct {
	loggerType       v1beta1.LoggerType
	logUrl           *url.URL
	sourceUrl        *url.URL
	inferenceService string
	namespace        string
	endpoint         string
	component        string
	metadataHeaders  []string
	annotations      map[string]string
	certName         string
	tlsSkipVerify    bool
}

type batcherArgs struct {
	maxBatchSize int
	maxLatency   int
}

func main() {
	flag.Parse()
	// Parse the environment.
	var env config
	if err := envconfig.Process("", &env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger, _ := pkglogging.NewLogger(env.ServingLoggingConfig, env.ServingLoggingLevel)
	ctrl.SetLogger(zapr.NewLogger(logger.Desugar()))
	// Setup probe to run for checking user container healthiness.
	probe := func() bool { return true }
	if env.ServingReadinessProbe != "" {
		probe = buildProbe(logger, env.ServingReadinessProbe, env.EnableHTTP2AutoDetection, env.EnableMultiContainerProbes).ProbeContainer
	}

	if *enablePuller {
		logger.Infof("Initializing model agent with config-dir %s, model-dir %s", *configDir, *modelDir)
		startModelPuller(logger)
	}

	var loggerArgs *loggerArgs
	if *logUrl != "" {
		logger.Info("Starting logger")
		loggerArgs = startLogger(*workers, logger)
	}

	var batcherArgs *batcherArgs
	if *enableBatcher {
		logger.Info("Starting batcher")
		batcherArgs = startBatcher(logger)
	}
	logger.Info("Starting agent http server...")
	ctx := signals.NewContext()
	mainServer, drain := buildServer(*port, *componentPort, loggerArgs, batcherArgs, probe, logger)
	servers := map[string]*http.Server{
		"main": mainServer,
	}
	errCh := make(chan error)
	listenCh := make(chan struct{})
	for name, server := range servers {
		go func(name string, s *http.Server) {
			l, err := net.Listen("tcp", s.Addr)
			if err != nil {
				errCh <- fmt.Errorf("%s server failed to listen: %w", name, err)
				return
			}

			// Notify the unix socket setup that the tcp socket for the main server is ready.
			if s == mainServer {
				close(listenCh)
			}

			// Don't forward ErrServerClosed as that indicates we're already shutting down.
			if err := s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("%s server failed to serve: %w", name, err)
			}
		}(name, server)
	}

	// Listen on a unix socket so that the exec probe can avoid having to go
	// through the full tcp network stack.
	go func() {
		// Only start listening on the unix socket once the tcp socket for the
		// main server is setup.
		// This avoids the unix socket path succeeding before the tcp socket path
		// is actually working and thus it avoids a race.
		<-listenCh

		l, err := net.Listen("unix", unixSocketPath)
		if err != nil {
			errCh <- fmt.Errorf("failed to listen to unix socket: %w", err)
			return
		}
		// Create an http.Server instance with timeouts
		// https://medium.com/a-journey-with-go/go-understand-and-mitigate-slowloris-attack-711c1b1403f6
		ServerInstance := &http.Server{
			Handler:           mainServer.Handler, // specify your HTTP handler
			ReadHeaderTimeout: time.Minute,        // set the maximum duration for reading the entire request, including the body
			WriteTimeout:      time.Minute,        // set the maximum duration before timing out writes of the response
			IdleTimeout:       3 * time.Minute,    // set the maximum amount of time to wait for the next request when keep-alives are enabled
		}

		if err := ServerInstance.Serve(l); err != nil {
			errCh <- fmt.Errorf("serving failed on unix socket: %w", err)
		}
	}()

	// Blocks until we actually receive a TERM signal or one of the servers
	// exit unexpectedly. We fold both signals together because we only want
	// to act on the first of those to reach here.
	select {
	case err := <-errCh:
		logger.Errorw("Failed to bring up agent, shutting down.", zap.Error(err))
		// This extra flush is needed because defers are not handled via os.Exit calls.
		if err := logger.Sync(); err != nil {
			logger.Errorf("Error syncing logger: %v", err)
		}
		os.Stdout.Sync()
		os.Stderr.Sync()
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("Received TERM signal, attempting to gracefully shutdown servers.")
		logger.Infof("Sleeping %v to allow K8s propagation of non-ready state", drainSleepDuration)
		drain()

		for serverName, srv := range servers {
			logger.Info("Shutting down server: ", serverName)
			if err := srv.Shutdown(context.Background()); err != nil {
				logger.Errorw("Failed to shutdown server", zap.String("server", serverName), zap.Error(err))
			}
		}
		logger.Info("Shutdown complete, exiting...")
	}
}

func startBatcher(logger *zap.SugaredLogger) *batcherArgs {
	maxBatchSizeInt, err := strconv.Atoi(*maxBatchSize)
	if err != nil || maxBatchSizeInt <= 0 {
		logger.Error(errors.New("Invalid max batch size"), *maxBatchSize)
		os.Exit(1)
	}

	maxLatencyInt, err := strconv.Atoi(*maxLatency)
	if err != nil || maxLatencyInt <= 0 {
		logger.Error(errors.New("Invalid max latency"), *maxLatency)
		os.Exit(1)
	}

	return &batcherArgs{
		maxLatency:   maxLatencyInt,
		maxBatchSize: maxBatchSizeInt,
	}
}

func startLogger(workers int, logger *zap.SugaredLogger) *loggerArgs {
	loggingMode := v1beta1.LoggerType(*logMode)
	switch loggingMode {
	case v1beta1.LogAll, v1beta1.LogRequest, v1beta1.LogResponse:
	default:
		logger.Errorf("Malformed log-mode %s", *logMode)
		os.Exit(-1)
	}

	logUrlParsed, err := url.Parse(*logUrl)
	if err != nil {
		logger.Errorf("Malformed log-url %s", *logUrl)
		os.Exit(-1)
	}

	if *sourceUri == "" {
		*sourceUri = fmt.Sprintf("http://localhost:%s/", *port)
	}

	sourceUriParsed, err := url.Parse(*sourceUri)
	if err != nil {
		logger.Errorf("Malformed source_uri %s", *sourceUri)
		os.Exit(-1)
	}

	var annotationKVPair map[string]string = map[string]string{}
	for _, annotations := range *metadataAnnotations {
		k, v, found := strings.Cut(annotations, "=")
		if found {
			annotationKVPair[k] = v
		} else {
			logger.Errorf("annotation does not adhere to desired format got key: %s value: %s", k, v)
			os.Exit(-1)
		}
	}

	logger.Info("Starting the log dispatcher")
	kfslogger.StartDispatcher(workers, logger)
	return &loggerArgs{
		loggerType:       loggingMode,
		logUrl:           logUrlParsed,
		sourceUrl:        sourceUriParsed,
		inferenceService: *inferenceService,
		endpoint:         *endpoint,
		namespace:        *namespace,
		component:        *component,
		metadataHeaders:  *metadataHeaders,
		annotations:      annotationKVPair,
		certName:         *CaCertFile,
		tlsSkipVerify:    *TlsSkipVerify,
	}
}

func startModelPuller(logger *zap.SugaredLogger) {
	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[storage.Protocol]storage.Provider{},
		Logger:    logger,
	}
	watcher := agent.NewWatcher(*configDir, *modelDir, logger)
	logger.Info("Starting puller")
	agent.StartPullerAndProcessModels(&downloader, watcher.ModelEvents, logger)
	go watcher.Start()
}

func buildProbe(logger *zap.SugaredLogger, probeJSON string, autodetectHTTP2 bool, multiContainerProbes bool) *readiness.Probe {
	coreProbes, err := readiness.DecodeProbes(probeJSON, multiContainerProbes)
	if err != nil {
		logger.Fatalw("Agent failed to parse readiness probe", zap.Error(err))
		panic("Agent failed to parse readiness probe")
	}
	for _, probe := range coreProbes {
		if probe.InitialDelaySeconds == 0 {
			probe.InitialDelaySeconds = 10
		}
	}

	if autodetectHTTP2 {
		return readiness.NewProbeWithHTTP2AutoDetection(coreProbes)
	}
	newProbe := readiness.NewProbe(coreProbes)
	return newProbe
}

func buildServer(port string, userPort int, loggerArgs *loggerArgs, batcherArgs *batcherArgs,
	probeContainer func() bool, logging *zap.SugaredLogger,
) (server *http.Server, drain func()) {
	logging.Infof("Building server user port %d port %s", userPort, port)
	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(userPort)),
	}

	maxIdleConns := 1000 // TODO: somewhat arbitrary value for CC=0, needs experimental validation.

	httpProxy := httputil.NewSingleHostReverseProxy(target)
	httpProxy.Transport = pkgnet.NewAutoTransport(maxIdleConns /* max-idle */, maxIdleConns /* max-idle-per-host */)
	httpProxy.ErrorHandler = pkghandler.Error(logging)
	httpProxy.BufferPool = proxy.NewBufferPool()
	httpProxy.FlushInterval = proxy.FlushInterval

	// Create handler chain.
	// Note: innermost handlers are specified first, ie. the last handler in the chain will be executed first.
	var composedHandler http.Handler = httpProxy

	if batcherArgs != nil {
		composedHandler = batcher.New(batcherArgs.maxBatchSize, batcherArgs.maxLatency, composedHandler, logging)
	}
	if loggerArgs != nil {
		composedHandler = kfslogger.New(loggerArgs.logUrl, loggerArgs.sourceUrl, loggerArgs.loggerType,
			loggerArgs.inferenceService, loggerArgs.namespace, loggerArgs.endpoint, loggerArgs.component, composedHandler,
			loggerArgs.metadataHeaders, loggerArgs.certName, loggerArgs.annotations, loggerArgs.tlsSkipVerify)
	}

	composedHandler = queue.ForwardedShimHandler(composedHandler)

	drainer := &pkghandler.Drainer{
		QuietPeriod: drainSleepDuration,
		// Add Activator probe header to the drainer so it can handle probes directly from activator
		HealthCheckUAPrefixes: []string{header.ActivatorUserAgent},
		Inner:                 composedHandler,
		HealthCheck:           health.ProbeHandler(probeContainer, false),
	}
	composedHandler = drainer
	return pkgnet.NewServer(":"+port, composedHandler), drainer.Drain
}
