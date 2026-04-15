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
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

// testPhase1Backoff replaces the 2s/10-step production backoff with 1ms/3-step
// so unit tests run in milliseconds without sleep.
var testPhase1Backoff = wait.Backoff{
	Duration: 1 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0,
	Steps:    3,
}

const testPollInterval = 1 * time.Millisecond

// retry is a test helper that calls runMigrationWithRetryBackoff with tiny
// backoff durations so tests run fast.
func retry(ctx context.Context, timeout time.Duration, migrate func(context.Context) error) error {
	return runMigrationWithRetryBackoff(ctx, timeout, testPollInterval, testPhase1Backoff, logr.Discard(), migrate)
}

func TestRunMigration_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := retry(context.Background(), time.Hour, func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRunMigration_SuccessAfterTransientFailures(t *testing.T) {
	calls := 0
	err := retry(context.Background(), time.Hour, func(_ context.Context) error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil after transient failures, got: %v", err)
	}
}

func TestRunMigration_Phase2Fallthrough(t *testing.T) {
	// Phase 1 has Steps=3. Failing those forces a fallthrough to phase 2.
	calls := 0
	err := retry(context.Background(), time.Hour, func(_ context.Context) error {
		calls++
		if calls <= testPhase1Backoff.Steps {
			return errors.New("not ready yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil after phase 2 success, got: %v", err)
	}
	if calls <= testPhase1Backoff.Steps {
		t.Fatalf("expected migration to succeed in phase 2 (calls > %d), got %d",
			testPhase1Backoff.Steps, calls)
	}
}

func TestRunMigration_FatalErrorShortCircuits(t *testing.T) {
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "test", Resource: "things"}, "name", errors.New("forbidden"))
	calls := 0
	err := retry(context.Background(), time.Hour, func(_ context.Context) error {
		calls++
		return forbidden
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call before fatal error short-circuits, got %d", calls)
	}
	if !errors.Is(err, forbidden) {
		t.Fatalf("expected error to wrap forbidden, got: %v", err)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("fatal API error must not be reported as timeout, got: %q", err.Error())
	}
}

func TestRunMigration_TimeoutReportsLastError(t *testing.T) {
	sentinel := errors.New("always failing")
	err := retry(context.Background(), 20*time.Millisecond, func(_ context.Context) error {
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected error to wrap sentinel, got: %v", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected 'timed out' in error message, got: %q", err.Error())
	}
}

func TestRunMigration_CancelledReportsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	err := retry(ctx, time.Hour, func(_ context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return errors.New("transient")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("cancelled context must not produce 'timed out', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected 'cancelled' in error message, got: %q", err.Error())
	}
}

func TestRunMigration_FatalErrorNotMisclassifiedAsTimeout(t *testing.T) {
	// Regression guard: previously reportErr gated on `lastErr != nil && ctx.Err() != nil`,
	// which could classify a fatal Forbidden error as "timed out" if the context deadline
	// happened to expire at the same instant. The fix gates on wait.Interrupted(err).
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "test", Resource: "things"}, "name", errors.New("forbidden"))

	// Use a 1-step backoff so the function returns immediately with the fatal error.
	// The timeout is short enough that the context may already be expired on return,
	// which is the scenario the old code mishandled.
	err := runMigrationWithRetryBackoff(
		context.Background(), 5*time.Millisecond, testPollInterval,
		wait.Backoff{Duration: 1 * time.Millisecond, Factor: 1, Steps: 1},
		logr.Discard(),
		func(_ context.Context) error { return forbidden },
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("fatal API error must not be classified as timeout, got: %q", err.Error())
	}
	if !errors.Is(err, forbidden) {
		t.Fatalf("expected error to wrap forbidden, got: %v", err)
	}
}

func TestRunMigration_PreExpiredContextReportsTimeout(t *testing.T) {
	// Simulates resource 1 consuming the entire shared budget; resource 2's call
	// receives an already-expired context. Must classify as "timed out", not panic.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	err := retry(ctx, time.Hour, func(_ context.Context) error { return errors.New("never called") })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("pre-expired context must report 'timed out', got: %q", err.Error())
	}
}
