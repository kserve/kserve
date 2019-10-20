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

	"github.com/kubeflow/kfserving/pkg/inferencelogger"
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
	logType   = flag.String("log_type", "both", "Whether to log request, response or both")
	sample    = flag.Float64("sample", 1.0, "Probability to emit a log event")
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
	var loggingType v1alpha2.InferenceLoggerType
	if *logType == "both" {
		loggingType = v1alpha2.InferenceLogBoth
	} else if *logType == "request" {
		loggingType = v1alpha2.InferenceLogRequest
	} else if *logType == "response" {
		loggingType = v1alpha2.InferenceLogresponse
	} else {
		log.Info("Malformed log_type", "type", *logType)
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

	stopCh := signals.SetupSignalHandler()

	var eh http.Handler = inferencelogger.New(log, *svcPort, logUrlParsed, sourceUriParsed, loggingType, *sample)

	h1s := &http.Server{
		Addr:    ":" + *port,
		Handler: h2c.NewHandler(eh, &http2.Server{}),
	}

	fmt.Println("Starting the dispatcher")
	inferencelogger.StartDispatcher(*workers, log)

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
