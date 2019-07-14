// +build e2e

/*
Copyright 2018 The Knative Authors

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

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/build/pkg/apis/build/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/test"
	"go.opencensus.io/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	interval = 1 * time.Second
	timeout  = 5 * time.Minute
)

// TestCancelledBuild tests that a build with a long running step
// so that we have time to cancel it.
func TestCancelledBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "cancelled-build"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image:   "ubuntu",
				Command: []string{"bash"},
				Args:    []string{"-c", "sleep 5000"}, // fails.
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	// Wait for the build to be running
	if err := waitForBuildState(clients, buildName, func(b *v1alpha1.Build) (bool, error) {
		c := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue || c.Status == corev1.ConditionFalse {
				return true, fmt.Errorf("build %s already finished!", buildName)
			} else if c.Status == corev1.ConditionUnknown && (c.Reason == "Building" || c.Reason == "Pending") {
				return true, nil
			}
		}
		return false, nil
	}, "ToBeCancelledBuildRunning"); err != nil {
		t.Fatalf("Error waiting for build %q to be running: %v", buildName, err)
	}

	b, err := clients.buildClient.builds.Get(buildName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error getting build %s: %v", buildName, err)
	}
	b.Spec.Status = v1alpha1.BuildSpecStatusCancelled
	if _, err := clients.buildClient.builds.Update(b); err != nil {
		t.Fatalf("Error update build status for %s: %v", buildName, err)
	}

	b, err = clients.buildClient.watchBuild(buildName)
	if err == nil {
		t.Fatalf("watchBuild did not return expected `cancelled` error")
	}
	if d := cmp.Diff(b.Status.GetCondition(duckv1alpha1.ConditionSucceeded), &duckv1alpha1.Condition{
		Type:    duckv1alpha1.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildCancelled",
		Message: fmt.Sprintf("Build %q was cancelled", b.Name),
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %s", d)
	}
}

func waitForBuildState(c *clients, name string, inState func(b *v1alpha1.Build) (bool, error), desc string) error {
	metricName := fmt.Sprintf("WaitForBuildState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		r, err := c.buildClient.builds.Get(name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(r)
	})
}
