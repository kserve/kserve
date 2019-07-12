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

package reconciler

import (
	"fmt"
	"testing"
	"time"

	"go.opencensus.io/tag"

	"go.opencensus.io/stats/view"
)

const (
	reconcilerMockName   = "mock_reconciler"
	testServiceNamespace = "test_namespace"
	testServiceName      = "test_service"
)

func TestNewStatsReporter(t *testing.T) {
	r, err := NewStatsReporter(reconcilerMockName)
	if err != nil {
		t.Errorf("Failed to create reporter: %v", err)
	}

	m := tag.FromContext(r.(*reporter).ctx)
	v, ok := m.Value(reconcilerTagKey)
	if !ok {
		t.Fatalf("Expected tag %q", reconcilerTagKey)
	}
	if v != reconcilerMockName {
		t.Fatalf("Expected %q for tag %q, got %q", reconcilerMockName, reconcilerTagKey, v)
	}
}

func TestReporter_ReportDuration(t *testing.T) {
	reporter, err := NewStatsReporter(reconcilerMockName)
	if err != nil {
		t.Errorf("Failed to create reporter: %v", err)
	}
	countWas := int64(0)
	if m := getMetric(t, ServiceReadyCountN); m != nil {
		countWas = m.Data.(*view.CountData).Value
	}

	if err = reporter.ReportServiceReady(testServiceNamespace, testServiceName, time.Second); err != nil {
		t.Error(err)
	}
	expectedTags := []tag.Tag{
		{Key: keyTagKey, Value: fmt.Sprintf("%s/%s", testServiceNamespace, testServiceName)},
		{Key: reconcilerTagKey, Value: reconcilerMockName},
	}

	latency := getMetric(t, ServiceReadyLatencyN)
	if v := latency.Data.(*view.LastValueData).Value; v != 1000 {
		t.Errorf("Expected latency %v, Got %v", 1000, v)
	}
	checkTags(t, expectedTags, latency.Tags)

	count := getMetric(t, ServiceReadyCountN)
	if got, want := count.Data.(*view.CountData).Value, countWas+1; got != want {
		t.Errorf("Latency report count = %d, want: %d", got, want)
	}
	checkTags(t, expectedTags, count.Tags)
}

func getMetric(t *testing.T, metric string) *view.Row {
	t.Helper()
	rows, err := view.RetrieveData(metric)
	if err != nil {
		t.Errorf("Failed retrieving data: %v", err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func checkTags(t *testing.T, expected, observed []tag.Tag) {
	t.Helper()
	if len(expected) != len(observed) {
		t.Errorf("Unexpected tags: desired %v observed %v", expected, observed)
	}
	for i := 0; i < len(expected); i++ {
		if expected[i] != observed[i] {
			t.Errorf("Unexpected tag at location %v: desired %v observed %v", i, expected[i], observed[i])
		}
	}
}
