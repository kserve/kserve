// +build e2e

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

package e2e

import (
	"fmt"
	"testing"
	"time"

	. "github.com/knative/serving/test/e2e"
)

type nopLatencies struct {
	t *testing.T
}

var _ Latencies = (*nopLatencies)(nil)

func (nl *nopLatencies) Add(metric string, start time.Time) {
	duration := time.Since(start)

	nl.t.Logf("%q took %v", metric, duration)
}

// Limit for scale in -short mode
const shortModeMaxScale = 10

// While redundant, we run two versions of this by default:
// 1. TestScaleToN/size-10: a developer smoke test that's useful when changing this to assess whether
//   things have gone horribly wrong.  This should take about 12-20 seconds total.
// 2. TestScaleToN/scale-50: a more proper execution of the test, which verifies a slightly more
//   interesting burst of deployments, but low enough to complete in a reasonable window.
func TestScaleToN(t *testing.T) {
	// Run each of these variations.
	tests := []struct {
		size    int
		timeout time.Duration
	}{{
		size:    10,
		timeout: 60 * time.Second,
	}, {
		size:    50,
		timeout: 5 * time.Minute,
	}}

	for _, test := range tests {
		t.Run(fmt.Sprintf("scale-%d", test.size), func(t *testing.T) {
			if testing.Short() && test.size > shortModeMaxScale {
				t.Skip("Skipping test in short mode")
			}
			ScaleToWithin(t, test.size, test.timeout, &nopLatencies{t})
		})
	}
}
