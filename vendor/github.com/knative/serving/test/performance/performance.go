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

package performance

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/pkg/test/logging"
	"github.com/knative/pkg/test/zipkin"
	"github.com/knative/serving/test"
	"github.com/knative/test-infra/shared/junit"
	"github.com/knative/test-infra/shared/prometheus"
	"github.com/knative/test-infra/shared/prow"

	// Mysteriously required to support GCP auth (required by k8s libs). Apparently just importing it is enough. @_@ side effects @_@. https://github.com/kubernetes/client-go/issues/242
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	istioNS      = "istio-system"
	monitoringNS = "knative-monitoring"
	gateway      = "istio-ingressgateway"
	// Property name used by testgrid.
	perfLatency = "perf_latency"
	duration    = 1 * time.Minute
	traceSuffix = "-Trace.json"
)

// Enable monitoring components
const (
	EnablePrometheus = iota
	EnableZipkinTracing
)

// Client is the client used in the performance tests.
type Client struct {
	E2EClients *test.Clients
	PromClient *prometheus.PromProxy
}

// traceFile is the name of the
var traceFile *os.File

// Setup creates all the clients that we need to interact with in our tests
func Setup(t *testing.T, monitoring ...int) (*Client, error) {
	clients, err := test.NewClients(pkgTest.Flags.Kubeconfig, pkgTest.Flags.Cluster, test.ServingNamespace)
	if err != nil {
		return nil, err
	}

	var p *prometheus.PromProxy
	for _, m := range monitoring {
		switch m {
		case EnablePrometheus:
			t.Log("Creating prometheus proxy client")
			p = &prometheus.PromProxy{Namespace: monitoringNS}
			p.Setup(clients.KubeClient.Kube, t.Logf)
		case EnableZipkinTracing:
			// Enable zipkin tracing
			zipkin.SetupZipkinTracing(clients.KubeClient.Kube, t.Logf)

			// Create file to store traces
			dir := prow.GetLocalArtifactsDir()
			if err := createDir(dir); nil != err {
				t.Log("Cannot create the artifacts dir. Will not log tracing.")
			} else {
				name := path.Join(dir, t.Name()+traceSuffix)
				t.Logf("Storing traces in %s", name)
				traceFile, err = os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					t.Log("Unable to create tracing file.")
				}
			}
		default:
			t.Log("No monitoring components enabled")
		}
	}

	return &Client{E2EClients: clients, PromClient: p}, nil
}

// TearDown cleans up resources used
func TearDown(client *Client, names test.ResourceNames, logf logging.FormatLogger) {
	test.TearDown(client.E2EClients, names)

	// Teardown prometheus client
	if client.PromClient != nil {
		client.PromClient.Teardown(logf)
	}

	// disable zipkin
	if traceFile != nil {
		zipkin.CleanupZipkinTracingSetup(logf)
		traceFile.Close()
	}
}

// CreatePerfTestCase creates a perf test case with the provided name and value
func CreatePerfTestCase(metricValue float32, metricName, testName string) junit.TestCase {
	tp := []junit.TestProperty{{Name: perfLatency, Value: fmt.Sprintf("%f", metricValue)}}
	tc := junit.TestCase{
		ClassName:  testName,
		Name:       fmt.Sprintf("%s/%s", testName, metricName),
		Properties: junit.TestProperties{Properties: tp}}
	return tc
}

// AddTrace gets the JSON zipkin trace for the traceId and stores it.
// https://github.com/openzipkin/zipkin-go/blob/master/model/span.go defines the struct for the JSON
func AddTrace(logf logging.FormatLogger, tName string, traceID string) {
	if traceFile == nil {
		logf("Trace file is not setup correctly. Exiting without adding trace")
		return
	}

	// Sleep to get traces
	time.Sleep(5 * time.Second)

	trace, err := zipkin.JSONTrace(traceID)
	if err != nil {
		logf("Skipping trace %s due to error: %v", traceID, err)
		return
	}

	if _, err := traceFile.WriteString(fmt.Sprintf("%s,\n", trace)); err != nil {
		logf("Cannot write to trace file: %v", err)
	}
}

// createDir creates dir if does not exist.
func createDir(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err = os.MkdirAll(dirPath, 0777); err != nil {
			return fmt.Errorf("Failed to create directory: %v", err)
		}
	}
	return nil
}
