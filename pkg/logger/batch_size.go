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

import "context"

// SizeBatch accumulates records and emits when N records are buffered.
type SizeBatch struct {
	size int
}

var _ BatchStrategy = &SizeBatch{}

// NewSizeBatch creates a new SizeBatch with the specified batch size.
func NewSizeBatch(n int) *SizeBatch {
	return &SizeBatch{size: n}
}

// Run reads from in, accumulates records, emits when size N is reached,
// flushes remaining records when in is closed, closes out when done,
// and respects ctx cancellation.
func (b *SizeBatch) Run(ctx context.Context, in <-chan LogRequest, out chan<- []LogRequest) {
	defer close(out)

	// Handle invalid sizes
	if b.size <= 0 {
		return
	}

	buffer := make([]LogRequest, 0, b.size)

	flush := func() {
		if len(buffer) > 0 {
			select {
			case <-ctx.Done():
				return
			case out <- buffer:
				buffer = make([]LogRequest, 0, b.size)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
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
