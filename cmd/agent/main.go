package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kelseyhightower/envconfig"
	"github.com/kubeflow/kfserving/pkg/agent"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	s3credential "github.com/kubeflow/kfserving/pkg/credentials/s3"
	kfslogger "github.com/kubeflow/kfserving/pkg/logger"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/intstr"
	network "knative.dev/networking/pkg"
	pkglogging "knative.dev/pkg/logging"
	pkgnet "knative.dev/pkg/network"
	"knative.dev/pkg/signals"
	"knative.dev/serving/pkg/queue"
	"knative.dev/serving/pkg/queue/health"
	"knative.dev/serving/pkg/queue/readiness"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strconv"
	"strings"
	"time"
)

var (
	log  = logf.Log.WithName("modelAgent")
	port = flag.String("port", "8081", "Agent port")
	// model puller flags
	enablePuller = flag.Bool("enable-puller", false, "Enable model puller")
	configDir    = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir     = flag.String("model-dir", "/mnt/models", "directory for model files")
	// logger flags
	logUrl           = flag.String("log-url", "", "The URL to send request/response logs to")
	workers          = flag.Int("workers", 5, "Number of workers")
	sourceUri        = flag.String("source-uri", "", "The source URI to use when publishing cloudevents")
	logMode          = flag.String("log-mode", string(v1beta1.LogAll), "Whether to log 'request', 'response' or 'all'")
	inferenceService = flag.String("inference-service", "", "The InferenceService name to add as header to log events")
	namespace        = flag.String("namespace", "", "The namespace to add as header to log events")
	endpoint         = flag.String("endpoint", "", "The endpoint name to add as header to log events")

	// probing flags
	readinessProbeTimeout = flag.Duration("probe-period", -1, "run readiness probe with given timeout")
	// This creates an abstract socket instead of an actual file.
	unixSocketPath = "@/kfserving/agent.sock"
)

type config struct {
	ContainerConcurrency   int    `split_words:"true" required:"true"`
	QueueServingPort       int    `split_words:"true" required:"true"`
	UserPort               int    `split_words:"true" required:"true"`
	RevisionTimeoutSeconds int    `split_words:"true" required:"true"`
	ServingReadinessProbe  string `split_words:"true" required:"true"`
	// Logging configuration
	ServingLoggingConfig         string `split_words:"true" required:"true"`
	ServingLoggingLevel          string `split_words:"true" required:"true"`
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
}

func main() {
	flag.Parse()
	// Parse the environment.
	var env config
	if err := envconfig.Process("", &env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// If this is set, we run as a standalone binary to probe the queue-proxy.
	if *readinessProbeTimeout >= 0 {
		// Use a unix socket rather than TCP to avoid going via entire TCP stack
		// when we're actually in the same container.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", unixSocketPath)
		}

		os.Exit(standaloneProbeMain(*readinessProbeTimeout, transport, *port))
	}
	if *enablePuller {
		startModelPuller()
	}

	var loggerArgs *loggerArgs
	if *logUrl != "" {
		startLogger(*workers)
	}

	healthState := &health.State{}
	ctx := signals.NewContext()
	// Setup probe to run for checking user-application healthiness.
	logger, _ := pkglogging.NewLogger(env.ServingLoggingConfig, env.ServingLoggingLevel)
	probe := buildProbe(logger, env.ServingReadinessProbe, env.UserPort)
	userPort := strconv.Itoa(env.UserPort)
	mainServer := buildServer(ctx, *port, userPort, loggerArgs, healthState, probe, logger)
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
		if err := http.Serve(l, mainServer.Handler); err != nil {
			errCh <- fmt.Errorf("serving failed on unix socket: %w", err)
		}
	}()

	// Blocks until we actually receive a TERM signal or one of the servers
	// exit unexpectedly. We fold both signals together because we only want
	// to act on the first of those to reach here.
	select {
	case err := <-errCh:
		logger.Errorw("Failed to bring up queue-proxy, shutting down.", zap.Error(err))
		// This extra flush is needed because defers are not handled via os.Exit calls.
		logger.Sync()
		os.Stdout.Sync()
		os.Stderr.Sync()
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("Received TERM signal, attempting to gracefully shutdown servers.")
		healthState.Shutdown(func() {
			logger.Infof("Sleeping %v to allow K8s propagation of non-ready state", pkgnet.DefaultDrainTimeout)
			time.Sleep(pkgnet.DefaultDrainTimeout)

			// Calling server.Shutdown() allows pending requests to
			// complete, while no new work is accepted.
			logger.Info("Shutting down main server")
			if err := mainServer.Shutdown(context.Background()); err != nil {
				logger.Errorw("Failed to shutdown proxy server", zap.Error(err))
			}
			// Removing the main server from the shutdown logic as we've already shut it down.
			delete(servers, "main")
		})

		for serverName, srv := range servers {
			logger.Info("Shutting down server: ", serverName)
			if err := srv.Shutdown(context.Background()); err != nil {
				logger.Errorw("Failed to shutdown server", zap.String("server", serverName), zap.Error(err))
			}
		}
		logger.Info("Shutdown complete, exiting...")
	}
}

