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

package conformance

import (
	"context"
	"math"
	"strings"
	"testing"

	_ "github.com/knative/pkg/system/testing"
	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/test"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (

	// This test uses the two pizza planet test images for the blue and green deployment.
	expectedBlue  = pizzaPlanetText1
	expectedGreen = pizzaPlanetText2
)

// TestBlueGreenRoute verifies that a route configured with a 50/50 traffic split
// between two revisions will (approximately) route traffic evenly between them.
// Also, traffic that targets revisions *directly* will be routed to the correct
// revision 100% of the time.
func TestBlueGreenRoute(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	var imagePaths []string
	imagePaths = append(imagePaths, pkgTest.ImagePath(pizzaPlanet1))
	imagePaths = append(imagePaths, pkgTest.ImagePath(pizzaPlanet2))

	var names, blue, green test.ResourceNames
	// Set Service and Image for names to create the initial service
	names.Service = test.ObjectNameForTest(t)
	names.Image = pizzaPlanet1

	// Set names for traffic targets to make them directly routable.
	blue.TrafficTarget = "blue"
	green.TrafficTarget = "green"

	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	// Setup Initial Service
	t.Log("Creating a new Service in runLatest")
	objects, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{})
	if err != nil {
		t.Fatalf("Failed to create initial Service: %v: %v", names.Service, err)
	}

	// The first revision created is "blue"
	blue.Revision = names.Revision

	t.Log("Updating to a Manual Service to allow configuration and route to be manually modified")
	svc, err := test.PatchManualService(t, clients, objects.Service)
	if err != nil {
		t.Fatalf("Failed to update Service %s: %v", names.Service, err)
	}
	objects.Service = svc

	t.Log("Updating the Configuration to use a different image")
	cfg, err := test.PatchConfigImage(clients, objects.Config, imagePaths[1])
	if err != nil {
		t.Fatalf("Patch update for Configuration %s with new image %s failed: %v", names.Config, imagePaths[1], err)
	}
	objects.Config = cfg

	t.Log("Since the Configuration was updated a new Revision will be created and the Configuration will be updated")
	green.Revision, err = test.WaitForConfigLatestRevision(clients, names)
	if err != nil {
		t.Fatalf("Configuration %s was not updated with the Revision for image %s: %v", names.Config, imagePaths[1], err)
	}

	t.Log("Updating Route")
	if _, err := test.UpdateBlueGreenRoute(t, clients, names, blue, green); err != nil {
		t.Fatalf("Failed to create Route: %v", err)
	}

	t.Log("Wait for the route domains to be ready")
	if err := test.WaitForRouteState(clients.ServingClient, names.Route, test.IsRouteReady, "RouteIsReady"); err != nil {
		t.Fatalf("The Route %s was not marked as Ready to serve traffic: %v", names.Route, err)
	}

	route, err := clients.ServingClient.Routes.Get(names.Route, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error fetching Route %s: %v", names.Route, err)
	}

	var blueDomain, greenDomain string
	for _, tt := range route.Status.Traffic {
		if tt.Name == blue.TrafficTarget {
			// Strip prefix as WaitForEndPointState expects a domain
			// without scheme.
			blueDomain = strings.TrimPrefix(tt.URL, "http://")
		}
		if tt.Name == green.TrafficTarget {
			// Strip prefix as WaitForEndPointState expects a domain
			// without scheme.
			greenDomain = strings.TrimPrefix(tt.URL, "http://")
		}
	}
	if blueDomain == "" || greenDomain == "" {
		t.Fatalf("Unable to fetch URLs from traffic targets: %#v", route.Status.Traffic)
	}
	tealDomain := route.Status.Domain

	// Istio network programming takes some time to be effective.  Currently Istio
	// does not expose a Status, so we rely on probes to know when they are effective.
	// Since we are updating the route the teal domain probe will succeed before our changes
	// take effect so we probe the green domain.
	t.Logf("Probing domain %s", greenDomain)
	if _, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		greenDomain,
		test.RetryingRouteInconsistency(pkgTest.IsStatusOK),
		"WaitForSuccessfulResponse",
		test.ServingFlags.ResolvableDomain); err != nil {
		t.Fatalf("Error probing domain %s: %v", greenDomain, err)
	}

	// Send concurrentRequests to blueDomain, greenDomain, and tealDomain.
	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		min := int(math.Floor(concurrentRequests * minSplitPercentage))
		return checkDistribution(t, clients, tealDomain, concurrentRequests, min, []string{expectedBlue, expectedGreen})
	})
	g.Go(func() error {
		min := int(math.Floor(concurrentRequests * minDirectPercentage))
		return checkDistribution(t, clients, blueDomain, concurrentRequests, min, []string{expectedBlue})
	})
	g.Go(func() error {
		min := int(math.Floor(concurrentRequests * minDirectPercentage))
		return checkDistribution(t, clients, greenDomain, concurrentRequests, min, []string{expectedGreen})
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("Error sending requests: %v", err)
	}
}
