/*
Copyright 2018 The Kubernetes Authors.

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

package source_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("Source", func() {
	Describe("Kind", func() {
		var c chan struct{}
		var p *corev1.Pod
		var ic *informertest.FakeInformers

		BeforeEach(func(done Done) {
			ic = &informertest.FakeInformers{}
			c = make(chan struct{})
			p = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test", Image: "test"},
					},
				},
			}
			close(done)
		})

		Context("for a Pod resource", func() {
			It("should provide a Pod CreateEvent", func(done Done) {
				c := make(chan struct{})
				p := &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "test"},
						},
					},
				}

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Kind{
					Type: &corev1.Pod{},
				}
				inject.CacheInto(ic, instance)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						close(c)
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Add(p)
				<-c
				close(done)
			})

			It("should provide a Pod UpdateEvent", func(done Done) {
				p2 := p.DeepCopy()
				p2.SetLabels(map[string]string{"biz": "baz"})

				ic := &informertest.FakeInformers{}
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Kind{
					Type: &corev1.Pod{},
				}
				instance.InjectCache(ic)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(evt event.UpdateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.MetaOld).To(Equal(p))
						Expect(evt.ObjectOld).To(Equal(p))

						Expect(evt.MetaNew).To(Equal(p2))
						Expect(evt.ObjectNew).To(Equal(p2))

						close(c)
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Update(p, p2)
				<-c
				close(done)
			})

			It("should provide a Pod DeletedEvent", func(done Done) {
				c := make(chan struct{})
				p := &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "test"},
						},
					},
				}

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Kind{
					Type: &corev1.Pod{},
				}
				inject.CacheInto(ic, instance)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(evt event.DeleteEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						close(c)
					},
					GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Delete(p)
				<-c
				close(done)
			})
		})

		It("should return an error from Start if informers were not injected", func(done Done) {
			instance := source.Kind{Type: &corev1.Pod{}}
			err := instance.Start(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must call CacheInto on Kind before calling Start"))

			close(done)
		})

		It("should return an error from Start if a type was not provided", func(done Done) {
			instance := source.Kind{}
			instance.InjectCache(&informertest.FakeInformers{})
			err := instance.Start(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must specify Kind.Type"))

			close(done)
		})

		Context("for a Kind not in the cache", func() {
			It("should return an error when Start is called", func(done Done) {
				ic.Error = fmt.Errorf("test error")
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")

				instance := &source.Kind{
					Type: &corev1.Pod{},
				}
				instance.InjectCache(ic)
				err := instance.Start(handler.Funcs{}, q)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Func", func() {
		It("should be called from Start", func(done Done) {
			run := false
			instance := source.Func(func(
				handler.EventHandler,
				workqueue.RateLimitingInterface, ...predicate.Predicate) error {
				run = true
				return nil
			})
			Expect(instance.Start(nil, nil)).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())

			expected := fmt.Errorf("expected error: Func")
			instance = source.Func(func(
				handler.EventHandler,
				workqueue.RateLimitingInterface, ...predicate.Predicate) error {
				return expected
			})
			Expect(instance.Start(nil, nil)).To(Equal(expected))

			close(done)
		})
	})

	Describe("Channel", func() {
		var stop chan struct{}
		var ch chan event.GenericEvent

		BeforeEach(func() {
			stop = make(chan struct{})
			ch = make(chan event.GenericEvent)
		})

		AfterEach(func() {
			close(stop)
			close(ch)
		})

		Context("for a source", func() {
			It("should provide a GenericEvent", func(done Done) {
				ch := make(chan event.GenericEvent)
				c := make(chan struct{})
				p := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				}
				evt := event.GenericEvent{
					Object: p,
					Meta:   p,
				}
				// Event that should be filtered out by predicates
				invalidEvt := event.GenericEvent{}

				// Predicate to filter out empty event
				prct := predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool {
						return e.Object != nil && e.Meta != nil
					},
				}

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Channel{Source: ch}
				inject.StopChannelInto(stop, instance)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(evt event.GenericEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						// The empty event should have been filtered out by the predicates,
						// and will not be passed to the handler.
						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						close(c)
					},
				}, q, prct)
				Expect(err).NotTo(HaveOccurred())

				ch <- invalidEvt
				ch <- evt
				<-c
				close(done)
			})
			It("should get pending events processed once channel unblocked", func(done Done) {
				ch := make(chan event.GenericEvent)
				unblock := make(chan struct{})
				processed := make(chan struct{})
				evt := event.GenericEvent{}
				eventCount := 0

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				// Add a handler to get distribution blocked
				instance := &source.Channel{Source: ch}
				instance.DestBufferSize = 1
				inject.StopChannelInto(stop, instance)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(evt event.GenericEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						// Block for the first time
						if eventCount == 0 {
							<-unblock
						}
						eventCount += 1

						if eventCount == 3 {
							close(processed)
						}
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				// Write 3 events into the source channel.
				// The 1st should be passed into the generic func of the handler;
				// The 2nd should be fetched out of the source channel, and waiting to write into dest channel;
				// The 3rd should be pending in the source channel.
				ch <- evt
				ch <- evt
				ch <- evt

				// Validate none of the events have been processed.
				Expect(eventCount).To(Equal(0))

				close(unblock)

				<-processed

				// Validate all of the events have been processed.
				Expect(eventCount).To(Equal(3))

				close(done)
			})
			It("should get error if no source specified", func(done Done) {
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Channel{ /*no source specified*/ }
				inject.StopChannelInto(stop, instance)
				err := instance.Start(handler.Funcs{}, q)
				Expect(err).To(Equal(fmt.Errorf("must specify Channel.Source")))
				close(done)
			})
			It("should get error if no stop channel injected", func(done Done) {
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Channel{Source: ch}
				err := instance.Start(handler.Funcs{}, q)
				Expect(err).To(Equal(fmt.Errorf("must call InjectStop on Channel before calling Start")))
				close(done)
			})

		})
		Context("for multi sources (handlers)", func() {
			It("should provide GenericEvents for all handlers", func(done Done) {
				ch := make(chan event.GenericEvent)
				p := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				}
				evt := event.GenericEvent{
					Object: p,
					Meta:   p,
				}

				var resEvent1, resEvent2 event.GenericEvent
				c1 := make(chan struct{})
				c2 := make(chan struct{})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Channel{Source: ch}
				inject.StopChannelInto(stop, instance)
				err := instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(evt event.GenericEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						resEvent1 = evt
						close(c1)
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				err = instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(evt event.GenericEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						resEvent2 = evt
						close(c2)
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				ch <- evt
				<-c1
				<-c2

				// Validate the two handlers received same event
				Expect(resEvent1).To(Equal(resEvent2))
				close(done)
			})
		})
	})
})
