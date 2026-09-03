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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// FanOut issues a GET to path on every backend concurrently and returns one result per backend.
// Per-backend deadlines are enforced via context; order of results matches backends.
func FanOut(ctx context.Context, client *http.Client, backends []Backend, path string, header http.Header) []BackendResult {
	results := make([]BackendResult, len(backends))
	if len(backends) == 0 {
		return results
	}

	var wg sync.WaitGroup
	wg.Add(len(backends))
	for i := range backends {
		go func() {
			defer wg.Done()
			results[i] = queryBackend(ctx, client, backends[i], path, header)
		}()
	}
	wg.Wait()
	return results
}

func queryBackend(ctx context.Context, client *http.Client, backend Backend, path string, header http.Header) BackendResult {
	start := time.Now()
	result := BackendResult{Backend: backend}

	target, err := joinURL(backend.URL, path)
	if err != nil {
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}
	// Forward inbound headers (including Authorization) so tenancy/auth
	// tokens on the aggregator request reach the model servers.
	for k, values := range header {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap
	result.StatusCode = resp.StatusCode
	result.Body = body
	result.Err = err
	result.Duration = time.Since(start)
	return result
}

func joinURL(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid backend URL %q: %w", base, err)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", path, err)
	}
	return u.ResolveReference(ref).String(), nil
}
