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

// scale_test.go brings up a number of services tracking the time to various waypoints.

package performance

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/knative/test-infra/shared/junit"
	"github.com/knative/test-infra/shared/testgrid"

	"github.com/knative/serving/test/e2e"
)

type metrics struct {
	min           time.Duration
	max           time.Duration
	totalDuration time.Duration
	num           int64
}

func (m *metrics) Add(start time.Time) {
	// Compute the duration since the provided start time.
	d := time.Since(start)

	if d < m.min || m.min == 0 {
		m.min = d
	}
	if d > m.max {
		m.max = d
	}
	m.totalDuration += d
	m.num++
}

func (m metrics) Min() float32 {
	return float32(m.min.Seconds())
}

func (m metrics) Max() float32 {
	return float32(m.max.Seconds())
}

func (m metrics) Avg() float32 {
	if m.num == 0 {
		return -1.0
	}
	return float32(m.totalDuration.Seconds()) / float32(m.num)
}

func (m metrics) Num() int64 {
	return m.num
}

type latencies struct {
	// Guards access to latencies.
	m       sync.RWMutex
	metrics map[string]metrics
}

var _ e2e.Latencies = (*latencies)(nil)

func (l *latencies) Add(name string, start time.Time) {
	l.m.Lock()
	defer l.m.Unlock()

	m := l.metrics[name]
	m.Add(start)
	l.metrics[name] = m
}

func (l *latencies) Min(name string) float32 {
	l.m.RLock()
	defer l.m.RUnlock()
	return l.metrics[name].Min()
}

func (l *latencies) Max(name string) float32 {
	l.m.RLock()
	defer l.m.RUnlock()
	return l.metrics[name].Max()
}

func (l *latencies) Avg(name string) float32 {
	l.m.RLock()
	defer l.m.RUnlock()
	return l.metrics[name].Avg()
}

func (l *latencies) Num(name string) int64 {
	l.m.RLock()
	defer l.m.RUnlock()
	return l.metrics[name].Num()
}

func (l *latencies) Results(t *testing.T) []junit.TestCase {
	l.m.RLock()
	defer l.m.RUnlock()

	order := make([]string, 0, len(l.metrics))
	for k := range l.metrics {
		order = append(order, k)
	}
	sort.Strings(order)

	// Add latency metrics
	tc := make([]junit.TestCase, 0, 3*len(order))
	for _, key := range order {
		tc = append(tc,
			CreatePerfTestCase(l.Min(key), fmt.Sprintf("%s.min", key), t.Name()),
			CreatePerfTestCase(l.Max(key), fmt.Sprintf("%s.max", key), t.Name()),
			CreatePerfTestCase(l.Avg(key), fmt.Sprintf("%s.avg", key), t.Name()),
			CreatePerfTestCase(float32(l.Num(key)), fmt.Sprintf("%s.num", key), t.Name()))
	}
	return tc
}

func TestScaleToN(t *testing.T) {
	// Run each of these variations.
	// >50 testsize will fail the expected slo's based on the current istio config.
	tests := []int{10, 50}

	// Accumulate the results from each row in our table (recorded below).
	var results []junit.TestCase

	for _, size := range tests {
		t.Run(fmt.Sprintf("scale-%02d", size), func(t *testing.T) {
			// Record the observed latencies.
			l := &latencies{
				metrics: make(map[string]metrics),
			}
			defer func() {
				results = append(results, l.Results(t)...)
			}()

			e2e.ScaleToWithin(t, size, 30*time.Minute, l)
		})
	}

	if err := testgrid.CreateXMLOutput(results, t.Name()); err != nil {
		t.Errorf("Cannot create output xml: %v", err)
	}
}
