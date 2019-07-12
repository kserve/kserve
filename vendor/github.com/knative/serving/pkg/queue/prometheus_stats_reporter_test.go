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

package queue

import (
	"errors"
	"testing"

	"github.com/knative/serving/pkg/autoscaler"
	"github.com/prometheus/client_golang/prometheus"

	dto "github.com/prometheus/client_model/go"
)

const (
	namespace = "default"
	config    = "helloworld-go"
	revision  = "helloworld-go-00001"
	pod       = "helloworld-go-00001-deployment-8ff587cc9-7g9gc"
)

func TestNewPrometheusStatsReporter_negative(t *testing.T) {
	tests := []struct {
		name      string
		errorMsg  string
		result    error
		namespace string
		config    string
		revision  string
		pod       string
	}{
		{
			"Empty_Namespace_Value",
			"Expected namespace empty error",
			errors.New("namespace must not be empty"),
			"",
			config,
			revision,
			pod,
		},
		{
			"Empty_Config_Value",
			"Expected config empty error",
			errors.New("config must not be empty"),
			namespace,
			"",
			revision,
			pod,
		},
		{
			"Empty_Revision_Value",
			"Expected revision empty error",
			errors.New("revision must not be empty"),
			namespace,
			config,
			"",
			pod,
		},
		{
			"Empty_Pod_Value",
			"Expected pod empty error",
			errors.New("pod must not be empty"),
			namespace,
			config,
			revision,
			"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewPrometheusStatsReporter(test.namespace, test.config, test.revision, test.pod); err.Error() != test.result.Error() {
				t.Errorf("Got error msg from NewPrometheusStatsReporter(): '%+v', wanted '%+v'", err, test.errorMsg)
			}
		})
	}
}

func TestReporter_ReportNoProxied(t *testing.T) {
	testReportWithProxiedRequests(t, &autoscaler.Stat{RequestCount: 39, AverageConcurrentRequests: 3}, 39, 3, 0, 0)
}

func TestReporter_Report(t *testing.T) {
	testReportWithProxiedRequests(t, &autoscaler.Stat{RequestCount: 39, AverageConcurrentRequests: 3, ProxiedRequestCount: 15, AverageProxiedConcurrentRequests: 2}, 39, 3, 15, 2)
}

func testReportWithProxiedRequests(t *testing.T, stat *autoscaler.Stat, reqCount, concurrency, proxiedCount, proxiedConcurrency float64) {
	t.Helper()
	reporter, err := NewPrometheusStatsReporter(namespace, config, revision, pod)
	if err != nil {
		t.Errorf("Something went wrong with creating a reporter, '%v'.", err)
	}
	if !reporter.initialized {
		t.Error("Reporter should be initialized")
	}
	if err := reporter.Report(stat); err != nil {
		t.Error(err)
	}
	checkData(t, operationsPerSecondGV, reqCount)
	checkData(t, averageConcurrentRequestsGV, concurrency)
	checkData(t, proxiedOperationsPerSecondGV, proxiedCount)
	checkData(t, averageProxiedConcurrentRequestsGV, proxiedConcurrency)
}

func checkData(t *testing.T, gv *prometheus.GaugeVec, wanted float64) {
	t.Helper()
	g, err := gv.GetMetricWith(prometheus.Labels{
		destinationNsLabel:     namespace,
		destinationConfigLabel: config,
		destinationRevLabel:    revision,
		destinationPodLabel:    pod,
	})
	if err != nil {
		t.Fatalf("GaugeVec.GetMetricWith() error = %v", err)
	}

	m := dto.Metric{}
	if err := g.Write(&m); err != nil {
		t.Fatalf("Gauge.Write() error = %v", err)
	}
	if got := *m.Gauge.Value; wanted != got {
		t.Errorf("Got %v for Gauge value, wanted %v", got, wanted)
	}
}
