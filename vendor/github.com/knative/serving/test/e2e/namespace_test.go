// +build e2e

/*
Copyright 2019 The Knative Authors

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
	"fmt"
	"testing"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/test"
)

func checkResponse(t *testing.T, clients *test.Clients, names test.ResourceNames, expectedText string) error {
	_, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		names.Domain,
		test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.EventuallyMatchesBody(expectedText))),
		"WaitForEndpointToServeText",
		test.ServingFlags.ResolvableDomain)
	if err != nil {
		return fmt.Errorf("the endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, names.Domain, expectedText, err)
	}

	return nil
}

func TestMultipleNamespace(t *testing.T) {
	t.Parallel()

	altServiceNamespace := fmt.Sprintf("%s-%s", test.ServingNamespace, test.ObjectNameForTest(t))
	serviceName := test.ObjectNameForTest(t)

	defaultClients := Setup(t) // This one uses the default namespace `test.ServingNamespace`
	altClients := SetupWithNamespace(t, altServiceNamespace)

	defer test.DeleteNamespace(altClients, altServiceNamespace)
	err := test.CreateNamespace(altClients, altServiceNamespace)
	if err != nil {
		t.Fatal(err)
	}

	defaultResources := test.ResourceNames{
		Service: serviceName,
		Image:   pizzaPlanet1,
	}
	defer test.TearDown(defaultClients, defaultResources)
	if _, err := test.CreateRunLatestServiceReady(t, defaultClients, &defaultResources, &test.Options{}); err != nil {
		t.Fatalf("Failed to create Service %v in namespace %v: %v", defaultResources.Service, test.ServingNamespace, err)
	}

	altResources := test.ResourceNames{
		Service: serviceName,
		Image:   pizzaPlanet2,
	}
	defer test.TearDown(altClients, altResources)
	if _, err := test.CreateRunLatestServiceReady(t, altClients, &altResources, &test.Options{}); err != nil {
		t.Fatalf("Failed to create Service %v in namespace %v: %v", altResources.Service, altServiceNamespace, err)
	}

	if err := checkResponse(t, defaultClients, defaultResources, pizzaPlanetText1); err != nil {
		t.Error(err)
	}

	if err := checkResponse(t, altClients, altResources, pizzaPlanetText2); err != nil {
		t.Error(err)
	}
}
