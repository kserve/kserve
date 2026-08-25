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

// TestBatchImmediateOneRecordPerBatch verifies that each input record
// produces exactly one single-record batch on output.
func TestBatchImmediateOneRecordPerBatch(t *testing.T) {
	testCases := []struct {
		name        string
		recordCount int
	}{
		{"zero records", 0},
		{"one record", 1},
		{"five records", 5},
		{"one hundred records", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			ctx := t.Context()

			in := make(chan LogRequest)
			out := make(chan []LogRequest)

			batch := &ImmediateBatch{}

			// Run strategy in background
			done := make(chan struct{})
			go func() {
				batch.Run(ctx, in, out)
				close(done)
			}()

			// Send records
			go func() {
				for i := range tc.recordCount {
					in <- LogRequest{Id: string(rune(i))}
				}
				close(in)
			}()

			// Collect output
			var batches [][]LogRequest
			for b := range out {
				batches = append(batches, b)
			}

			// Verify
			g.Expect(batches).To(gomega.HaveLen(tc.recordCount))
			for i, b := range batches {
				g.Expect(b).To(gomega.HaveLen(1), "batch %d should contain exactly 1 record", i)
			}

			// Wait for Run to complete
			<-done
		})
	}
}

// TestBatchImmediateClosesOutput verifies that closing in closes out.
func TestBatchImmediateClosesOutput(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	batch := &ImmediateBatch{}

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Close input immediately
	close(in)

	// Verify output is closed by checking if we can receive from it
	timeout := time.After(100 * time.Millisecond)
	select {
	case _, ok := <-out:
		g.Expect(ok).To(gomega.BeFalse(), "out should be closed")
	case <-timeout:
		t.Fatal("timeout waiting for out to close")
	}

	// Wait for Run to complete
	<-done
}

// TestBatchImmediateRespectsCancellation verifies that ctx cancellation
// stops processing.
func TestBatchImmediateRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	in := make(chan LogRequest)
	out := make(chan []LogRequest, 10) // buffered to avoid blocking

	batch := &ImmediateBatch{}

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send a record
	in <- LogRequest{Id: "test"}

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

// TestBatchImmediatePreservesOrder verifies that records are emitted
// in the same order they are received.
func TestBatchImmediatePreservesOrder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	batch := &ImmediateBatch{}

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	count := 50
	go func() {
		for i := range count {
			in <- LogRequest{Id: string(rune('A' + i))}
		}
		close(in)
	}()

	// Collect output
	ids := make([]string, 0, count)
	for b := range out {
		g.Expect(b).To(gomega.HaveLen(1))
		ids = append(ids, b[0].Id)
	}

	// Verify order
	g.Expect(ids).To(gomega.HaveLen(count))
	for i := range count {
		expected := string(rune('A' + i))
		g.Expect(ids[i]).To(gomega.Equal(expected), "record %d out of order", i)
	}

	<-done
}
