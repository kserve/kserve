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

package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// semAcquireTimeout is a timeout for tests that try to acquire
	// a token of a semaphore.
	semAcquireTimeout = 10 * time.Second

	// semNoChangeTimeout is some additional wait time after a number
	// of acquires is reached to assert that no more acquires get through.
	semNoChangeTimeout = 50 * time.Millisecond
)

type request struct {
	barrier  chan struct{}
	accepted chan bool
}

func (r *request) wait() {
	r.accepted <- <-r.accepted
}

func TestBreakerInvalidConstructor(t *testing.T) {
	tests := []struct {
		name    string
		options BreakerParams
	}{{
		"QueueDepth = 0",
		BreakerParams{QueueDepth: 0, MaxConcurrency: 1, InitialCapacity: 1},
	}, {
		"MaxConcurrency negative",
		BreakerParams{QueueDepth: 1, MaxConcurrency: -1, InitialCapacity: 1},
	}, {
		"InitialCapacity negative",
		BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: -1},
	}, {
		"InitialCapacity out-of-bounds",
		BreakerParams{QueueDepth: 1, MaxConcurrency: 5, InitialCapacity: 6},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected a panic but the code didn't panic.")
				}
			}()

			NewBreaker(test.options)
		})
	}
}

func TestBreakerOverload(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 1}
	b := NewBreaker(params)           // Breaker capacity = 2
	want := []bool{true, true, false} // Only first two requests will be processed

	locks := b.concurrentRequests(3)

	unlockAll(locks)

	if diff := cmp.Diff(accepted(locks), want); diff != "" {
		t.Errorf("Unexpected accepted requests (-want +got): %s", diff)
	}
}

func TestBreakerOverloadWithEmptySemaphore(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 0}
	b := NewBreaker(params)           // Breaker capacity = 2
	want := []bool{true, true, false} // Only first two requests are processed

	b.sem.release()
	locks := b.concurrentRequests(3)

	unlockAll(locks)

	if diff := cmp.Diff(accepted(locks), want); diff != "" {
		t.Errorf("Unexpected accepted requests (-want +got): %s", diff)
	}
}

func TestBreakerNoOverload(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 1}
	b := NewBreaker(params)                // Breaker capacity = 2
	want := []bool{true, true, true, true} // Only two requests will be in flight at a time
	locks := make([]request, 4)
	locks[0] = b.concurrentRequest()
	locks[1] = b.concurrentRequest()
	unlock(locks[0])
	locks[2] = b.concurrentRequest()
	unlock(locks[1])
	locks[3] = b.concurrentRequest()
	unlockAll(locks[2:])

	if diff := cmp.Diff(accepted(locks), want); diff != "" {
		t.Errorf("Unexpected accepted requests (-want +got): %s", diff)
	}
}

func TestBreakerRecover(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 1}
	b := NewBreaker(params)                              // Breaker capacity = 2
	want := []bool{true, true, false, false, true, true} // Shedding will stop when capacity opens up

	locks := b.concurrentRequests(4)
	unlockAll(locks)
	// Breaker recovers
	moreLocks := b.concurrentRequests(2)
	unlockAll(moreLocks)

	if diff := cmp.Diff(accepted(append(locks, moreLocks...)), want); diff != "" {
		t.Errorf("Unexpected accepted requests (-want +got): %s", diff)
	}
}

func TestBreakerLargeCapacityRecover(t *testing.T) {
	params := BreakerParams{QueueDepth: 5, MaxConcurrency: 45, InitialCapacity: 45}
	b := NewBreaker(params)   // Breaker capacity = 50
	want := make([]bool, 150) // Process 150 requests
	for i := 0; i < 50; i++ {
		want[i] = true // First 50 will fill the breaker capacity.
	}
	for i := 50; i < 100; i++ {
		want[i] = false // The next 50 will be shed.
	}
	for i := 100; i < 150; i++ {
		want[i] = true // The next 50 will be processed as capacity opens up.
	}

	// Send 100 requests.
	locks := b.concurrentRequests(100)
	// Process one request and send one request, 50 times.
	for i := 100; i < 150; i++ {
		// Open capacity
		unlock(locks[i-100])
		// Add another request
		locks = append(locks, b.concurrentRequest())
	}
	unlockAll(locks[50:])

	if diff := cmp.Diff(accepted(locks), want); diff != "" {
		t.Errorf("Unexpected accepted requests (-want +got): %s", diff)
	}
}

