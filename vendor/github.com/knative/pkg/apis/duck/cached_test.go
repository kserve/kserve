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

package duck

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type BlockingInformerFactory struct {
	lock    sync.Mutex
	getting int32
}

var _ InformerFactory = (*BlockingInformerFactory)(nil)

func (bif *BlockingInformerFactory) Get(gvr schema.GroupVersionResource) (cache.SharedIndexInformer, cache.GenericLister, error) {
	atomic.AddInt32(&bif.getting, 1)
	// Wait here until we can acquire the lock!
	bif.lock.Lock()
	bif.lock.Unlock()
	return nil, nil, nil
}

func TestSameGVR(t *testing.T) {
	bif := &BlockingInformerFactory{}

	// Suspend progress.
	bif.lock.Lock()

	cif := &CachedInformerFactory{
		Delegate: bif,
	}

	grp, _ := errgroup.WithContext(context.TODO())
	returned := int32(0)

	// Use the same GVR each iteration to ensure we hit the cache
	// and don't initialize too many InformerFactory's thru our
	// Delegate.
	gvr := schema.GroupVersionResource{
		Group:    "testing.knative.dev",
		Version:  "v3",
		Resource: "caches",
	}
	for i := 0; i < 10; i++ {
		grp.Go(func() error {
			_, _, err := cif.Get(gvr)
			atomic.AddInt32(&returned, 1)
			return err
		})
	}

	// Give the goroutines time to make progress.
	time.Sleep(100 * time.Millisecond)

	// Check that none have returned and we have one Get in progress.
	if got, want := atomic.LoadInt32(&returned), int32(0); got != want {
		t.Errorf("Got %v returned, wanted %v", got, want)
	}
	if got, want := atomic.LoadInt32(&bif.getting), int32(1); got != want {
		t.Errorf("Got %v calls to bif.Get, wanted %v", got, want)
	}

	// Allow the Get calls to proceed.
	bif.lock.Unlock()

	if err := grp.Wait(); err != nil {
		t.Errorf("Wait() = %v", err)
	}
}

func TestDifferentGVRs(t *testing.T) {
	bif := &BlockingInformerFactory{}

	// Suspend progress.
	bif.lock.Lock()

	cif := &CachedInformerFactory{
		Delegate: bif,
	}

	grp, _ := errgroup.WithContext(context.TODO())
	returned := int32(0)
	for i := 0; i < 10; i++ {
		// Use a different GVR each iteration to check that calls
		// to bif.Get can proceed even if a call is in progress
		// for another GVR.
		gvr := schema.GroupVersionResource{
			Group:    "testing.knative.dev",
			Version:  fmt.Sprintf("v%d", i),
			Resource: "caches",
		}
		grp.Go(func() error {
			_, _, err := cif.Get(gvr)
			atomic.AddInt32(&returned, 1)
			return err
		})
	}

	// Give the goroutines time to make progress.
	time.Sleep(100 * time.Millisecond)

	// Check that none have returned and we have 10 Gets in progress.
	if got, want := atomic.LoadInt32(&returned), int32(0); got != want {
		t.Errorf("Got %v returned, wanted %v", got, want)
	}
	if got, want := atomic.LoadInt32(&bif.getting), int32(10); got != want {
		t.Errorf("Got %v calls to bif.Get, wanted %v", got, want)
	}

	// Allow the Get calls to proceed.
	bif.lock.Unlock()

	if err := grp.Wait(); err != nil {
		t.Errorf("Wait() = %v", err)
	}
}
