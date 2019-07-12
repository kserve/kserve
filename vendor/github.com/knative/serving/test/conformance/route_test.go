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
	"testing"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/test"
)

func assertResourcesUpdatedWhenRevisionIsReady(t *testing.T, clients *test.Clients, names test.ResourceNames, domain string, expectedGeneration, expectedText string) {
	t.Log("When the Route reports as Ready, everything should be ready.")
	if err := test.WaitForRouteState(clients.ServingClient, names.Route, test.IsRouteReady, "RouteIsReady"); err != nil {
		t.Fatalf("The Route %s was not marked as Ready to serve traffic to Revision %s: %v", names.Route, names.Revision, err)
	}

	// TODO(#1178): Remove "Wait" from all checks below this point.
	t.Log("Serves the expected data at the endpoint")

	_, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		domain,
		test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.EventuallyMatchesBody(expectedText))),
		"WaitForEndpointToServeText",
		test.ServingFlags.ResolvableDomain)
	if err != nil {
		t.Fatalf("The endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, domain, expectedText, err)
	}

	// We want to verify that the endpoint works as soon as Ready: True, but there are a bunch of other pieces of state that we validate for conformance.
	t.Log("The Revision will be marked as Ready when it can serve traffic")
	err = test.CheckRevisionState(clients.ServingClient, names.Revision, test.IsRevisionReady)
	if err != nil {
		t.Fatalf("Revision %s did not become ready to serve traffic: %v", names.Revision, err)
	}
	t.Log("The Revision will be annotated with the generation")
	err = test.CheckRevisionState(clients.ServingClient, names.Revision, test.IsRevisionAtExpectedGeneration(expectedGeneration))
	if err != nil {
		t.Fatalf("Revision %s did not have an expected annotation with generation %s: %v", names.Revision, expectedGeneration, err)
	}
	t.Log("Updates the Configuration that the Revision is ready")
	err = test.CheckConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		return c.Status.LatestReadyRevisionName == names.Revision, nil
	})
	if err != nil {
		t.Fatalf("The Configuration %s was not updated indicating that the Revision %s was ready: %v", names.Config, names.Revision, err)
	}
	t.Log("Updates the Route to route traffic to the Revision")
	err = test.CheckRouteState(clients.ServingClient, names.Route, test.AllRouteTrafficAtRevision(names))
	if err != nil {
		t.Fatalf("The Route %s was not updated to route traffic to the Revision %s: %v", names.Route, names.Revision, err)
	}
	t.Log("TODO: The Route is accessible from inside the cluster without external DNS")
	err = test.CheckRouteState(clients.ServingClient, names.Route, test.TODO_RouteTrafficToRevisionWithInClusterDNS)
	if err != nil {
		t.Fatalf("The Route %s was not able to route traffic to the Revision %s with in cluster DNS: %v", names.Route, names.Revision, err)
	}
}

func getRouteDomain(clients *test.Clients, names test.ResourceNames) (string, error) {
	var domain string

	err := test.WaitForRouteState(
		clients.ServingClient,
		names.Route,
		func(r *v1alpha1.Route) (bool, error) {
			domain = r.Status.Domain
			return domain != "", nil
		},
		"RouteDomain",
	)

	return domain, err
}

func TestRouteCreation(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	var objects test.ResourceObjects
	svcName := test.ObjectNameForTest(t)
	names := test.ResourceNames{
		Config:        svcName,
		Route:         svcName,
		TrafficTarget: svcName,
		Image:         pizzaPlanet1,
	}

	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	t.Log("Creating a new Route and Configuration")
	config, err := test.CreateConfiguration(t, clients, names, &test.Options{})
	if err != nil {
		t.Fatalf("Failed to create Configuration: %v", err)
	}
	objects.Config = config

	route, err := test.CreateRoute(t, clients, names)
	if err != nil {
		t.Fatalf("Failed to create Route: %v", err)
	}
	objects.Route = route

	t.Log("The Configuration will be updated with the name of the Revision")
	names.Revision, err = test.WaitForConfigLatestRevision(clients, names)
	if err != nil {
		t.Fatalf("Configuration %s was not updated with the new revision: %v", names.Config, err)
	}

	domain, err := getRouteDomain(clients, names)
	if err != nil {
		t.Fatalf("Failed to get domain from route %s: %v", names.Route, err)
	}

	t.Logf("The Route domain is: %s", domain)
	assertResourcesUpdatedWhenRevisionIsReady(t, clients, names, domain, "1", pizzaPlanetText1)

	// We start a prober at background thread to test if Route is always healthy even during Route update.
	prober := test.RunRouteProber(t, clients, domain)
	defer test.AssertProberDefault(t, prober)

	t.Log("Updating the Configuration to use a different image")
	objects.Config, err = test.PatchConfigImage(clients, objects.Config, pkgTest.ImagePath(pizzaPlanet2))
	if err != nil {
		t.Fatalf("Patch update for Configuration %s with new image %s failed: %v", names.Config, pizzaPlanet2, err)
	}

	t.Log("Since the Configuration was updated a new Revision will be created and the Configuration will be updated")
	names.Revision, err = test.WaitForConfigLatestRevision(clients, names)
	if err != nil {
		t.Fatalf("Configuration %s was not updated with the Revision for image %s: %v", names.Config, pizzaPlanet2, err)
	}

	assertResourcesUpdatedWhenRevisionIsReady(t, clients, names, domain, "2", pizzaPlanetText2)
}
