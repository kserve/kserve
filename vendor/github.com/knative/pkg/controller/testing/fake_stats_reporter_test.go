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

package testing

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/knative/pkg/controller"
)

var _ controller.StatsReporter = (*FakeStatsReporter)(nil)

func TestReportQueueDepth(t *testing.T) {
	r := &FakeStatsReporter{}
	r.ReportQueueDepth(10)
	if diff := cmp.Diff(r.GetQueueDepths(), []int64{10}); diff != "" {
		t.Errorf("queue depth len: %v", diff)
	}
}

func TestReportReconcile(t *testing.T) {
	r := &FakeStatsReporter{}
	r.ReportReconcile(time.Duration(123), "testkey", "False")
	if got, want := r.GetReconcileData(), []FakeReconcileStatData{{time.Duration(123), "testkey", "False"}}; !reflect.DeepEqual(want, got) {
		t.Errorf("reconcile data len: want: %v, got: %v", want, got)
	}
}
