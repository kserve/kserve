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

package activator

import (
	"testing"
	"time"

	"github.com/knative/pkg/metrics/metricskey"

	"go.opencensus.io/stats/view"
)

// unregister, ehm, unregisters the metrics that were registered, by
// virtue of StatsReporter creation.
// Since golang executes test iterations within the same process, the stats reporter
// returns an error if the metric is already registered and the test panics.
func unregister() {
	for _, s := range []string{
		"request_count",
		"request_latencies",
	} {
		if v := view.Find(s); v != nil {
			view.Unregister(v)
		}
	}
}

func TestActivatorReporter(t *testing.T) {
	r := &Reporter{}

	if err := r.ReportRequestCount("testns", "testsvc", "testconfig", "testrev", 200, 1, 1); err == nil {
		t.Error("Reporter expected an error for Report call before init. Got success.")
	}

	var err error
	if r, err = NewStatsReporter(); err != nil {
		t.Errorf("Failed to create a new reporter: %v", err)
	}
	// Without this `go test ... -count=X`, where X > 1, fails, since
	// we get an error about view already being registered.
	defer unregister()

	// test ReportRequestCount
	wantTags2 := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       "testsvc",
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
		"response_code":                   "200",
		"response_code_class":             "2xx",
		"num_tries":                       "6",
	}
	expectSuccess(t, func() error { return r.ReportRequestCount("testns", "testsvc", "testconfig", "testrev", 200, 6, 1) })
	expectSuccess(t, func() error { return r.ReportRequestCount("testns", "testsvc", "testconfig", "testrev", 200, 6, 3) })
	checkSumData(t, "request_count", wantTags2, 4)

	// test ReportResponseTime
	wantTags3 := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       "testsvc",
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
		"response_code":                   "200",
		"response_code_class":             "2xx",
	}
	expectSuccess(t, func() error {
		return r.ReportResponseTime("testns", "testsvc", "testconfig", "testrev", 200, 1100*time.Millisecond)
	})
	expectSuccess(t, func() error {
		return r.ReportResponseTime("testns", "testsvc", "testconfig", "testrev", 200, 9100*time.Millisecond)
	})
	checkDistributionData(t, "request_latencies", wantTags3, 2, 1100.0, 9100.0)
}

func TestReportRequestCount_EmptyServiceName(t *testing.T) {
	r, _ := NewStatsReporter()
	defer unregister()

	wantTags := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       metricskey.ValueUnknown,
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
		"response_code":                   "200",
		"response_code_class":             "2xx",
		"num_tries":                       "6",
	}
	expectSuccess(t, func() error {
		return r.ReportRequestCount("testns" /*service=*/, "", "testconfig", "testrev", 200, 6, 10)
	})
	checkSumData(t, "request_count", wantTags, 10)
}

func TestReportResponseTime_EmptyServiceName(t *testing.T) {
	r, _ := NewStatsReporter()
	defer unregister()

	wantTags := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       metricskey.ValueUnknown,
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
		"response_code":                   "200",
		"response_code_class":             "2xx",
	}
	expectSuccess(t, func() error {
		return r.ReportResponseTime("testns" /*service=*/, "", "testconfig", "testrev", 200, 7100*time.Millisecond)
	})
	expectSuccess(t, func() error {
		return r.ReportResponseTime("testns" /*service=*/, "", "testconfig", "testrev", 200, 5100*time.Millisecond)
	})
	checkDistributionData(t, "request_latencies", wantTags, 2, 5100.0, 7100.0)
}

func expectSuccess(t *testing.T, f func() error) {
	t.Helper()
	if err := f(); err != nil {
		t.Errorf("Reporter expected success but got error: %v", err)
	}
}

func checkSumData(t *testing.T, name string, wantTags map[string]string, wantValue int) {
	t.Helper()
	if d, err := view.RetrieveData(name); err != nil {
		t.Errorf("Unexpected reporter error: %v", err)
	} else {
		if len(d) != 1 {
			t.Errorf("Reporter len(d) = %d, want: 1", len(d))
		}
		for _, got := range d[0].Tags {
			n := got.Key.Name()
			if want, ok := wantTags[n]; !ok {
				t.Errorf("Reporter got an extra tag %v: %v", n, got.Value)
			} else if got.Value != want {
				t.Errorf("Reporter expected a different tag value for key: %s, got: %s, want: %s", n, got.Value, want)
			}
		}

		if s, ok := d[0].Data.(*view.SumData); !ok {
			t.Error("Reporter expected a SumData type")
		} else if s.Value != float64(wantValue) {
			t.Errorf("For %s value = %v, want: %d", name, s.Value, wantValue)
		}
	}
}

func checkDistributionData(t *testing.T, name string, wantTags map[string]string, expectedCount int, expectedMin float64, expectedMax float64) {
	t.Helper()
	if d, err := view.RetrieveData(name); err != nil {
		t.Errorf("Unexpected reporter error: %v", err)
	} else {
		if len(d) != 1 {
			t.Errorf("Reporter len(d) = %d, want: 1", len(d))
		}
		for _, got := range d[0].Tags {
			n := got.Key.Name()
			if want, ok := wantTags[n]; !ok {
				t.Errorf("Reporter got an extra tag %v: %v", n, got.Value)
			} else if got.Value != want {
				t.Errorf("Reporter expected a different tag value for key: %s, got: %s, want: %s", n, got.Value, want)
			}
		}

		if s, ok := d[0].Data.(*view.DistributionData); !ok {
			t.Error("Reporter expected a DistributionData type")
		} else {
			if s.Count != int64(expectedCount) {
				t.Errorf("For metric %s: reporter count = %d, want = %d", name, s.Count, expectedCount)
			}
			if s.Min != expectedMin {
				t.Errorf("For metric %s: reporter count = %f, want = %f", name, s.Min, expectedMin)
			}
			if s.Max != expectedMax {
				t.Errorf("For metric %s: reporter count = %f, want = %f", name, s.Max, expectedMax)
			}
		}
	}
}
