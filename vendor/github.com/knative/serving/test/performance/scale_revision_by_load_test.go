// +build performance

/*
Copyright 2019 The Knative Authors

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

package performance

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/knative/pkg/controller"
	pkgTest "github.com/knative/pkg/test"
	ingress "github.com/knative/pkg/test/ingress"
	testingv1alpha1 "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
	"github.com/knative/serving/pkg/resources"
	"github.com/knative/serving/test"
	"github.com/knative/test-infra/shared/junit"
	"github.com/knative/test-infra/shared/loadgenerator"
	"github.com/knative/test-infra/shared/testgrid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	qpsPerClient         = 10               // frequencies of requests per client
	iterationDuration    = 60 * time.Second // iteration duration for a single scale
	processingTimeMillis = 100              // delay of each request on "server" side
	targetConcurrency    = 10
)

var concurrentClients = []int{10, 20, 40, 80, 160, 320}

type scaleEvent struct {
	oldScale  int
	newScale  int
	timestamp time.Time
}

// TestScaleRevisionByLoad performs several iterations with increasing number of clients
// while measuring response times, error rates, and time to scale up.
func TestScaleRevisionByLoad(t *testing.T) {
	tc := make([]junit.TestCase, 0)
	for _, numClients := range concurrentClients {
		t.Run(fmt.Sprintf("clients-%03d", numClients), func(t *testing.T) {
			tc = append(tc, scaleRevisionByLoad(t, numClients)...)
		})
	}
	if err := testgrid.CreateXMLOutput(tc, t.Name()); err != nil {
		t.Fatalf("Cannot create output XML: %v", err)
	}
}

func scaleRevisionByLoad(t *testing.T, numClients int) []junit.TestCase {
	perfClients, err := Setup(t)
	if err != nil {
		t.Fatalf("Cannot initialize performance client: %v", err)
	}

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   "observed-concurrency",
	}
	clients := perfClients.E2EClients

	defer TearDown(perfClients, names, t.Logf)
	test.CleanupOnInterrupt(func() { TearDown(perfClients, names, t.Logf) })

	t.Log("Creating a new Service")
	objs, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{
		ContainerResources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("20Mi"),
			},
		}},
		testingv1alpha1.WithConfigAnnotations(map[string]string{"autoscaling.knative.dev/target": fmt.Sprintf("%d", targetConcurrency)}),
	)
	if err != nil {
		t.Fatalf("Failed to create Service: %v", err)
	}

	domain := objs.Route.Status.Domain
	endpoint, err := ingress.GetIngressEndpoint(clients.KubeClient.Kube)
	if err != nil {
		t.Fatalf("Cannot get service endpoint: %v", err)
	}

	// Make sure we are ready to serve.
	st := time.Now()
	t.Log("Starting to probe the endpoint at", st)
	_, err = pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		domain+"/?timeout=10", // To generate any kind of a valid response.
		test.RetryingRouteInconsistency(pkgTest.IsStatusOK),
		"WaitForEndpointToServeText",
		test.ServingFlags.ResolvableDomain)
	if err != nil {
		t.Fatalf("The endpoint at domain %s didn't serve the expected response: %v", domain, err)
	}
	t.Logf("Took %v for the endpoint to start serving", time.Since(st))

	// The number of scale events should be at most ~numClients/targetConcurrency,
	// adding a big buffer to account for unexpected events
	scaleCh := make(chan *scaleEvent, numClients/targetConcurrency*10)
	stopCh := make(chan struct{})

	factory := informers.NewSharedInformerFactory(clients.KubeClient.Kube, 0)
	endpointsInformer := factory.Core().V1().Endpoints().Informer()
	endpointsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			newEndpoints := newObj.(*corev1.Endpoints)
			if strings.Contains(newEndpoints.GetName(), names.Service) {
				newNumAddresses := resources.ReadyAddressCount(newEndpoints)
				oldNumAddresses := resources.ReadyAddressCount(oldObj.(*corev1.Endpoints))
				if newNumAddresses != oldNumAddresses {
					event := &scaleEvent{
						oldScale:  oldNumAddresses,
						newScale:  newNumAddresses,
						timestamp: time.Now(),
					}
					select {
					case scaleCh <- event:
					default:
					}
				}
			}
		},
	})
	controller.StartInformers(stopCh, endpointsInformer)

	opts := loadgenerator.GeneratorOptions{
		Duration:       iterationDuration,
		NumThreads:     numClients,
		NumConnections: numClients,
		Domain:         domain,
		BaseQPS:        qpsPerClient * float64(numClients),
		URL:            fmt.Sprintf("http://%s/?timeout=%d", *endpoint, processingTimeMillis),
		LoadFactors:    []float64{1},
	}

	t.Logf("Starting test with %d clients at %s", numClients, time.Now())
	resp, err := opts.RunLoadTest(false)
	if err != nil {
		t.Fatalf("Generating traffic via fortio failed: %v", err)
	}

	close(stopCh)
	close(scaleCh)

	// Save the json result for benchmarking
	resp.SaveJSON(strings.Replace(t.Name(), "/", "_", -1))

	tc := make([]junit.TestCase, 0)

	tc = append(tc, CreatePerfTestCase(float32(resp.Result[0].DurationHistogram.Count), "requestCount", t.Name()))
	tc = append(tc, CreatePerfTestCase(float32(qpsPerClient*numClients), "requestedQPS", t.Name()))
	tc = append(tc, CreatePerfTestCase(float32(resp.Result[0].ActualQPS), "actualQPS", t.Name()))
	tc = append(tc, CreatePerfTestCase(float32(errorsPercentage(resp)), "errorsPercentage", t.Name()))

	for ev := range scaleCh {
		t.Logf("Scaled: %d -> %d in %v", ev.oldScale, ev.newScale, ev.timestamp.Sub(resp.Result[0].StartTime))
		tc = append(tc, CreatePerfTestCase(float32(ev.timestamp.Sub(resp.Result[0].StartTime)/time.Second), fmt.Sprintf("scale-from-%02d-to-%02d(seconds)", ev.oldScale, ev.newScale), t.Name()))
	}

	for _, p := range resp.Result[0].DurationHistogram.Percentiles {
		val := float32(p.Value) * 1000
		name := fmt.Sprintf("p%d(ms)", int(p.Percentile))
		tc = append(tc, CreatePerfTestCase(val, name, t.Name()))
	}

	return tc
}

func errorsPercentage(resp *loadgenerator.GeneratorResults) float64 {
	var successes, errors int64
	for retCode, count := range resp.Result[0].RetCodes {
		if retCode == http.StatusOK {
			successes = successes + count
		} else {
			errors = errors + count
		}
	}
	return float64(errors*100) / float64(errors+successes)
}
