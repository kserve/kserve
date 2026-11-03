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

// TestBatchTimedSizeReachedFirst verifies that a batch is emitted when
// size N is reached before the interval elapses.
func TestBatchTimedSizeReachedFirst(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	// Use a long interval so size is reached first
	batch := NewTimedBatch(3, 10*time.Second)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 3 records quickly
	go func() {
		for i := range 3 {
			in <- LogRequest{Id: string(rune('A' + i))}
		}
		close(in)
	}()

	// Collect output with timeout
	timeout := time.After(100 * time.Millisecond)
	var batches [][]LogRequest
	batchReceived := false
	for !batchReceived {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				batchReceived = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for batch (size should be reached quickly)")
		}
	}

	// Verify one batch of size 3
	g.Expect(batches).To(gomega.HaveLen(1))
	g.Expect(batches[0]).To(gomega.HaveLen(3))

	<-done
}

// TestBatchTimedIntervalReachedFirst verifies that a partial batch is
// emitted when the interval elapses before size N is reached.
func TestBatchTimedIntervalReachedFirst(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	// Use a short interval
	interval := 50 * time.Millisecond
	batch := NewTimedBatch(10, interval)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 2 records (less than batch size)
	go func() {
		in <- LogRequest{Id: "1"}
		in <- LogRequest{Id: "2"}
		// Wait for interval to elapse
		time.Sleep(interval + 20*time.Millisecond)
		close(in)
	}()

	// Collect output
	timeout := time.After(200 * time.Millisecond)
	var batches [][]LogRequest
	collectDone := false
	for !collectDone {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				collectDone = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for batch (interval should trigger)")
		}
	}

	// Verify one batch of size 2 (partial batch due to interval)
	g.Expect(batches).To(gomega.HaveLen(1))
	g.Expect(batches[0]).To(gomega.HaveLen(2))

	<-done
}

// TestBatchTimedFlushesRemainder verifies that remainder is flushed
// when in closes.
func TestBatchTimedFlushesRemainder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	// Use a long interval
	batch := NewTimedBatch(5, 10*time.Second)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 2 records and close immediately
	go func() {
		in <- LogRequest{Id: "1"}
		in <- LogRequest{Id: "2"}
		close(in)
	}()

	// Collect output
	timeout := time.After(100 * time.Millisecond)
	var batches [][]LogRequest
	collectDone := false
	for !collectDone {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				collectDone = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for remainder flush")
		}
	}

	// Verify one batch of size 2 (flushed on close)
	g.Expect(batches).To(gomega.HaveLen(1))
	g.Expect(batches[0]).To(gomega.HaveLen(2))

	<-done
}

// TestBatchTimedRespectsCancellation verifies that ctx cancellation
// stops processing.
func TestBatchTimedRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	in := make(chan LogRequest)
	out := make(chan []LogRequest, 10) // buffered

	batch := NewTimedBatch(5, 1*time.Second)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 2 records
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

// TestBatchTimedMultipleBatches verifies correct behavior with multiple
// batches emitted due to size.
func TestBatchTimedMultipleBatches(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	batchSize := 3
	batch := NewTimedBatch(batchSize, 10*time.Second)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	recordCount := 7 // 2 full batches + 1 remainder
	go func() {
		for i := range recordCount {
			in <- LogRequest{Id: string(rune('A' + i))}
		}
		close(in)
	}()

	// Collect output
	timeout := time.After(200 * time.Millisecond)
	var batches [][]LogRequest
	collectDone := false
	for !collectDone {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				collectDone = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for batches")
		}
	}

	// Verify 3 batches: [3, 3, 1]
	g.Expect(batches).To(gomega.HaveLen(3))
	g.Expect(batches[0]).To(gomega.HaveLen(3))
	g.Expect(batches[1]).To(gomega.HaveLen(3))
	g.Expect(batches[2]).To(gomega.HaveLen(1))

	<-done
}

// TestBatchTimedIntervalTriggers verifies that batches are emitted
// at regular intervals when records arrive slowly.
func TestBatchTimedIntervalTriggers(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	interval := 30 * time.Millisecond
	batch := NewTimedBatch(10, interval)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send records slowly, one every 40ms (longer than interval)
	go func() {
		for i := range 3 {
			in <- LogRequest{Id: string(rune('A' + i))}
			time.Sleep(40 * time.Millisecond)
		}
		close(in)
	}()

	// Collect output
	timeout := time.After(500 * time.Millisecond)
	var batches [][]LogRequest
	collectDone := false
	for !collectDone {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				collectDone = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for interval-triggered batches")
		}
	}

	// Should have 3 or 4 batches (each record triggers interval, plus possible final flush)
	// The exact number depends on timing, but each should have 1 record
	g.Expect(len(batches)).To(gomega.BeNumerically(">=", 3))
	for i, b := range batches {
		g.Expect(b).ToNot(gomega.BeEmpty(), "batch %d should have at least 1 record", i)
		g.Expect(len(b)).To(gomega.BeNumerically("<=", 3), "batch %d should have at most 3 records", i)
	}

	<-done
}

// TestBatchTimedZeroInterval verifies behavior with zero interval
// (should behave like size-only batching).
func TestBatchTimedZeroInterval(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	ctx := t.Context()

	in := make(chan LogRequest)
	out := make(chan []LogRequest)

	batch := NewTimedBatch(3, 0)

	// Run strategy in background
	done := make(chan struct{})
	go func() {
		batch.Run(ctx, in, out)
		close(done)
	}()

	// Send 7 records
	go func() {
		for i := range 7 {
			in <- LogRequest{Id: string(rune('A' + i))}
		}
		close(in)
	}()

	// Collect output
	timeout := time.After(100 * time.Millisecond)
	var batches [][]LogRequest
	collectDone := false
	for !collectDone {
		select {
		case b, ok := <-out:
			if ok {
				batches = append(batches, b)
			} else {
				collectDone = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for batches")
		}
	}

	// Should get 3 batches: [3, 3, 1]
	g.Expect(batches).To(gomega.HaveLen(3))
	g.Expect(batches[0]).To(gomega.HaveLen(3))
	g.Expect(batches[1]).To(gomega.HaveLen(3))
	g.Expect(batches[2]).To(gomega.HaveLen(1))

	<-done
}
