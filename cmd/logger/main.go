package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"net/http"
	"net/url"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/kubeflow/kfserving/pkg/logger"
	"github.com/pkg/errors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	logUrl    = flag.String("log_url", "", "The URL to send request/response logs to")
	port      = flag.String("port", "8080", "Executor port")
	svcPort   = flag.String("svc_port", "8081", "The local port of the service")
	workers   = flag.Int("workers", 5, "Number of workers")
	sourceUri = flag.String("source_uri", "", "The source URI to use when publishing cloudevents")
	logMode   = flag.String("log_mode", "all", "Whether to log 'request', 'response' or 'all'")
	sample    = flag.Float64("sample", 1.0, "Probability to emit a log event")
	modelUri  = flag.String("model_uri", "", "The model uri to add as header to log events")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	if *logUrl == "" {
		log.Info("logUrl argument must not be empty.")
		os.Exit(-1)
	}

	logUrlParsed, err := url.Parse(*logUrl)
	if err != nil {
		log.Info("Malformed log_url", "URL", *logUrl)
		os.Exit(-1)
	}
	var loggingType v1alpha2.LoggerMode
	if *logMode == string(v1alpha2.LogAll) {
		loggingType = v1alpha2.LogAll
	} else if *logMode == string(v1alpha2.LogRequest) {
		loggingType = v1alpha2.LogRequest
	} else if *logMode == string(v1alpha2.LogResponse) {
		loggingType = v1alpha2.LogResponse
	} else {
		log.Info("Malformed log_type", "type", *logMode)
		os.Exit(-1)
	}

	if *sample < 0 || *sample > 1.0 {
		log.Info("Malformed sample", "value", *sample)
		os.Exit(-1)
	}

	if *sourceUri == "" {
		*sourceUri = "http://localhost:" + *port + "/"
	}
	sourceUriParsed, err := url.Parse(*sourceUri)
	if err != nil {
		log.Info("Malformed source_uri", "URL", *sourceUri)
		os.Exit(-1)
	}

	modelUriParsed, err := url.Parse(*modelUri)
	if err != nil {
		if *modelUri != "" {
			log.Info("Malformed model_uri", "URL", *modelUri)
			os.Exit(-1)
		}
	}

	stopCh := signals.SetupSignalHandler()

	var eh http.Handler = logger.New(log, *svcPort, logUrlParsed, sourceUriParsed, loggingType, *sample, modelUriParsed)

	h1s := &http.Server{
		Addr:    ":" + *port,
		Handler: h2c.NewHandler(eh, &http2.Server{}),
	}

	fmt.Println("Starting the dispatcher")
	logger.StartDispatcher(*workers, log)

	log.Info("Starting", "port", *port)

	errCh := make(chan error, 1)
	go func(name string, s *http.Server) {
		// Don't forward ErrServerClosed as that indicates we're already shutting down.
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- errors.Wrapf(err, "%s server failed", name)
		}
	}("default", h1s)

	// Exit as soon as we see a shutdown signal or the server failed.
	select {
	case <-stopCh:
	case err := <-errCh:
		log.Error(err, "Failed to run HTTP server")
	}

	err = h1s.Shutdown(context.Background())
	if err != nil {
		log.Error(err, "Failed to shutdown HTTP server")
	}

}
