package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	ohandler "github.com/kubeflow/kfserving/pkg/executor"
	perrors "github.com/pkg/errors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	predictor   = flag.String("predictor", "", "The FQDN of the predictor service")
	preprocess  = flag.String("preprocess", "", "The FQDN of the preprocess service")
	postprocess = flag.String("postprocess", "", "The FQDN of the postprocess service")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	stopCh := signals.SetupSignalHandler()

	var oh http.Handler = ohandler.New(log, *preprocess, *predictor, *postprocess)

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
