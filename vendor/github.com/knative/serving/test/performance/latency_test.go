// +build performance

/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// latency_test.go brings up a helloworld app and gets the latency metric

package performance

import (
	"fmt"
	"testing"
	"time"

	pkgTest "github.com/knative/pkg/test"
	ingress "github.com/knative/pkg/test/ingress"
	"github.com/knative/serving/test"
	"github.com/knative/test-infra/shared/junit"
	"github.com/knative/test-infra/shared/loadgenerator"
	"github.com/knative/test-infra/shared/testgrid"
)

const (
	sleepTime = 1 * time.Minute
	// sleepReqTimeout should be > sleepTime. Else, the request will time out before receiving the response
	sleepReqTimeout = 2 * time.Minute
	hwReqtimeout    = 30 * time.Second
)

func timeToServe(t *testing.T, img, query string, reqTimeout time.Duration) {
	t.Helper()
	tName := t.Name()
	perfClients, err := Setup(t)
	if err != nil {
		t.Fatalf("Cannot initialize performance client: %v", err)
	}

	clients := perfClients.E2EClients
	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   img,
	}

	defer TearDown(perfClients, names, t.Logf)
	test.CleanupOnInterrupt(func() { TearDown(perfClients, names, t.Logf) })

	t.Log("Creating a new Service")
	objs, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{})
	if err != nil {
		t.Fatalf("Failed to create Service: %v", err)
	}

	domain := objs.Route.Status.Domain
	endpoint, err := ingress.GetIngressEndpoint(clients.KubeClient.Kube)
	if err != nil {
		t.Fatalf("Cannot get service endpoint: %v", err)
	}

	if _, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		domain,
		test.RetryingRouteInconsistency(pkgTest.IsStatusOK),
		"WaitForSuccessfulResponse",
		test.ServingFlags.ResolvableDomain); err != nil {
		t.Fatalf("Error probing domain %s: %v", domain, err)
	}

	opts := loadgenerator.GeneratorOptions{
		Duration:       duration,
		NumThreads:     1,
		NumConnections: 5,
		Domain:         domain,
		URL:            fmt.Sprintf("http://%s/?%s", *endpoint, query),
		RequestTimeout: reqTimeout,
		LoadFactors:    []float64{1},
	}
	resp, err := opts.RunLoadTest(false)
	if err != nil {
		t.Fatalf("Generating traffic via fortio failed: %v", err)
	}

	// Save the json result for benchmarking
	resp.SaveJSON(tName)

	if len(resp.Result) == 0 {
		t.Fatalf("No result found for the load test")
	}

	// Add latency metrics
	var tc []junit.TestCase
	for _, p := range resp.Result[0].DurationHistogram.Percentiles {
		val := float32(p.Value) * 1000
		name := fmt.Sprintf("p%d(ms)", int(p.Percentile))
		tc = append(tc, CreatePerfTestCase(val, name, tName))
	}

	if err = testgrid.CreateXMLOutput(tc, tName); err != nil {
		t.Fatalf("Cannot create output xml: %v", err)
	}
}

// Performs perf test on the hello world app
func TestTimeToServeLatency(t *testing.T) {
	timeToServe(t, "helloworld", "", hwReqtimeout)
}

// Performs perf testing on a long running app.
// It uses the timeout app that sleeps for the specified amount of time.
func TestTimeToServeLatencyLongRunning(t *testing.T) {
	q := fmt.Sprintf("timeout=%d", sleepTime/time.Millisecond)
	timeToServe(t, "timeout", q, sleepReqTimeout)
}
