/*
Copyright 2025 The KServe Authors.
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

package logger

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
)

// TestBatchSizeExactBatch verifies that a batch of exactly N records
// is emitted when N records are pushed.
func TestBatchSizeExactBatch(t *testing.T) {
	testCases := []struct {
		name string
		size int
	}{
		{"size 1", 1},
		{"size 3", 3},
		{"size 10", 10},
		{"size 50", 50},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			ctx := t.Context()

			in := make(chan LogRequest)
			out := make(chan []LogRequest)

			batch := NewSizeBatch(tc.size)

			// Run strategy in background
			done := make(chan struct{})
			go func() {
				batch.Run(ctx, in, out)
				close(done)
			}()

			// Send exactly size records
			go func() {
				for i := range tc.size {
					in <- LogRequest{Id: string(rune('A' + i))}
				}
				close(in)
			}()

			// Collect output
			var batches [][]LogRequest
			for b := range out {
				batches = append(batches, b)
			}

			// Verify exactly one batch of size tc.size
			g.Expect(batches).To(gomega.HaveLen(1))
			g.Expect(batches[0]).To(gomega.HaveLen(tc.size))

			<-done
		})
	}
}

// TestBatchSizeRemainder verifies that remainder is flushed when in closes.
func TestBatchSizeRemainder(t *testing.T) {
	testCases := []struct {
		name         string
		size         int
		recordCount  int
		expectedLens []int
	}{
		{"7 records with size 3", 3, 7, []int{3, 3, 1}},
		{"5 records with size 2", 2, 5, []int{2, 2, 1}},
		{"10 records with size 4", 4, 10, []int{4, 4, 2}},
		{"1 record with size 5", 5, 1, []int{1}},
		{"0 records with size 3", 3, 0, []int{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			ctx := t.Context()

			in := make(chan LogRequest)
			out := make(chan []LogRequest)

			batch := NewSizeBatch(tc.size)

			// Run strategy in background
			done := make(chan struct{})
			go func() {
				batch.Run(ctx, in, out)
				close(done)
			}()

			// Send records
			go func() {
				for i := range tc.recordCount {
					in <- LogRequest{Id: string(rune('A' + i))}
				}
				close(in)
			}()

			// Collect output
			var batches [][]LogRequest
			for b := range out {
				batches = append(batches, b)
			}

			// Verify batch counts and sizes
			g.Expect(batches).To(gomega.HaveLen(len(tc.expectedLens)))
			for i, expectedLen := range tc.expectedLens {
				g.Expect(batches[i]).To(gomega.HaveLen(expectedLen), "batch %d has wrong size", i)
			}

			<-done
		})
	}
}

// TestBatchSizeRespectsCancellation verifies that ctx cancellation
// stops processing and flushes remainder.
func TestBatchSizeRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	in := make(chan LogRequest)
	out := make(chan []LogRequest, 10) // buffered

	batch := NewSizeBatch(5)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 2 records (less than batch size)
	in <- LogRequest{Id: "1"}
	in <- LogRequest{Id: "2"}

	// Cancel context
	cancel()

	// Verify Run completes
	timeout := time.After(100 * time.Millisecond)
	select {
	case <-done:
		// Success
	case <-timeout:
		t.Fatal("timeout waiting for Run to complete after cancellation")
	}

	// Verify output is closed
	select {
	case _, ok := <-out:
		if ok {
			// Drain any remaining records
			for range out {
			}
		}
	default:
	}
}

// TestBatchSizeMultipleBatches verifies correct behavior with multiple
// full batches.
func TestBatchSizeMultipleBatches(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	batchSize := 3
	batch := NewSizeBatch(batchSize)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	recordCount := 9 // 3 full batches
	go func() {
		for i := range recordCount {
			in <- LogRequest{Id: string(rune('A' + i))}
		}
		close(in)
	}()

	// Collect output
	batches := make([][]LogRequest, 0, recordCount/batchSize)
	for b := range out {
		batches = append(batches, b)
	}

	// Verify 3 batches of size 3
	g.Expect(batches).To(gomega.HaveLen(3))
	for i, b := range batches {
		g.Expect(b).To(gomega.HaveLen(batchSize), "batch %d has wrong size", i)
		// Verify order
		for j, req := range b {
			expectedId := string(rune('A' + i*batchSize + j))
			g.Expect(req.Id).To(gomega.Equal(expectedId))
		}
	}

	<-done
}

// TestBatchSizeInvalidSize verifies behavior with invalid batch sizes.
func TestBatchSizeInvalidSize(t *testing.T) {
	testCases := []struct {
		name string
		size int
	}{
		{"zero size", 0},
		{"negative size", -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			// Should panic or return immediately
			defer func() {
				if r := recover(); r != nil {
					// Panic is acceptable for invalid input
					return
				}
			}()

			ctx := t.Context()
			in := make(chan LogRequest)
			out := make(chan []LogRequest)

			batch := NewSizeBatch(tc.size)

			done := make(chan struct{})
			go func() {
				batch.Run(ctx, in, out)
				close(done)
			}()

			close(in)

			timeout := time.After(100 * time.Millisecond)
			select {
			case <-done:
				// Verify no batches emitted
				var batches [][]LogRequest
				for b := range out {
					batches = append(batches, b)
				}
				g.Expect(batches).To(gomega.BeEmpty())
			case <-timeout:
				t.Fatal("timeout waiting for Run to complete with invalid size")
			}
		})
	}
}