func TestBreaker_UpdateConcurrency(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 0}
	b := NewBreaker(params)
	b.UpdateConcurrency(1)
	if got, want := b.Capacity(), 1; got != want {
		t.Errorf("Capacity() = %d, want: %d", got, want)
	}

	b.UpdateConcurrency(0)
	if got, want := b.Capacity(), 0; got != want {
		t.Errorf("Capacity() = %d, want: %d", got, want)
	}

	if err := b.UpdateConcurrency(-2); err != ErrUpdateCapacity {
		t.Errorf("UpdateConcurrency = %v, want: %v", err, ErrUpdateCapacity)
	}
}

func TestBreaker_UpdateConcurrency_Overlow(t *testing.T) {
	params := BreakerParams{QueueDepth: 1, MaxConcurrency: 1, InitialCapacity: 0}
	b := NewBreaker(params)
	if err := b.UpdateConcurrency(2); err != ErrUpdateCapacity {
		t.Errorf("UpdateConcurrency = %v, want: %v", err, ErrUpdateCapacity)
	}
}

// Test empty semaphore, token cannot be acquired
func TestSemaphore_acquire_HasNoCapacity(t *testing.T) {
	gotChan := make(chan struct{}, 1)

	sem := newSemaphore(1, 0)
	tryAcquire(sem, gotChan)

	select {
	case <-gotChan:
		t.Error("Token was acquired but shouldn't have been")
	case <-time.After(semNoChangeTimeout):
		// Test succeeds, semaphore didn't change in configured time.
	}
}

// Test empty semaphore, add capacity, token can be acquired
func TestSemaphore_acquire_HasCapacity(t *testing.T) {
	gotChan := make(chan struct{}, 1)
	want := 1

	sem := newSemaphore(1, 0)
	tryAcquire(sem, gotChan)
	sem.release() // Allows 1 acquire

	for i := 0; i < want; i++ {
		select {
		case <-gotChan:
			// Successfully acquired a token.
		case <-time.After(semAcquireTimeout):
			t.Error("Was not able to acquire token before timeout")
		}
	}

	select {
	case <-gotChan:
		t.Errorf("Got more acquires than wanted, want = %d, got at least %d", want, want+1)
	case <-time.After(semNoChangeTimeout):
		// No change happened, success.
	}
}

func TestSemaphore_release(t *testing.T) {
	sem := newSemaphore(1, 1)
	sem.acquire()
	if err := sem.release(); err != nil {
		t.Errorf("release = %v; want: %v", err, nil)
	}
	if err := sem.release(); err != ErrRelease {
		t.Errorf("release = %v; want: %v", err, ErrRelease)
	}
}

func TestSemaphore_releasesSeveralReducers(t *testing.T) {
	const wantAfterFirstrelease = 1
	const wantAfterSecondrelease = 0
	sem := newSemaphore(2, 2)
	sem.acquire()
	sem.acquire()
	sem.updateCapacity(0)
	sem.release()
	if got := sem.Capacity(); got != wantAfterSecondrelease {
		t.Errorf("Capacity = %d, want: %d", got, wantAfterSecondrelease)
	}
	if sem.reducers != wantAfterFirstrelease {
		t.Errorf("sem.reducers = %d, want: %d", sem.reducers, wantAfterFirstrelease)
	}

	sem.release()
	if got := sem.Capacity(); got != wantAfterSecondrelease {
		t.Errorf("Capacity = %d, want: %d", got, wantAfterSecondrelease)
	}
	if sem.reducers != wantAfterSecondrelease {
		t.Errorf("sem.reducers = %d, want: %d", sem.reducers, wantAfterSecondrelease)
	}
}

func TestSemaphore_updateCapacity(t *testing.T) {
	const initialCapacity = 1
	sem := newSemaphore(3, initialCapacity)
	if got, want := sem.Capacity(), 1; got != want {
		t.Errorf("Capacity = %d, want: %d", got, want)
	}
	sem.acquire()
	sem.updateCapacity(initialCapacity + 2)
	if got, want := sem.Capacity(), 3; got != want {
		t.Errorf("Capacity = %d, want: %d", got, want)
	}
}

