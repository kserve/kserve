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
	"net/http"
	"time"
)

// Aggregated endpoint paths exposed by the aggregation layer.
const (
	PathModels  = "/v1/models"
	PathHealth  = "/health"
	PathMetrics = "/metrics"
	PathLoad    = "/load"
)

// Backend is one model server that can be queried by the aggregator.
type Backend struct {
	// Name is a short identifier for the backend (e.g. service name).
	Name string
	// Namespace is an optional namespace qualifier for the backend.
	Namespace string
	// URL is the base URL used to reach the backend (scheme + host[+port][+path prefix]).
	URL string
	// Models are optional model identities associated with this backend.
	Models []string
}

// ID returns a stable backend identifier suitable for logs and merge labels.
func (b Backend) ID() string {
	if b.Namespace == "" {
		return b.Name
	}
	return b.Namespace + "/" + b.Name
}

// BackendResult is the outcome of querying one backend for one path.
type BackendResult struct {
	Backend    Backend
	StatusCode int
	Body       []byte
	Err        error
	Duration   time.Duration
}

// OK reports whether the backend responded with a 2xx status and no transport error.
func (r BackendResult) OK() bool {
	return r.Err == nil && r.StatusCode >= 200 && r.StatusCode <= 299
}

// BackendDiscovery lists backends that should participate in aggregation.
type BackendDiscovery interface {
	List(ctx context.Context) ([]Backend, error)
}

// BackendFilter selects which backends participate for a given request.
// Consumers inject header-based tenancy/auth filtering here; the default is a no-op.
// Compose auth, tenancy, or label selectors with ChainFilters.
type BackendFilter func(r *http.Request, backends []Backend) []Backend

// ChainFilters applies filters in order. Nil entries are skipped.
func ChainFilters(filters ...BackendFilter) BackendFilter {
	return func(r *http.Request, backends []Backend) []Backend {
		for _, f := range filters {
			if f == nil {
				continue
			}
			backends = f(r, backends)
		}
		return backends
	}
}

// Merger combines per-backend results into a single HTTP response body and status.
type Merger func(results []BackendResult) (body []byte, statusCode int, err error)

// Options configures an Aggregator.
type Options struct {
	// Discovery lists backends. Required.
	Discovery BackendDiscovery
	// Filter optionally narrows backends per request. Nil means include all.
	// Use ChainFilters to compose auth, tenancy, or label selectors into one Filter.
	Filter BackendFilter
	// Timeout is the per-backend request timeout applied via context. Zero defaults to 3s.
	Timeout time.Duration
	// HTTPClient is used for backend requests. Nil uses a default client with no
	// client-level timeout; Timeout is enforced on the request context instead.
	HTTPClient *http.Client
	// Mergers overrides default merge strategies keyed by request path.
	// Missing paths fall back to built-in mergers.
	Mergers map[string]Merger
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return 3 * time.Second
}
