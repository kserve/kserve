package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	activatorhandler "github.com/knative/serving/pkg/activator/handler"
	ohandler "github.com/kubeflow/kfserving/pkg/orchestrator/handler"
	perrors "github.com/pkg/errors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	serviceFQDN = flag.String("service", "", "The FQDN of the service to proxy")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	stopCh := signals.SetupSignalHandler()

	// Create handler chain
	// Note: innermost handlers are specified first, ie. the last handler in the chain will be executed first
	var oh http.Handler = ohandler.New(log, *serviceFQDN)
	oh = &activatorhandler.ProbeHandler{NextHandler: oh}
	oh = &ohandler.HealthHandler{Log: log, NextHandler: oh}

	port := 8080

	h1s := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h2c.NewHandler(oh, &http2.Server{}),
	}

	log.Info("Starting", "port", port)

	errCh := make(chan error, 1)
	go func(name string, s *http.Server) {
		// Don't forward ErrServerClosed as that indicates we're already shutting down.
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- perrors.Wrapf(err, "%s server failed", name)
		}
	}("default", h1s)

	// Exit as soon as we see a shutdown signal or the server failed.
	select {
	case <-stopCh:
	case err := <-errCh:
		log.Error(err, "Failed to run HTTP server")
	}

	h1s.Shutdown(context.Background())

}
