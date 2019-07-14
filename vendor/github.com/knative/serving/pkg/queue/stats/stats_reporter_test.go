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

package stats

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/knative/pkg/metrics/metricskey"
	"go.opencensus.io/stats/view"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	testNs   = "test-default"
	testSvc  = "helloworld-go-service"
	testConf = "helloworld-go"
	testRev  = "helloworld-go-00001"
)

func TestNewStatsReporter_negative(t *testing.T) {
	tests := []struct {
		name      string
		errorMsg  string
		result    error
		namespace string
		config    string
		revision  string
	}{{
		"Empty_Namespace_Value",
		"Expected namespace empty error",
		errors.New("namespace must not be empty"),
		"",
		testConf,
		testRev,
	}, {
		"Empty_Config_Value",
		"Expected config empty error",
		errors.New("config must not be empty"),
		testNs,
		"",
		testRev,
	}, {
		"Empty_Revision_Value",
		"Expected revision empty error",
		errors.New("revision must not be empty"),
		testRev,
		testConf,
		"",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewStatsReporter(test.namespace, testSvc, test.config, test.revision); err.Error() != test.result.Error() {
				t.Errorf("%+v, got: '%+v'", test.errorMsg, err)
			}
		})
	}
}

func TestReporter_Report(t *testing.T) {
	r := &Reporter{}
	if err := r.ReportRequestCount(200, 10); err == nil {
		t.Error("Reporter.ReportRequestCount() expected an error for Report call before init. Got success.")
	}

	r, err := NewStatsReporter(testNs, testSvc, testConf, testRev)
	if err != nil {
		t.Fatalf("Unexpected error from NewStatsReporter() = %v", err)
	}
	wantTags := map[string]string{
		metricskey.LabelNamespaceName:     testNs,
		metricskey.LabelServiceName:       testSvc,
		metricskey.LabelConfigurationName: testConf,
		metricskey.LabelRevisionName:      testRev,
		"response_code":                   "200",
		"response_code_class":             "2xx",
	}

	// Send statistics only once and observe the results
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportRequestCount(200, 1) })
	assertSumData(t, "request_count", wantTags, 1)

	// The stats are cumulative - record multiple entries, should get sum
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportRequestCount(200, 2) })
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportRequestCount(200, 3) })
	assertSumData(t, "request_count", wantTags, 6)

	// Send statistics only once and observe the results
	expectSuccess(t, "ReportResponseTime", func() error { return r.ReportResponseTime(200, 100*time.Millisecond) })
	assertDistributionData(t, "request_latencies", wantTags, 1, 100, 100)

	// The stats are cumulative - record multiple entries, should get count sum
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportResponseTime(200, 200*time.Millisecond) })
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportResponseTime(200, 300*time.Millisecond) })
	assertDistributionData(t, "request_latencies", wantTags, 3, 100, 300)

	unregisterViews(r)

	// Test reporter with empty service name
	r, err = NewStatsReporter(testNs, "" /*service name*/, testConf, testRev)
	if err != nil {
		t.Fatalf("Unexpected error from NewStatsReporter() = %v", err)
	}
	wantTags = map[string]string{
		metricskey.LabelNamespaceName:     testNs,
		metricskey.LabelServiceName:       "unknown",
		metricskey.LabelConfigurationName: testConf,
		metricskey.LabelRevisionName:      testRev,
		"response_code":                   "200",
		"response_code_class":             "2xx",
	}

	// Send statistics only once and observe the results
	expectSuccess(t, "ReportRequestCount", func() error { return r.ReportRequestCount(200, 1) })
	assertSumData(t, "request_count", wantTags, 1)

	unregisterViews(r)
}

func expectSuccess(t *testing.T, funcName string, f func() error) {
	if err := f(); err != nil {
		t.Errorf("Reporter.%v() expected success but got error %v", funcName, err)
	}
}

// TODO(yanweiguo): move these helper functions to a shared lib.
func assertSumData(t *testing.T, name string, wantTags map[string]string, wantValue float64) {
	var err error
	wait.PollImmediate(1*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err = checkSumData(name, wantTags, wantValue); err != nil {
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		t.Error(err)
	}
}

func checkSumData(name string, wantTags map[string]string, wantValue float64) error {
	d, err := view.RetrieveData(name)
	if err != nil {
		return err
	}

	if len(d) < 1 {
		return errors.New("len(d) = 0, want: >= 0")
	}
	last := d[len(d)-1]

	for _, got := range last.Tags {
		want, ok := wantTags[got.Key.Name()]
		if !ok {
			return fmt.Errorf("got an unexpected tag from view.RetrieveData: (%v, %v)", got.Key.Name(), got.Value)
		}
		if got.Value != want {
			return fmt.Errorf("Tags[%v] = %v, want: %v", got.Key.Name(), got.Value, want)
		}
	}

	var value *view.SumData
	value, ok := last.Data.(*view.SumData)
	if !ok {
		return fmt.Errorf("last.Data.(Type) = %T, want: %T", last.Data, value)
	}

	if value.Value != wantValue {
		return fmt.Errorf("value = %v, want: %v", value.Value, wantValue)
	}

	return nil
}

func assertDistributionData(t *testing.T, name string, wantTags map[string]string, wantCount int64, wantMin, wantMax float64) {
	var err error
	wait.PollImmediate(1*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err = checkDistributionData(name, wantTags, wantCount, wantMin, wantMax); err != nil {
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		t.Error(err)
	}
}

func checkDistributionData(name string, wantTags map[string]string, wantCount int64, wantMin, wantMax float64) error {
	d, err := view.RetrieveData(name)
	if err != nil {
		return err
	}

	if len(d) < 1 {
		return errors.New("len(d) = 0, want: >= 0")
	}
	last := d[len(d)-1]

	for _, got := range last.Tags {
		want, ok := wantTags[got.Key.Name()]
		if !ok {
			return fmt.Errorf("got an unexpected tag from view.RetrieveData: (%v, %v)", got.Key.Name(), got.Value)
		}
		if got.Value != want {
			return fmt.Errorf("Tags[%v] = %v, want: %v", got.Key.Name(), got.Value, want)
		}
	}

	var value *view.DistributionData
	value, ok := last.Data.(*view.DistributionData)
	if !ok {
		return fmt.Errorf("last.Data.(Type) = %T, want: %T", last.Data, value)
	}

	if value.Count != wantCount {
		return fmt.Errorf("value.Count = %v, want: %v", value.Count, wantCount)
	}

	if value.Min != wantMin {
		return fmt.Errorf("value.Min = %v, want: %v", value.Min, wantMin)
	}

	if value.Max != wantMax {
		return fmt.Errorf("value.Count = %v, want: %v", value.Max, wantMax)
	}

	return nil
}

// unregisterViews unregisters the views registerred in NewStatsReporter.
func unregisterViews(r *Reporter) error {
	if !r.initialized {
		return errors.New("reporter is not initialized")
	}
	var views []*view.View
	if v := view.Find(requestCountN); v != nil {
		views = append(views, v)
	}
	if v := view.Find(responseTimeInMsecN); v != nil {
		views = append(views, v)
	}
	view.Unregister(views...)
	r.initialized = false
	return nil
}
