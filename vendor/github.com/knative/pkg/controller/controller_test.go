/*
Copyright 2017 The Knative Authors

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

package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	. "github.com/knative/pkg/controller/testing"
	. "github.com/knative/pkg/logging/testing"
	. "github.com/knative/pkg/testing"
)

func TestPassNew(t *testing.T) {
	old := "foo"
	new := "bar"

	PassNew(func(got interface{}) {
		if new != got.(string) {
			t.Errorf("PassNew() = %v, wanted %v", got, new)
		}
	})(old, new)
}

var (
	boolTrue  = true
	boolFalse = false
	gvk       = schema.GroupVersionKind{
		Group:   "pkg.knative.dev",
		Version: "v1meta1",
		Kind:    "Parent",
	}
)

func TestFilter(t *testing.T) {
	filter := Filter(gvk)

	tests := []struct {
		name  string
		input interface{}
		want  bool
	}{{
		name:  "not a metav1.Object",
		input: "foo",
		want:  false,
	}, {
		name:  "nil",
		input: nil,
		want:  false,
	}, {
		name: "no owner reference",
		input: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
		},
		want: false,
	}, {
		name: "wrong owner reference, not controller",
		input: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "another.knative.dev/v1beta3",
					Kind:       "Parent",
					Controller: &boolFalse,
				}},
			},
		},
		want: false,
	}, {
		name: "right owner reference, not controller",
		input: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
					Controller: &boolFalse,
				}},
			},
		},
		want: false,
	}, {
		name: "wrong owner reference, but controller",
		input: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "another.knative.dev/v1beta3",
					Kind:       "Parent",
					Controller: &boolTrue,
				}},
			},
		},
		want: false,
	}, {
		name: "right owner reference, is controller",
		input: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
					Controller: &boolTrue,
				}},
			},
		},
		want: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := filter(test.input)
			if test.want != got {
				t.Errorf("Filter() = %v, wanted %v", got, test.want)
			}
		})
	}
}

type NopReconciler struct{}

func (nr *NopReconciler) Reconcile(context.Context, string) error {
	return nil
}

func TestEnqueues(t *testing.T) {
	tests := []struct {
		name      string
		work      func(*Impl)
		wantQueue []string
	}{{
		name: "do nothing",
		work: func(*Impl) {},
	}, {
		name: "enqueue key",
		work: func(impl *Impl) {
			impl.EnqueueKey("foo/bar")
		},
		wantQueue: []string{"foo/bar"},
	}, {
		name: "enqueue duplicate key",
		work: func(impl *Impl) {
			impl.EnqueueKey("foo/bar")
			impl.EnqueueKey("foo/bar")
		},
		// The queue deduplicates.
		wantQueue: []string{"foo/bar"},
	}, {
		name: "enqueue different keys",
		work: func(impl *Impl) {
			impl.EnqueueKey("foo/bar")
			impl.EnqueueKey("foo/baz")
		},
		wantQueue: []string{"foo/bar", "foo/baz"},
	}, {
		name: "enqueue resource",
		work: func(impl *Impl) {
			impl.Enqueue(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			})
		},
		wantQueue: []string{"bar/foo"},
	}, {
		name: "enqueue bad resource",
		work: func(impl *Impl) {
			impl.Enqueue("baz/blah")
		},
	}, {
		name: "enqueue controller of bad resource",
		work: func(impl *Impl) {
			impl.EnqueueControllerOf("baz/blah")
		},
	}, {
		name: "enqueue controller of resource without owner",
		work: func(impl *Impl) {
			impl.EnqueueControllerOf(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			})
		},
	}, {
		name: "enqueue controller of resource with owner",
		work: func(impl *Impl) {
			impl.EnqueueControllerOf(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: gvk.GroupVersion().String(),
						Kind:       gvk.Kind,
						Name:       "baz",
						Controller: &boolTrue,
					}},
				},
			})
		},
		wantQueue: []string{"bar/baz"},
	}, {
		name: "enqueue controller of deleted resource with owner",
		work: func(impl *Impl) {
			impl.EnqueueControllerOf(cache.DeletedFinalStateUnknown{
				Key: "foo/bar",
				Obj: &Resource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: gvk.GroupVersion().String(),
							Kind:       gvk.Kind,
							Name:       "baz",
							Controller: &boolTrue,
						}},
					},
				},
			})
		},
		wantQueue: []string{"bar/baz"},
	}, {
		name: "enqueue controller of deleted bad resource",
		work: func(impl *Impl) {
			impl.EnqueueControllerOf(cache.DeletedFinalStateUnknown{
				Key: "foo/bar",
				Obj: "bad-resource",
			})
		},
	}, {
		name: "enqueue label of namespaced resource bad resource",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("test-ns", "test-name")("baz/blah")
		},
	}, {
		name: "enqueue label of namespaced resource without label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("ns-key", "name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"ns-key": "bar",
					},
				},
			})
		},
	}, {
		name: "enqueue label of namespaced resource without namespace label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("ns-key", "name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"name-key": "baz",
					},
				},
			})
		},
	}, {
		name: "enqueue label of namespaced resource with labels",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("ns-key", "name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"ns-key":   "qux",
						"name-key": "baz",
					},
				},
			})
		},
		wantQueue: []string{"qux/baz"},
	}, {
		name: "enqueue label of namespaced resource with empty namespace label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("", "name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"name-key": "baz",
					},
				},
			})
		},
		wantQueue: []string{"bar/baz"},
	}, {
		name: "enqueue label of deleted namespaced resource with label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("ns-key", "name-key")(cache.DeletedFinalStateUnknown{
				Key: "foo/bar",
				Obj: &Resource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
						Labels: map[string]string{
							"ns-key":   "qux",
							"name-key": "baz",
						},
					},
				},
			})
		},
		wantQueue: []string{"qux/baz"},
	}, {
		name: "enqueue label of deleted bad namespaced resource",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfNamespaceScopedResource("ns-key", "name-key")(cache.DeletedFinalStateUnknown{
				Key: "foo/bar",
				Obj: "bad-resource",
			})
		},
	}, {
		name: "enqueue label of cluster scoped resource bad resource",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfClusterScopedResource("name-key")("baz")
		},
	}, {
		name: "enqueue label of cluster scoped resource without label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfClusterScopedResource("name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels:    map[string]string{},
				},
			})
		},
	}, {
		name: "enqueue label of cluster scoped resource with label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfClusterScopedResource("name-key")(&Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"name-key": "baz",
					},
				},
			})
		},
		wantQueue: []string{"baz"},
	}, {
		name: "enqueue label of deleted cluster scoped resource with label",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfClusterScopedResource("name-key")(cache.DeletedFinalStateUnknown{
				Key: "foo/bar",
				Obj: &Resource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
						Labels: map[string]string{
							"name-key": "baz",
						},
					},
				},
			})
		},
		wantQueue: []string{"baz"},
	}, {
		name: "enqueue label of deleted bad cluster scoped resource",
		work: func(impl *Impl) {
			impl.EnqueueLabelOfClusterScopedResource("name-key")(cache.DeletedFinalStateUnknown{
				Key: "bar",
				Obj: "bad-resource",
			})
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			impl := NewImpl(&NopReconciler{}, TestLogger(t), "Testing", &FakeStatsReporter{})
			test.work(impl)

			// The rate limit on our queue delays when things are added to the queue.
			time.Sleep(50 * time.Millisecond)
			impl.WorkQueue.ShutDown()
			gotQueue := drainWorkQueue(impl.WorkQueue)

			if diff := cmp.Diff(test.wantQueue, gotQueue); diff != "" {
				t.Errorf("unexpected queue (-want +got): %s", diff)
			}
		})
	}
}

type CountingReconciler struct {
	m     sync.Mutex
	Count int
}

func (cr *CountingReconciler) Reconcile(context.Context, string) error {
	cr.m.Lock()
	defer cr.m.Unlock()
	cr.Count++
	return nil
}

func TestStartAndShutdown(t *testing.T) {
	r := &CountingReconciler{}
	impl := NewImpl(r, TestLogger(t), "Testing", &FakeStatsReporter{})

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		StartAll(stopCh, impl)
	}()

	select {
	case <-time.After(10 * time.Millisecond):
		// We don't expect completion before the stopCh closes.
	case <-doneCh:
		t.Error("StartAll finished early.")
	}
	close(stopCh)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for controller to finish.")
	case <-doneCh:
		// We expect the work to complete.
	}

	if got, want := r.Count, 0; got != want {
		t.Errorf("Count = %v, wanted %v", got, want)
	}
}

func TestStartAndShutdownWithWork(t *testing.T) {
	r := &CountingReconciler{}
	reporter := &FakeStatsReporter{}
	impl := NewImpl(r, TestLogger(t), "Testing", reporter)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	impl.EnqueueKey("foo/bar")

	go func() {
		defer close(doneCh)
		StartAll(stopCh, impl)
	}()

	select {
	case <-time.After(10 * time.Millisecond):
		// We don't expect completion before the stopCh closes.
	case <-doneCh:
		t.Error("StartAll finished early.")
	}
	close(stopCh)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for controller to finish.")
	case <-doneCh:
		// We expect the work to complete.
	}

	if got, want := r.Count, 1; got != want {
		t.Errorf("Count = %v, wanted %v", got, want)
	}
	if got, want := impl.WorkQueue.NumRequeues("foo/bar"), 0; got != want {
		t.Errorf("Count = %v, wanted %v", got, want)
	}

	checkStats(t, reporter, 1, 0, 1, trueString)
}

type ErrorReconciler struct{}

func (er *ErrorReconciler) Reconcile(context.Context, string) error {
	return errors.New("I always error")
}

func TestStartAndShutdownWithErroringWork(t *testing.T) {
	r := &ErrorReconciler{}
	reporter := &FakeStatsReporter{}
	impl := NewImpl(r, TestLogger(t), "Testing", reporter)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	impl.EnqueueKey("foo/bar")

	go func() {
		defer close(doneCh)
		StartAll(stopCh, impl)
	}()

	select {
	case <-time.After(20 * time.Millisecond):
		// We don't expect completion before the stopCh closes.
	case <-doneCh:
		t.Error("StartAll finished early.")
	}
	close(stopCh)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for controller to finish.")
	case <-doneCh:
		// We expect the work to complete.
	}

	// Check that the work was requeued in RateLimiter.
	// As NumRequeues can't fully reflect the real state of queue length.
	// Here we need to wait for NumRequeues to be more than 1, to ensure
	// the key get re-queued and reprocessed as expect.
	if got, want := impl.WorkQueue.NumRequeues("foo/bar"), 3; got != want {
		t.Errorf("Requeue count = %v, wanted %v", got, want)
	}

	checkStats(t, reporter, 3, 0, 3, falseString)
}

type PermanentErrorReconciler struct{}

func (er *PermanentErrorReconciler) Reconcile(context.Context, string) error {
	err := errors.New("I always error")
	return NewPermanentError(err)
}

func TestStartAndShutdownWithPermanentErroringWork(t *testing.T) {
	r := &PermanentErrorReconciler{}
	reporter := &FakeStatsReporter{}
	impl := NewImpl(r, TestLogger(t), "Testing", reporter)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	impl.EnqueueKey("foo/bar")

	go func() {
		defer close(doneCh)
		StartAll(stopCh, impl)
	}()

	select {
	case <-time.After(20 * time.Millisecond):
		// We don't expect completion before the stopCh closes.
	case <-doneCh:
		t.Error("StartAll finished early.")
	}
	close(stopCh)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for controller to finish.")
	case <-doneCh:
		// We expect the work to complete.
	}

	// Check that the work was not requeued in RateLimiter.
	if got, want := impl.WorkQueue.NumRequeues("foo/bar"), 0; got != want {
		t.Errorf("Requeue count = %v, wanted %v", got, want)
	}

	checkStats(t, reporter, 1, 0, 1, falseString)
}

func drainWorkQueue(wq workqueue.RateLimitingInterface) (hasQueue []string) {
	for {
		key, shutdown := wq.Get()
		if key == nil && shutdown {
			break
		}
		hasQueue = append(hasQueue, key.(string))
	}
	return
}

type dummyInformer struct {
	cache.SharedInformer
}

type dummyStore struct {
	cache.Store
}

func (*dummyInformer) GetStore() cache.Store {
	return &dummyStore{}
}

var dummyKeys = []string{"foo/bar", "bar/foo", "fizz/buzz"}
var dummyObjs = []interface{}{"foo/bar", "bar/foo", "fizz/buzz"}

func (*dummyStore) ListKeys() []string {
	return dummyKeys
}

func (*dummyStore) List() []interface{} {
	return dummyObjs
}

func TestImplGlobalResync(t *testing.T) {
	r := &CountingReconciler{}
	impl := NewImpl(r, TestLogger(t), "Testing", &FakeStatsReporter{})

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		StartAll(stopCh, impl)
	}()

	impl.GlobalResync(&dummyInformer{})

	select {
	case <-time.After(10 * time.Millisecond):
		// We don't expect completion before the stopCh closes.
	case <-doneCh:
		t.Error("StartAll finished early.")
	}
	close(stopCh)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for controller to finish.")
	case <-doneCh:
		// We expect the work to complete.
	}

	if want, got := 3, r.Count; want != got {
		t.Errorf("GlobalResync: want = %v, got = %v", want, got)
	}
}

func checkStats(t *testing.T, r *FakeStatsReporter, reportCount, lastQueueDepth, reconcileCount int, lastReconcileSuccess string) {
	qd := r.GetQueueDepths()
	if got, want := len(qd), reportCount; got != want {
		t.Errorf("Queue depth reports = %v, wanted %v", got, want)
	}
	if got, want := qd[len(qd)-1], int64(lastQueueDepth); got != want {
		t.Errorf("Queue depth report = %v, wanted %v", got, want)
	}
	rd := r.GetReconcileData()
	if got, want := len(rd), reconcileCount; got != want {
		t.Errorf("Reconcile reports = %v, wanted %v", got, want)
	}
	if got, want := rd[len(rd)-1].Success, lastReconcileSuccess; got != want {
		t.Errorf("Reconcile success = %v, wanted %v", got, want)
	}
}

type fixedInformer struct {
	m    sync.Mutex
	sunk bool
}

var _ Informer = (*fixedInformer)(nil)

func (fi *fixedInformer) Run(<-chan struct{}) {}
func (fi *fixedInformer) HasSynced() bool {
	fi.m.Lock()
	defer fi.m.Unlock()
	return fi.sunk
}

func (fi *fixedInformer) ToggleSynced(b bool) {
	fi.m.Lock()
	defer fi.m.Unlock()
	fi.sunk = b
}

func TestStartInformersSuccess(t *testing.T) {
	errCh := make(chan error)
	defer close(errCh)

	fi := &fixedInformer{sunk: true}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go func() {
		errCh <- StartInformers(stopCh, fi)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for informers to sync.")
	}
}

func TestStartInformersEventualSuccess(t *testing.T) {
	errCh := make(chan error)
	defer close(errCh)

	fi := &fixedInformer{sunk: false}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go func() {
		errCh <- StartInformers(stopCh, fi)
	}()

	select {
	case err := <-errCh:
		t.Errorf("Unexpected send on errCh: %v", err)
	case <-time.After(1 * time.Second):
		// Wait a brief period to ensure nothing is sent.
	}

	// Let the Sync complete.
	fi.ToggleSynced(true)

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for informers to sync.")
	}
}

func TestStartInformersFailure(t *testing.T) {
	errCh := make(chan error)
	defer close(errCh)

	fi := &fixedInformer{sunk: false}

	stopCh := make(chan struct{})
	go func() {
		errCh <- StartInformers(stopCh, fi)
	}()

	select {
	case err := <-errCh:
		t.Errorf("Unexpected send on errCh: %v", err)
	case <-time.After(1 * time.Second):
		// Wait a brief period to ensure nothing is sent.
	}

	// Now close the stopCh and we should see an error sent.
	close(stopCh)

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Unexpected success syncing informers after stopCh closed.")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for informers to sync.")
	}
}
