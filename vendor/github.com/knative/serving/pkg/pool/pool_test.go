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

package pool

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelismNoErrors(t *testing.T) {
	tests := []struct {
		name string
		size int
		work int
	}{{
		name: "single threaded",
		size: 1,
		work: 3,
	}, {
		name: "three workers",
		size: 3,
		work: 10,
	}, {
		name: "ten workers",
		size: 10,
		work: 100,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				// m guards max.
				m      sync.Mutex
				max    int32
				active int32
			)

			// Use our own waitgroup to ensure that the work
			// can all complete before we block on the error
			// result.
			wg := &sync.WaitGroup{}

			worker := func() error {
				defer wg.Done()
				na := atomic.AddInt32(&active, 1)
				defer atomic.AddInt32(&active, -1)

				func() {
					m.Lock()
					defer m.Unlock()
					if max < na {
						max = na
					}
				}()

				// Sleep a small amount to simulate work.  This should be
				// sufficient to saturate the threadpool before the first
				// one wakes up.
				time.Sleep(10 * time.Millisecond)
				return nil
			}

			p := New(tc.size)
			for idx := 0; idx < tc.work; idx++ {
				wg.Add(1)
				p.Go(worker)
			}

			// First wait for the waitgroup to finish, so that
			// we are sure it isn't the Wait call that flushes
			// remaining work.
			wg.Wait()

			if err := p.Wait(); err != nil {
				t.Errorf("Wait() = %v", err)
			}

			if err := p.Wait(); err != nil {
				t.Errorf("Wait() = %v", err)
			}

			if got, want := max, int32(tc.size); got != want {
				t.Errorf("max active = %v, wanted %v", got, want)
			}
		})
	}
}

func TestParallelismWithErrors(t *testing.T) {
	tests := []struct {
		name string
		size int
		work int
	}{{
		name: "single threaded",
		size: 1,
		work: 3,
	}, {
		name: "three workers",
		size: 3,
		work: 10,
	}, {
		name: "ten workers",
		size: 10,
		// This is the number of errors that we can buffer before
		// the test kernel below will deadlock because we need the
		// pool's Wait call to drain the buffered errors before more
		// than this can be sent.
		work: DefaultCapacity + 10, /* size */
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				// m guards max.
				m      sync.Mutex
				max    int32
				active int32
			)

			// Use our own waitgroup to ensure that the work
			// can all complete before we block on the error
			// result.
			wg := &sync.WaitGroup{}

			errExpected := errors.New("this is what I expect")
			workerFactory := func(err error) func() error {
				return func() error {
					defer wg.Done()
					na := atomic.AddInt32(&active, 1)
					defer atomic.AddInt32(&active, -1)

					func() {
						m.Lock()
						defer m.Unlock()
						if max < na {
							max = na
						}
					}()

					// Make the first piece of work finish quickly.
					if err != errExpected {
						// Sleep a small amount to simulate work.  This should be
						// sufficient to saturate the threadpool before the first
						// one wakes up.
						time.Sleep(10 * time.Millisecond)
					}
					return err
				}
			}

			p := New(tc.size)

			// Let the work complete.
			wg.Add(1)
			p.Go(workerFactory(errExpected))
			time.Sleep(10 * time.Millisecond)

			// Change the error we return and queue the remaining work.
			for idx := 1; idx < tc.work; idx++ {
				wg.Add(1)
				p.Go(workerFactory(errors.New("this is not what I expect")))
			}

			// First wait for the waitgroup to finish, so that
			// we are sure it isn't the Wait call that flushes
			// remaining work.
			wg.Wait()

			if err := p.Wait(); err != errExpected {
				t.Errorf("Wait() = %v, wanted %v", err, errExpected)
			}

			if got, want := max, int32(tc.size); got != want {
				t.Errorf("max active = %v, wanted %v", got, want)
			}
		})
	}
}
