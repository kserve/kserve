/*
Copyright 2020 The Knative Authors

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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	network "knative.dev/networking/pkg"
	"knative.dev/serving/pkg/queue"
	"knative.dev/serving/pkg/queue/health"
	"knative.dev/serving/pkg/queue/readiness"
)

const (
	healthURLPrefix = "http://127.0.0.1:"

	// The 25 millisecond retry interval is an unscientific compromise between wanting to get
	// started as early as possible while still wanting to give the container some breathing
	// room to get up and running.
	aggressivePollInterval = 25 * time.Millisecond

	badProbeTemplate = "unexpected probe header value: "
)

// As well as running as a long-running proxy server, the Queue Proxy can be
// run as an exec probe if the `--probe-period` flag is passed.
//
// In this mode, the exec probe (repeatedly) sends an HTTP request to the Queue
// Proxy server with a Probe header. The handler for this probe request
// (knativeProbeHandler) then performs the actual health check against the user
// container. The exec probe binary exits 0 (success) if it eventually gets a
// 200 status code back from the knativeProbeHandler, or 1 (fail) if it never
// does.
//
// The reason we use an exec probe to hit an HTTP endpoint on the Queue
// Proxy to then run the actual probe against the user container rather than
// using an HTTP probe directly is because an exec probe can be launched
// immediately after the container starts, whereas an HTTP probe would
// initially race with the server starting up and fail. The minimum retry
// period after this failure in upstream kubernetes is a second. The exec
// probe, on the other hand, automatically polls on an aggressivePollInterval
// until the HTTP endpoint responds with success. This allows us to get an
// initial readiness result much faster than the effective upstream Kubernetes
// minimum of 1 second.
func standaloneProbeMain(timeout time.Duration, transport http.RoundTripper, port string) (exitCode int) {
	servingPort, err := strconv.ParseUint(port, 10, 16 /*ports are 16 bit*/)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse queue port:", err)
		return 1
	}
	if servingPort == 0 {
		fmt.Fprintln(os.Stderr, "port must be a positive value, got 0")
		return 1
	}

	if timeout == 0 {
		timeout = readiness.PollTimeout
	}

	if err := probeQueueHealthPath(timeout, int(servingPort), transport); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func probeQueueHealthPath(timeout time.Duration, servingPort int, transport http.RoundTripper) error {
	url := healthURLPrefix + strconv.Itoa(servingPort)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("probe failed: error creating request: %w", err)
	}

	// Add the header to indicate this is a probe request.
	req.Header.Add(network.ProbeHeaderName, queue.Name)
	req.Header.Add(network.UserAgentKey, network.QueueProxyUserAgent)

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		lastErr error
		res     *http.Response
	)
	// Using PollImmediateUntil instead of PollImmediate because if timeout is reached while waiting for first
	// invocation of conditionFunc, it exits immediately without trying for a second time.
	timeoutErr := wait.PollImmediateUntil(aggressivePollInterval, func() (bool, error) {
		res, lastErr = httpClient.Do(req)
		if lastErr != nil {
			// Return nil error for retrying.
			return false, nil
		}

		defer func() {
			// Ensure body is read and closed to ensure connection can be re-used via keep-alive.
			// No point handling errors here, connection just won't be reused.
			io.Copy(ioutil.Discard, res.Body)
			res.Body.Close()
		}()

		// Fail readiness immediately rather than retrying if we get a header indicating we're shutting down.
		if health.IsHTTPProbeShuttingDown(res) {
			lastErr = errors.New("failing probe deliberately for shutdown")
			return false, lastErr
		}
		return health.IsHTTPProbeReady(res), nil
	}, ctx.Done())

	if lastErr != nil {
		return fmt.Errorf("failed to probe: %w", lastErr)
	}

	// An http.StatusOK was never returned during probing.
	if timeoutErr != nil {
		return errors.New("probe returned not ready")
	}

	return nil
}

// ProbeHandler returns a http.HandlerFunc that responds to health checks if the
// knative network probe header is passed, and otherwise delegates to the next handler.
func ProbeHandler(healthState *health.State, prober func() bool, isAggressive bool, tracingEnabled bool, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ph := network.KnativeProbeHeader(r)

		if ph == "" {
			next.ServeHTTP(w, r)
			return
		}

		if ph != queue.Name {
			http.Error(w, badProbeTemplate+ph, http.StatusBadRequest)
			return
		}

		if prober == nil {
			http.Error(w, "no probe", http.StatusInternalServerError)
			return
		}

		healthState.HandleHealthProbe(func() bool {
			if !prober() {
				return false
			}
			return true
		}, isAggressive, w)
	}
}
