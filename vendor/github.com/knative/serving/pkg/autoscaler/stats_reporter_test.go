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

package autoscaler

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/knative/pkg/metrics/metricskey"
	"go.opencensus.io/stats/view"
)

func TestNewStatsReporterErrors(t *testing.T) {
	// These are invalid as defined by the current OpenCensus library.
	invalidTagValues := []string{
		"naïve",                  // Includes non-ASCII character.
		strings.Repeat("a", 256), // Longer than 255 characters.
	}

	for _, v := range invalidTagValues {
		_, err := NewStatsReporter(v, v, v, v)
		if err == nil {
			t.Errorf("Expected err to not be nil for value %q, got nil", v)
		}

	}
}

func TestReporter_Report(t *testing.T) {
	r := &Reporter{}
	if err := r.ReportDesiredPodCount(10); err == nil {
		t.Error("Reporter.ReportDesiredPodCount() expected an error for Report call before init. Got success.")
	}

	r, _ = NewStatsReporter("testns", "testsvc", "testconfig", "testrev")
	wantTags := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       "testsvc",
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
	}

	// Send statistics only once and observe the results
	expectSuccess(t, "ReportDesiredPodCount", func() error { return r.ReportDesiredPodCount(10) })
	expectSuccess(t, "ReportRequestedPodCount", func() error { return r.ReportRequestedPodCount(7) })
	expectSuccess(t, "ReportActualPodCount", func() error { return r.ReportActualPodCount(5) })
	expectSuccess(t, "ReportPanic", func() error { return r.ReportPanic(0) })
	expectSuccess(t, "ReportObservedPodCount", func() error { return r.ReportObservedPodCount(1) })
	expectSuccess(t, "ReportStableRequestConcurrency", func() error { return r.ReportStableRequestConcurrency(2) })
	expectSuccess(t, "ReportPanicRequestConcurrency", func() error { return r.ReportPanicRequestConcurrency(3) })
	expectSuccess(t, "ReportTargetRequestConcurrency", func() error { return r.ReportTargetRequestConcurrency(0.9) })
	assertData(t, "desired_pods", wantTags, 10)
	assertData(t, "requested_pods", wantTags, 7)
	assertData(t, "actual_pods", wantTags, 5)
	assertData(t, "panic_mode", wantTags, 0)
	assertData(t, "observed_pods", wantTags, 1)
	assertData(t, "stable_request_concurrency", wantTags, 2)
	assertData(t, "panic_request_concurrency", wantTags, 3)
	assertData(t, "target_concurrency_per_pod", wantTags, 0.9)

	// All the stats are gauges - record multiple entries for one stat - last one should stick
	expectSuccess(t, "ReportDesiredPodCount", func() error { return r.ReportDesiredPodCount(1) })
	expectSuccess(t, "ReportDesiredPodCount", func() error { return r.ReportDesiredPodCount(2) })
	expectSuccess(t, "ReportDesiredPodCount", func() error { return r.ReportDesiredPodCount(3) })
	assertData(t, "desired_pods", wantTags, 3)

	expectSuccess(t, "ReportRequestedPodCount", func() error { return r.ReportRequestedPodCount(4) })
	expectSuccess(t, "ReportRequestedPodCount", func() error { return r.ReportRequestedPodCount(5) })
	expectSuccess(t, "ReportRequestedPodCount", func() error { return r.ReportRequestedPodCount(6) })
	assertData(t, "requested_pods", wantTags, 6)

	expectSuccess(t, "ReportActualPodCount", func() error { return r.ReportActualPodCount(7) })
	expectSuccess(t, "ReportActualPodCount", func() error { return r.ReportActualPodCount(8) })
	expectSuccess(t, "ReportActualPodCount", func() error { return r.ReportActualPodCount(9) })
	assertData(t, "actual_pods", wantTags, 9)

	expectSuccess(t, "ReportPanic", func() error { return r.ReportPanic(1) })
	expectSuccess(t, "ReportPanic", func() error { return r.ReportPanic(0) })
	expectSuccess(t, "ReportPanic", func() error { return r.ReportPanic(1) })
	assertData(t, "panic_mode", wantTags, 1)

	expectSuccess(t, "ReportPanic", func() error { return r.ReportPanic(0) })
	assertData(t, "panic_mode", wantTags, 0)
}

func TestReporter_EmptyServiceName(t *testing.T) {
	// Metrics reported to an empty service name will be recorded with service "unknown" (metricskey.ValueUnknown).
	r, _ := NewStatsReporter("testns", "" /*service=*/, "testconfig", "testrev")
	wantTags := map[string]string{
		metricskey.LabelNamespaceName:     "testns",
		metricskey.LabelServiceName:       metricskey.ValueUnknown,
		metricskey.LabelConfigurationName: "testconfig",
		metricskey.LabelRevisionName:      "testrev",
	}
	expectSuccess(t, "ReportDesiredPodCount", func() error { return r.ReportDesiredPodCount(10) })
	assertData(t, "desired_pods", wantTags, 10)
}

func expectSuccess(t *testing.T, funcName string, f func() error) {
	if err := f(); err != nil {
		t.Errorf("Reporter.%v() expected success but got error %v", funcName, err)
	}
}

func assertData(t *testing.T, name string, wantTags map[string]string, wantValue float64) {
	var err error
	wait.PollImmediate(1*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err = checkData(name, wantTags, wantValue); err != nil {
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		t.Error(err)
	}
}

func checkData(name string, wantTags map[string]string, wantValue float64) error {
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

	var value *view.LastValueData
	value, ok := last.Data.(*view.LastValueData)
	if !ok {
		return fmt.Errorf("last.Data.(Type) = %T, want: %T", last.Data, value)
	}

	if value.Value != wantValue {
		return fmt.Errorf("Value = %v, want: %v", value.Value, wantValue)
	}

	return nil
}