func startLogger(workers int) *loggerArgs {
	loggingMode := v1beta1.LoggerType(*logMode)
	switch loggingMode {
	case v1beta1.LogAll, v1beta1.LogRequest, v1beta1.LogResponse:
	default:
		log.Info("Malformed log-mode", "mode", *logMode)
		os.Exit(-1)
	}

	logUrlParsed, err := url.Parse(*logUrl)
	if err != nil {
		log.Info("Malformed log-url", "URL", *logUrl)
		os.Exit(-1)
	}

	if *sourceUri == "" {
		*sourceUri = fmt.Sprintf("http://localhost:%s/", *port)
	}

	sourceUriParsed, err := url.Parse(*sourceUri)
	if err != nil {
		log.Info("Malformed source_uri", "URL", *sourceUri)
		os.Exit(-1)
	}
	log.Info("Starting the log dispatcher")
	kfslogger.StartDispatcher(workers, log)
	return &loggerArgs{
		loggerType:       loggingMode,
		logUrl:           logUrlParsed,
		sourceUrl:        sourceUriParsed,
		inferenceService: *inferenceService,
		endpoint:         *endpoint,
		namespace:        *namespace,
	}
}
func startModelPuller() {
	log.Info("Initializing model agent with", "config-dir", configDir, "model-dir", modelDir)

	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[storage.Protocol]storage.Provider{},
	}

	if endpoint, ok := os.LookupEnv(s3credential.AWSEndpointUrl); ok {
		region, _ := os.LookupEnv(s3credential.AWSRegion)
		useVirtualBucketString, ok := os.LookupEnv(s3credential.S3UseVirtualBucket)
		useVirtualBucket := true
		if ok && strings.ToLower(useVirtualBucketString) == "false" {
			useVirtualBucket = false
		}
		sess, err := session.NewSession(&aws.Config{
			Endpoint:         aws.String(endpoint),
			Region:           aws.String(region),
			S3ForcePathStyle: aws.Bool(!useVirtualBucket)},
		)
		log.Info("Initializing s3 client with ", "endpoint", endpoint, "region", region)
		if err != nil {
			panic(err)
		}
		sessionClient := s3.New(sess)
		downloader.Providers[storage.S3] = &storage.S3Provider{
			Client: sessionClient,
			Downloader: s3manager.NewDownloaderWithClient(sessionClient, func(d *s3manager.Downloader) {
			}),
		}
	}

	watcher := agent.NewWatcher(*configDir, *modelDir)
	agent.StartPuller(downloader, watcher.ModelEvents)
	watcher.Start()
}

func buildProbe(logger *zap.SugaredLogger, probeJSON string, port int) *readiness.Probe {
	coreProbe, err := readiness.DecodeProbe(probeJSON)
	if err != nil {
		logger.Fatalw("Queue container failed to parse readiness probe", zap.Error(err))
	}
	if coreProbe.TCPSocket != nil {
		coreProbe.TCPSocket.Port = intstr.FromInt(port)
	} else if coreProbe.HTTPGet != nil {
		coreProbe.HTTPGet.Port = intstr.FromInt(port)
	}
	return readiness.NewProbe(coreProbe)
}

func buildServer(ctx context.Context, port string, userPort string, loggerArgs *loggerArgs,
	healthState *health.State, rp *readiness.Probe,
	logging *zap.SugaredLogger) *http.Server {
	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", userPort),
	}

	maxIdleConns := 1000 // TODO: somewhat arbitrary value for CC=0, needs experimental validation.

	httpProxy := httputil.NewSingleHostReverseProxy(target)
	httpProxy.Transport = pkgnet.NewAutoTransport(maxIdleConns /* max-idle */, maxIdleConns /* max-idle-per-host */)
	httpProxy.ErrorHandler = pkgnet.ErrorHandler(logging)
	httpProxy.BufferPool = network.NewBufferPool()
	httpProxy.FlushInterval = network.FlushInterval

	// Create handler chain.
	// Note: innermost handlers are specified first, ie. the last handler in the chain will be executed first.
	var composedHandler http.Handler = httpProxy
	if loggerArgs != nil {
		composedHandler = kfslogger.New("127.0.0.1", userPort, loggerArgs.logUrl, loggerArgs.sourceUrl, loggerArgs.loggerType,
			loggerArgs.inferenceService, loggerArgs.namespace, loggerArgs.endpoint, composedHandler)

		composedHandler = queue.ForwardedShimHandler(composedHandler)

		composedHandler = ProbeHandler(healthState, rp.ProbeContainer, rp.IsAggressive(), false, composedHandler)
		composedHandler = network.NewProbeHandler(composedHandler)
	}

	return pkgnet.NewServer(":"+port, composedHandler)
}
