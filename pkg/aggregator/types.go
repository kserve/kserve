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
// It is not limited to LLMInferenceService; Kind records the source type when known.
type Backend struct {
	// Name is a short identifier for the backend (e.g. service name).
	Name string
	// Namespace is an optional namespace qualifier for the backend.
	Namespace string
	// URL is the base URL used to reach the backend (scheme + host[+port][+path prefix]).
	URL string
	// Models are optional model identities associated with this backend.
	Models []string
	// Kind is the source object kind when discovered from Kubernetes
	// (e.g. "LLMInferenceService"). Empty for generic backends.
	Kind string
	// Address is the status.addresses[].name this backend was mapped from
	// (e.g. "internal", "gateway-external"). Empty when not from an address list.
	Address string
}

// ID returns a stable backend identifier suitable for logs and merge labels.
func (b Backend) ID() string {
	id := b.Name
	if b.Namespace != "" {
		id = b.Namespace + "/" + b.Name
	}
	if b.Address != "" {
		return id + "@" + b.Address
	}
	return id
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