// Test the case when we add more capacity then the number of waiting reducers
func TestSemaphore_updateCapacity_LessThenReducers(t *testing.T) {
	const initialCapacity = 2
	sem := newSemaphore(2, initialCapacity)
	sem.acquire()
	sem.acquire()
	sem.updateCapacity(initialCapacity - 2)
	if got, want := sem.reducers, 2; got != want {
		t.Errorf("sem.reducers = %d, want: %d", got, want)
	}
	sem.release()
	sem.release()
	sem.release()
	if got, want := sem.reducers, 0; got != want {
		t.Errorf("sem.reducers = %d, want: %d", got, want)
	}
}

func TestSemaphore_updateCapacity_ConsumingReducers(t *testing.T) {
	const initialCapacity = 2
	sem := newSemaphore(2, initialCapacity)
	sem.acquire()
	sem.acquire()
	sem.updateCapacity(initialCapacity - 2)
	if got, want := sem.reducers, 2; got != want {
		t.Errorf("sem.reducers = %d, want: %d", got, want)
	}

	sem.updateCapacity(initialCapacity)
	if got, want := sem.reducers, 0; got != want {
		t.Errorf("sem.reducers = %d, want: %d", got, want)
	}
}

func TestSemaphore_updateCapacity_Overflow(t *testing.T) {
	sem := newSemaphore(2, 0)
	if err := sem.updateCapacity(3); err != ErrUpdateCapacity {
		t.Errorf("updateCapacity = %v, want: %v", err, ErrUpdateCapacity)
	}
}

func TestSemaphore_updateCapacity_OutOfBound(t *testing.T) {
	sem := newSemaphore(1, 1)
	sem.acquire()
	if err := sem.updateCapacity(-1); err != ErrUpdateCapacity {
		t.Errorf("updateCapacity = %v, want: %v", err, ErrUpdateCapacity)
	}
}

func TestSemaphore_updateCapacity_BrokenState(t *testing.T) {
	sem := newSemaphore(1, 0)
	sem.release() // This Release is not paired with an acquire
	if err := sem.updateCapacity(1); err != ErrUpdateCapacity {
		t.Errorf("updateCapacity = %v, want: %v", err, ErrUpdateCapacity)
	}
}

func TestSemaphore_updateCapacity_DoNothing(t *testing.T) {
	sem := newSemaphore(1, 1)
	if err := sem.updateCapacity(1); err != nil {
		t.Errorf("updateCapacity = %v, want: %v", err, nil)
	}
}

func TestSemaphore_WrongInitialCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic")
		}
	}()
	newSemaphore(1, 2)
}

// Attempts to perform a concurrent request against the specified breaker.
// Will wait for request to either be performed, enqueued or rejected.
func (b *Breaker) concurrentRequest() request {
	r := request{barrier: make(chan struct{}), accepted: make(chan bool, 1)}

	if len(b.sem.queue) > 0 {
		// Expect request to be performed
		defer waitForQueue(b.sem.queue, len(b.sem.queue)-1)
	} else if len(b.pendingRequests) < cap(b.pendingRequests) {
		// Expect request to be queued
		defer waitForQueue(b.pendingRequests, len(b.pendingRequests)+1)
	} else {
		// Expect request to be rejected
		defer r.wait()
	}

	var start sync.WaitGroup
	start.Add(1)
	go func() {
		start.Done()
		ok := b.Maybe(func() {
			<-r.barrier
		})
		r.accepted <- ok
	}()
	start.Wait() // Ensure that the go func has had a chance to execute.
	return r
}

// Perform n requests against the breaker, returning request objects.
func (b *Breaker) concurrentRequests(n int) []request {
	requests := make([]request, n)
	for i := range requests {
		requests[i] = b.concurrentRequest()
	}
	return requests
}

func waitForQueue(queue chan struct{}, size int) {
	if err := wait.PollImmediate(1*time.Millisecond, 100*time.Millisecond, func() (bool, error) {
		return len(queue) == size, nil
	}); err != nil {
		panic("timed out waiting for queue")
	}
}

func accepted(requests []request) []bool {
	got := make([]bool, len(requests))
	for i, r := range requests {
		got[i] = <-r.accepted
	}
	return got
}

func unlock(req request) {
	close(req.barrier)
	// Verify that function has completed
	req.wait()
}

func unlockAll(requests []request) {
	for _, lc := range requests {
		unlock(lc)
	}
}

func tryAcquire(sem *semaphore, gotChan chan struct{}) {
	go func() {
		// blocking until someone puts the token into the semaphore
		sem.acquire()
		gotChan <- struct{}{}
	}()
}
