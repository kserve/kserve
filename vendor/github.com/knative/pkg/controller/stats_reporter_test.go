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

package controller

import (
	"strings"
	"testing"
	"time"

	"go.opencensus.io/stats/view"
)

func TestNewStatsReporterErrors(t *testing.T) {
	// These are invalid as defined by the current OpenCensus library.
	invalidTagValues := []string{
		"naïve",                  // Includes non-ASCII character.
		strings.Repeat("a", 256), // Longer than 255 characters.
	}

	for _, v := range invalidTagValues {
		_, err := NewStatsReporter(v)
		if err == nil {
			t.Errorf("Expected err to not be nil for value %q, got nil", v)
		}

	}
}

func TestReportQueueDepth(t *testing.T) {
	r1 := &reporter{}
	if err := r1.ReportQueueDepth(10); err == nil {
		t.Error("Reporter.Report() expected an error for Report call before init. Got success.")
	}

	r, _ := NewStatsReporter("testreconciler")
	wantTags := map[string]string{
		"reconciler": "testreconciler",
	}

	// Send statistics only once and observe the results
	expectSuccess(t, func() error { return r.ReportQueueDepth(10) })
	checkLastValueData(t, "work_queue_depth", wantTags, 10)

	// Queue depth stats is a gauge - record multiple entries - last one should stick
	expectSuccess(t, func() error { return r.ReportQueueDepth(1) })
	expectSuccess(t, func() error { return r.ReportQueueDepth(2) })
	expectSuccess(t, func() error { return r.ReportQueueDepth(3) })
	checkLastValueData(t, "work_queue_depth", wantTags, 3)
}

func TestReportReconcile(t *testing.T) {
	r, _ := NewStatsReporter("testreconciler")
	wantTags := map[string]string{
		"reconciler": "testreconciler",
		"key":        "test/key",
		"success":    "true",
	}

	expectSuccess(t, func() error { return r.ReportReconcile(time.Duration(10*time.Millisecond), "test/key", "true") })
	checkCountData(t, "reconcile_count", wantTags, 1)
	checkDistributionData(t, "reconcile_latency", wantTags, 10)

	expectSuccess(t, func() error { return r.ReportReconcile(time.Duration(15*time.Millisecond), "test/key", "true") })
	checkCountData(t, "reconcile_count", wantTags, 2)
	checkDistributionData(t, "reconcile_latency", wantTags, 25)
}

func expectSuccess(t *testing.T, f func() error) {
	t.Helper()
	if err := f(); err != nil {
		t.Errorf("Reporter.Report() expected success but got error %v", err)
	}
}

func checkLastValueData(t *testing.T, name string, wantTags map[string]string, wantValue float64) {
	t.Helper()
	if row := checkRow(t, name); row != nil {
		checkTags(t, wantTags, row)
		if s, ok := row.Data.(*view.LastValueData); !ok {
			t.Error("Reporter.Report() expected a LastValueData type")
		} else if s.Value != wantValue {
			t.Errorf("Reporter.Report() expected %v got %v. metric: %v", s.Value, wantValue, name)
		}
	}
}

func checkCountData(t *testing.T, name string, wantTags map[string]string, wantValue int64) {
	t.Helper()
	row := checkRow(t, name)
	if row == nil {
		return
	}

	checkTags(t, wantTags, row)
	if s, ok := row.Data.(*view.CountData); !ok {
		t.Error("Reporter.Report() expected a LastValueData type")
	} else if s.Value != wantValue {
		t.Errorf("Reporter.Report() expected %v got %v. metric: %v", s.Value, (float64)(wantValue), name)
	}
}

func checkDistributionData(t *testing.T, name string, wantTags map[string]string, wantValue float64) {
	t.Helper()
	row := checkRow(t, name)
	if row == nil {
		return
	}

	checkTags(t, wantTags, row)
	if s, ok := row.Data.(*view.DistributionData); !ok {
		t.Error("Reporter.Report() expected a LastValueData type")
	} else if s.Sum() != wantValue {
		t.Errorf("Reporter.Report() expected %v got %v. metric: %v", s.Sum(), wantValue, name)
	}
}

func checkRow(t *testing.T, name string) *view.Row {
	t.Helper()
	d, err := view.RetrieveData(name)
	if err != nil {
		t.Fatalf("Reporter.Report() error = %v, wantErr %v", err, false)
		return nil
	}
	if len(d) != 1 {
		t.Errorf("Reporter.Report() len(d)=%v, want 1", len(d))
	}
	return d[0]
}

func checkTags(t *testing.T, wantTags map[string]string, row *view.Row) {
	t.Helper()
	for _, got := range row.Tags {
		if want, ok := wantTags[got.Key.Name()]; !ok {
			t.Errorf("Reporter.Report() got an extra tag %v: %v", got.Key.Name(), got.Value)
		} else if got.Value != want {
			t.Errorf("Reporter.Report() expected a different tag value. key:%v, got: %v, want: %v", got.Key.Name(), got.Value, want)
		}
	}
}
