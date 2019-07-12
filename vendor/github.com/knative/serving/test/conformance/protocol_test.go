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

package conformance

import (
	"encoding/json"
	"fmt"
	"testing"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/pkg/test/spoof"
	"github.com/knative/serving/test"
	corev1 "k8s.io/api/core/v1"
)

type protocolsTest struct {
	t       *testing.T
	clients *test.Clients
	names   test.ResourceNames
}

func (pt *protocolsTest) setup(t *testing.T) {
	pt.t = t

	pt.clients = setup(t)
	pt.names = test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   protocols,
	}

	test.CleanupOnInterrupt(func() { pt.teardown() })
}

func (pt *protocolsTest) teardown() {
	test.TearDown(pt.clients, pt.names)
}

func (pt *protocolsTest) getProtocol(resp *spoof.Response) protocol {
	var got protocol

	err := json.Unmarshal(resp.Body, &got)
	if err != nil {
		pt.t.Fatalf("Can't unmarshal response %s: %v", resp.Body, err)
	}

	pt.t.Logf("Parsed version: %q", got.String())

	return got
}

func (pt *protocolsTest) makeRequest(domain string) *spoof.Response {
	pt.t.Logf("Making request to %q", domain)

	resp, err := pkgTest.WaitForEndpointState(
		pt.clients.KubeClient, pt.t.Logf, domain,
		test.RetryingRouteInconsistency(pkgTest.IsStatusOK),
		pt.t.Name(), test.ServingFlags.ResolvableDomain,
	)
	if err != nil {
		pt.t.Fatalf("Failed to get a successful request from %s: %v", domain, err)
	}

	pt.t.Logf("Got response: %s", resp.Body)

	return resp
}

func (pt *protocolsTest) createService(options *test.Options) string {
	pt.t.Logf("Creating service %q with options: %#v", pt.names.Service, options)

	objects, err := test.CreateRunLatestServiceReady(pt.t, pt.clients, &pt.names, options)
	if err != nil {
		pt.t.Fatalf("Failed to create service %v", err)
	}

	return objects.Route.Status.Domain
}

type protocol struct {
	Major int `json:"protoMajor"`
	Minor int `json:"protoMinor"`
}

func (p *protocol) String() string {
	return fmt.Sprintf("HTTP/%d.%d", p.Major, p.Minor)
}

func portOption(portname string) *test.Options {
	options := &test.Options{}

	if portname != "" {
		options.ContainerPorts = []corev1.ContainerPort{{Name: portname, ContainerPort: 8080}}
	}

	return options
}

func TestProtocols(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name     string
		PortName string
		Want     protocol
	}{{
		Name:     "h2c",
		PortName: "h2c",
		Want:     protocol{Major: 2, Minor: 0},
	}, {
		Name:     "http1",
		PortName: "http1",
		Want:     protocol{Major: 1, Minor: 1},
	}, {
		Name:     "default",
		PortName: "",
		Want:     protocol{Major: 1, Minor: 1},
	}}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			var pt protocolsTest

			pt.setup(t)
			defer pt.teardown()

			options := portOption(tt.PortName)
			domain := pt.createService(options)

			response := pt.makeRequest(domain)
			got := pt.getProtocol(response)

			if got != tt.Want {
				t.Errorf("Want %s, got %s", tt.Want.String(), got.String())
			}
		})
	}
}
