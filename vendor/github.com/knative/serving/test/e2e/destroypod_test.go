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
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	rnames "github.com/knative/serving/pkg/reconciler/v1alpha1/revision/resources/names"
	"github.com/knative/serving/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeoutExpectedOutput  = "Slept for 0 milliseconds"
	revisionTimeoutSeconds = 45
	timeoutRequestDuration = 43 * time.Second
)

func TestDestroyPodInflight(t *testing.T) {
	t.Parallel()
	clients := Setup(t)

	t.Log("Creating a new Route and Configuration")
	names, err := CreateRouteAndConfig(t, clients, "timeout", &test.Options{RevisionTimeoutSeconds: revisionTimeoutSeconds})
	if err != nil {
		t.Fatalf("Failed to create Route and Configuration: %v", err)
	}
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	t.Log("When the Revision can have traffic routed to it, the Route is marked as Ready")
	if err := test.WaitForRouteState(clients.ServingClient, names.Route, test.IsRouteReady, "RouteIsReady"); err != nil {
		t.Fatalf("The Route %s was not marked as Ready to serve traffic: %v", names.Route, err)
	}

	route, err := clients.ServingClient.Routes.Get(names.Route, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error fetching Route %s: %v", names.Route, err)
	}
	domain := route.Status.Domain

	err = test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		if c.Status.LatestCreatedRevisionName != names.Revision {
			names.Revision = c.Status.LatestCreatedRevisionName
			return true, nil
		}
		return false, nil
	}, "ConfigurationUpdatedWithRevision")
	if err != nil {
		t.Fatalf("Error obtaining Revision's name %v", err)
	}

	_, err = pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		domain,
		test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.MatchesBody(timeoutExpectedOutput))),
		"TimeoutAppServesText",
		test.ServingFlags.ResolvableDomain)
	if err != nil {
		t.Fatalf("The endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, domain, helloWorldExpectedOutput, err)
	}

	client, err := pkgTest.NewSpoofingClient(clients.KubeClient, t.Logf, domain, test.ServingFlags.ResolvableDomain)
	if err != nil {
		t.Fatalf("Error creating spoofing client: %v", err)
	}

	// The timeout app sleeps for the time passed via the timeout query parameter in milliseconds
	timeoutRequestDurationInMillis := timeoutRequestDuration / time.Millisecond
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/?timeout=%d", domain, timeoutRequestDurationInMillis), nil)
	if err != nil {
		t.Fatalf("Error creating http request: %v", err)
	}

	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		t.Log("Sending in a long running request")
		res, err := client.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("Expected response to have status 200, had %d", res.StatusCode)
		}
		expectedBody := fmt.Sprintf("Slept for %d milliseconds", timeoutRequestDurationInMillis)
		gotBody := string(res.Body)
		if gotBody != expectedBody {
			return fmt.Errorf("Unexpected body, expected: %q got: %q", expectedBody, gotBody)
		}
		return nil
	})

	g.Go(func() error {
		// Give the request a bit of time to be established and reach the pod.
		time.Sleep(timeoutRequestDuration / 2)

		t.Log("Destroying the configuration (also destroys the pods)")
		return clients.ServingClient.Configs.Delete(names.Config, nil)
	})

	if err := g.Wait(); err != nil {
		t.Errorf("Something went wrong with the request: %v", err)
	}
}

const (
	// Give the pods plenty of time to disappear. It will take them at least 20 seconds to vanish
	// because we have a hard-coded sleep of 20 seconds before initiating the shutdown process.
	// This is still well below the 5 minutes it might take them to disappear max.
	maxTimeToDelete = 60 * time.Second
)

func TestDestroyPodTimely(t *testing.T) {
	t.Parallel()
	clients := Setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   "helloworld",
	}

	defer test.TearDown(clients, names)
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })

	objects, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{RevisionTimeoutSeconds: 5 * 60})
	if err != nil {
		t.Fatalf("Failed to create a service: %v", err)
	}

	start := time.Now()

	// Deleting the service will also delete all pods.
	clients.ServingClient.Services.Delete(names.Service, nil)

	// Wait until the pods have disappeared.
	deploymentName := rnames.Deployment(objects.Revision)
	pkgTest.WaitForPodListState(
		clients.KubeClient,
		func(p *v1.PodList) (bool, error) {
			for _, pod := range p.Items {
				if strings.Contains(pod.Name, deploymentName) {
					return false, nil
				}
			}
			return true, nil
		},
		"WaitForPodsToDisappear", test.ServingNamespace)

	timeToDelete := time.Since(start)
	if timeToDelete > maxTimeToDelete {
		t.Errorf("Time to delete pods = %v, want < %v", timeToDelete, maxTimeToDelete)
	}
}
