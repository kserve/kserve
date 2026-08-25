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
	"time"
)

// TimedBatch accumulates records and emits when N records are buffered
// or when duration d elapses, whichever comes first.
type TimedBatch struct {
	size     int
	interval time.Duration
}

var _ BatchStrategy = &TimedBatch{}

// NewTimedBatch creates a new TimedBatch with the specified batch size
// and time interval.
func NewTimedBatch(n int, d time.Duration) *TimedBatch {
	return &TimedBatch{size: n, interval: d}
}

// Run reads from in, accumulates records, emits when size N is reached
// or interval d elapses (whichever comes first), flushes remaining records
// when in is closed, closes out when done, and respects ctx cancellation.
func (b *TimedBatch) Run(ctx context.Context, in <-chan LogRequest, out chan<- []LogRequest) {
	defer close(out)

	// Handle invalid sizes
	if b.size <= 0 {
		return
	}

	buffer := make([]LogRequest, 0, b.size)
	var timer *time.Timer
	var timerCh <-chan time.Time

	// Initialize timer if interval is positive
	if b.interval > 0 {
		timer = time.NewTimer(b.interval)
		timerCh = timer.C
		defer timer.Stop()
	}

	flush := func() {
		if len(buffer) > 0 {
			select {
			case <-ctx.Done():
				return
			case out <- buffer:
				buffer = make([]LogRequest, 0, b.size)
			}
		}
		// Reset timer after flush if interval is set
		if timer != nil {
			if !timer.Stop() {
				// Drain timer channel if it fired
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(b.interval)
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-timerCh:
			// Timer fired, flush partial batch
			flush()
		case req, ok := <-in:
			if !ok {
				// Channel closed, flush remaining
				flush()
				return
			}
			buffer = append(buffer, req)
			if len(buffer) == b.size {
				flush()
			}
		}
	}
}
