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

package aggregator

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Aggregator lists backends via BackendDiscovery and serves aggregated endpoints.
type Aggregator struct {
	discovery BackendDiscovery
	filter    BackendFilter
	client    *http.Client
	timeout   time.Duration
	mergers   map[string]Merger
	log       logr.Logger
}

// New builds an Aggregator from Options.
func New(opts Options) (*Aggregator, error) {
	if opts.Discovery == nil {
		return nil, errors.New("discovery is required")
	}

	timeout := opts.timeout()
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	mergers := defaultMergers()
	for path, merger := range opts.Mergers {
		mergers[path] = merger
	}

	filter := opts.Filter
	if filter == nil {
		filter = func(_ *http.Request, backends []Backend) []Backend { return backends }
	}

	return &Aggregator{
		discovery: opts.Discovery,
		filter:    filter,
		client:    client,
		timeout:   timeout,
		mergers:   mergers,
		log:       logf.Log.WithName("aggregator"),
	}, nil
}

// Handler returns an http.Handler that serves aggregated paths.
// Unknown paths return 404.
func (a *Aggregator) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, path := range []string{PathModels, PathHealth, PathMetrics, PathLoad} {
		mux.HandleFunc(path, a.handle(path))
	}
	return mux
}

func (a *Aggregator) handle(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		backends, err := a.discovery.List(r.Context())
		if err != nil {
			a.log.Error(err, "Failed to list backends")
			http.Error(w, "failed to list backends", http.StatusInternalServerError)
			return
		}
		backends = a.filter(r, backends)

		ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
		defer cancel()

		results := FanOut(ctx, a.client, backends, path, r.Header)
		for _, res := range results {
			if res.Err != nil {
				a.log.Info("Backend request failed", "backend", res.Backend.ID(), "path", path, "error", res.Err.Error())
			} else if !res.OK() {
				a.log.Info("Backend returned non-success", "backend", res.Backend.ID(), "path", path, "status", res.StatusCode)
			}
		}

		merger, ok := a.mergers[path]
		if !ok {
			http.Error(w, "no merger configured", http.StatusInternalServerError)
			return
		}
		body, status, err := merger(results)
		if err != nil {
			a.log.Error(err, "Failed to merge backend responses", "path", path)
			http.Error(w, "failed to merge responses", http.StatusInternalServerError)
			return
		}

		contentType := "application/json"
		if path == PathMetrics {
			contentType = "text/plain; version=0.0.4; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			a.log.Error(err, "Failed to write response", "path", path)
		}
	}
}
