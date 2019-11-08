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
	logUrl        = flag.String("log-url", "", "The URL to send request/response logs to")
	port          = flag.String("port", "8080", "Logger port")
	componentHost = flag.String("component-host", "0.0.0.0", "Component host")
	componentPort = flag.String("component-port", "8081", "Component port")
	workers       = flag.Int("workers", 5, "Number of workers")
	sourceUri     = flag.String("source-uri", "", "The source URI to use when publishing cloudevents")
	logMode       = flag.String("log-mode", string(v1alpha2.LogAll), "Whether to log 'request', 'response' or 'all'")
	modelId       = flag.String("model-id", "", "The model ID to add as header to log events")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	if *logUrl == "" {
		log.Info("log-url argument must not be empty.")
		os.Exit(-1)
	}

	logUrlParsed, err := url.Parse(*logUrl)
	if err != nil {
		log.Info("Malformed log-url", "URL", *logUrl)
		os.Exit(-1)
	}
	loggingMode := v1alpha2.LoggerMode(*logMode)
	switch loggingMode {
	case v1alpha2.LogAll, v1alpha2.LogRequest, v1alpha2.LogResponse:
	default:
		log.Info("Malformed log-mode", "mode", *logMode)
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

	stopCh := signals.SetupSignalHandler()

	var eh http.Handler = logger.New(log, *componentHost, *componentPort, logUrlParsed, sourceUriParsed, loggingMode, *modelId)

	h1s := &http.Server{
		Addr:    ":" + *port,
		Handler: h2c.NewHandler(eh, &http2.Server{}),
	}

	log.Info("Starting the log dispatcher")
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
