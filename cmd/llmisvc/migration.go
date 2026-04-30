/*
Copyright 2025 The KServe Authors.

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
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

// fastMigrationBackoff is the phase-1 exponential backoff configuration.
// Cap must not be set: setting Cap zeroes Steps on first hit, causing early exit.
var fastMigrationBackoff = wait.Backoff{
	Duration: 2 * time.Second,
	Factor:   1.5,
	Jitter:   0.1,
	Steps:    10,
}

// runMigrationWithRetry attempts migrate using a two-phase retry strategy against
// a total deadline of timeout. Phase 1 uses exponential backoff for quick initial
// detection. Phase 2 switches to fixed-interval polling for the remaining budget.
// Fatal Kubernetes API errors (Forbidden, Unauthorized, NotFound) are not retried.
// log should be pre-keyed with the resource name for per-attempt retry messages.
func runMigrationWithRetry(ctx context.Context, timeout, pollInterval time.Duration, log logr.Logger, migrate func(context.Context) error) error {
	return runMigrationWithRetryBackoff(ctx, timeout, pollInterval, fastMigrationBackoff, log, migrate)
}

// runMigrationWithRetryBackoff is the internal implementation. phase1Backoff is
// separated from the public API so tests can pass tiny durations without sleeping.
func runMigrationWithRetryBackoff(ctx context.Context, timeout, pollInterval time.Duration, phase1Backoff wait.Backoff, log logr.Logger, migrate func(context.Context) error) error {
	// Child context with deadline. Using ctx (not context.Background()) so SIGTERM
	// from the manager still propagates and migration aborts cleanly on shutdown.
	retryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	condition := func(ctx context.Context) (bool, error) {
		if err := migrate(ctx); err != nil {
			lastErr = err
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err) {
				return false, err
			}
			log.Error(err, "storage version migration attempt failed, retrying")
			return false, nil
		}
		return true, nil
	}

	// reportErr classifies the final error into one of three categories:
	//   - timed out: wait was interrupted AND the deadline was exceeded
	//   - cancelled: wait was interrupted AND the parent context was cancelled
	//   - failed: a non-retryable (fatal) error was returned by condition
	//
	// Gating on wait.Interrupted(err) rather than retryCtx.Err() != nil prevents
	// a fatal API error (e.g. 403 Forbidden) from being misreported as a timeout if
	// the deadline happens to expire at the same instant.
	reportErr := func(err error) error {
		if wait.Interrupted(err) && retryCtx.Err() != nil {
			underlying := lastErr
			if underlying == nil {
				underlying = retryCtx.Err()
			}
			if errors.Is(retryCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out after %s: %w", timeout, underlying)
			}
			return fmt.Errorf("cancelled: %w", underlying)
		}
		return fmt.Errorf("failed: %w", err)
	}

	// Phase 1: exponential backoff for quick initial detection.
	if err := wait.ExponentialBackoffWithContext(retryCtx, phase1Backoff, condition); err == nil {
		return nil
	} else if !wait.Interrupted(err) {
		return reportErr(err)
	}

	// Phase 2: fixed-interval polling for the remaining timeout budget.
	// If budget is already exhausted after phase 1, skip the log and return
	// immediately - avoids a misleading "switching to steady-state polling" message
	// when PollUntilContextCancel would return instantly anyway.
	if retryCtx.Err() != nil {
		return reportErr(retryCtx.Err())
	}
	// immediate=false: phase 1 already attempted on its last step.
	log.Info("fast migration retries exhausted, switching to steady-state polling",
		"pollInterval", pollInterval)
	if err := wait.PollUntilContextCancel(retryCtx, pollInterval, false, condition); err != nil {
		return reportErr(err)
	}
	return nil
}
