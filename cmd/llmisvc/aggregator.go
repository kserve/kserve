/*
Copyright 2026 The KServe Authors.

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
	"errors"
	"net/http"
	"time"

	"github.com/kserve/kserve/pkg/aggregator"
)

// aggregatorServer serves aggregated LLMInferenceService endpoints from the
// llmisvc controller process. It does not require leader election so every
// replica can answer read-only aggregation requests.
type aggregatorServer struct {
	addr    string
	handler http.Handler
}

func (s aggregatorServer) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		// ctx is already canceled; WithoutCancel keeps values while allowing a
		// shutdown deadline that is not immediately expired.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	setupLog.Info("Starting LLMInferenceService aggregator", "addr", s.addr)
	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s aggregatorServer) NeedLeaderElection() bool { return false }

func newAggregatorServer(addr string, timeout time.Duration, discovery aggregator.BackendDiscovery) (aggregatorServer, error) {
	agg, err := aggregator.New(aggregator.Options{
		Discovery: discovery,
		Timeout:   timeout,
	})
	if err != nil {
		return aggregatorServer{}, err
	}
	return aggregatorServer{addr: addr, handler: agg.Handler()}, nil
}
