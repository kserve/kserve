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

package handler

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/system"
	"github.com/knative/serving/pkg/autoscaler"
)

const (
	requestOpTick  = "RequestOpTick"
	requestOpStart = "RequestOpStart"
	requestOpEnd   = "RequestOpEnd"
)

type fakeClock struct {
	Time time.Time
}

func (c fakeClock) Now() time.Time {
	return c.Time
}

type reqOp struct {
	op   string
	key  string
	time time.Time
}

func TestStats(t *testing.T) {
	tt := []struct {
		name          string
		ops           []reqOp
		expectedStats []*autoscaler.StatMessage
	}{{
		name: "Scale-from-zero sends stat",
		ops: []reqOp{{
			op:  requestOpStart,
			key: "pod1",
		}, {
			op:  requestOpStart,
			key: "pod2",
		}},
		expectedStats: []*autoscaler.StatMessage{{
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod2",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}},
		}}, {
		name: "Scale to two",
		ops: []reqOp{{
			op:  requestOpStart,
			key: "pod1",
		}, {
			op:  requestOpStart,
			key: "pod1",
		}, {
			op: requestOpTick,
		}},
		expectedStats: []*autoscaler.StatMessage{{
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 2,
				RequestCount:              2,
				PodName:                   "activator",
			}},
		}}, {
		name: "Scale-from-zero after tick sends stat",
		ops: []reqOp{{
			op:  requestOpStart,
			key: "pod1",
		}, {
			op:  requestOpEnd,
			key: "pod1",
		}, {
			op: requestOpTick,
		}, {
			op:  requestOpStart,
			key: "pod1",
		}},
		expectedStats: []*autoscaler.StatMessage{{
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}},
		}}, {
		name: "Multiple pods tick",
		ops: []reqOp{{
			op:  requestOpStart,
			key: "pod1",
		}, {
			op:  requestOpStart,
			key: "pod2",
		}, {
			op: requestOpTick,
		}, {
			op:  requestOpEnd,
			key: "pod1",
		}, {
			op:  requestOpStart,
			key: "pod3",
		}, {
			op: requestOpTick,
		}},
		expectedStats: []*autoscaler.StatMessage{{
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod2",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod1",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod2",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod3",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}}, {
			Key: "pod2",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              0,
				PodName:                   "activator",
			}}, {
			Key: "pod3",
			Stat: autoscaler.Stat{
				AverageConcurrentRequests: 1,
				RequestCount:              1,
				PodName:                   "activator",
			}},
		}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			closeCh := make(chan struct{})
			s, cr := newTestStats(fakeClock{})
			go func() {
				cr.Run(closeCh)
			}()

			// Apply request operations
			for _, op := range tc.ops {
				switch op.op {
				case requestOpStart:
					s.reqChan <- ReqEvent{Key: op.key, EventType: ReqIn}
				case requestOpEnd:
					s.reqChan <- ReqEvent{Key: op.key, EventType: ReqOut}
				case requestOpTick:
					s.reportBiChan <- op.time
				}
			}

			// Gather reported stats
			stats := make([]*autoscaler.StatMessage, 0)
			for i := 0; i < len(tc.expectedStats); i++ {
				sm := <-s.statChan
				stats = append(stats, sm)
			}

			// Check the stats we got match what we wanted
			sorter := cmpopts.SortSlices(func(a, b *autoscaler.StatMessage) bool {
				return a.Key < b.Key
			})
			if diff := cmp.Diff(tc.expectedStats, stats, sorter); diff != "" {
				t.Errorf("Unexpected stats (-want +got): %v", diff)
			}
		})
	}
}

// Test type to hold the bi-directional time channels
type testStats struct {
	reqChan      chan ReqEvent
	reportChan   <-chan time.Time
	statChan     chan *autoscaler.StatMessage
	reportBiChan chan time.Time
}

func newTestStats(clock system.Clock) (*testStats, *ConcurrencyReporter) {
	reportBiChan := make(chan time.Time)
	t := &testStats{
		reqChan:      make(chan ReqEvent),
		reportChan:   (<-chan time.Time)(reportBiChan),
		statChan:     make(chan *autoscaler.StatMessage, 20),
		reportBiChan: reportBiChan,
	}
	cr := NewConcurrencyReporterWithClock("activator", t.reqChan, t.reportChan, t.statChan, clock)
	return t, cr
}
