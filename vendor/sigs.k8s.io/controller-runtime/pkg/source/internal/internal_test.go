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

package internal_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source/internal"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("Internal", func() {

	var instance internal.EventHandler
	var funcs, setfuncs *handler.Funcs
	var set bool
	BeforeEach(func() {
		funcs = &handler.Funcs{
			CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect CreateEvent to be called.")
			},
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect DeleteEvent to be called.")
			},
			UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect UpdateEvent to be called.")
			},
			GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect GenericEvent to be called.")
			},
		}

		setfuncs = &handler.Funcs{
			CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
				set = true
			},
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				set = true
			},
			UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
				set = true
			},
			GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
				set = true
			},
		}
		instance = internal.EventHandler{
			Queue:        controllertest.Queue{},
			EventHandler: funcs,
		}
	})

	Describe("EventHandler", func() {
		var pod, newPod *corev1.Pod

		BeforeEach(func() {
			pod = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test", Image: "test"}},
				},
			}
			newPod = pod.DeepCopy()
			newPod.Labels = map[string]string{"foo": "bar"}
		})

		It("should create a CreateEvent", func(done Done) {
			funcs.CreateFunc = func(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))
				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnAdd(pod)
			close(done)
		})

		It("should used Predicates to filter CreateEvents", func(done Done) {
			instance = internal.EventHandler{
				Queue:        controllertest.Queue{},
				EventHandler: setfuncs,
			}

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return false }},
			}
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
			}
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return false }},
			}
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return false }},
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
			}
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
			}
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())

			close(done)
		})

		It("should not call Create EventHandler if the object is not a runtime.Object", func(done Done) {
			instance.OnAdd(&metav1.ObjectMeta{})
			close(done)
		})

		It("should not call Create EventHandler if the object does not have metadata", func(done Done) {
			instance.OnAdd(FooRuntimeObject{})
			close(done)
		})

		It("should create an UpdateEvent", func(done Done) {
			funcs.UpdateFunc = func(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))

				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.MetaOld).To(Equal(m))
				Expect(evt.ObjectOld).To(Equal(pod))

				m, err = meta.Accessor(newPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.MetaNew).To(Equal(m))
				Expect(evt.ObjectNew).To(Equal(newPod))
			}
			instance.OnUpdate(pod, newPod)
			close(done)
		})

		It("should used Predicates to filter UpdateEvents", func(done Done) {
			instance = internal.EventHandler{
				Queue:        controllertest.Queue{},
				EventHandler: setfuncs,
			}

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{UpdateFunc: func(updateEvent event.UpdateEvent) bool { return false }},
			}
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{UpdateFunc: func(event.UpdateEvent) bool { return true }},
			}
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{UpdateFunc: func(event.UpdateEvent) bool { return true }},
				predicate.Funcs{UpdateFunc: func(event.UpdateEvent) bool { return false }},
			}
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{UpdateFunc: func(event.UpdateEvent) bool { return false }},
				predicate.Funcs{UpdateFunc: func(event.UpdateEvent) bool { return true }},
			}
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
				predicate.Funcs{CreateFunc: func(event.CreateEvent) bool { return true }},
			}
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())

			close(done)
		})

		It("should not call Update EventHandler if the object is not a runtime.Object", func(done Done) {
			instance.OnUpdate(&metav1.ObjectMeta{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, &metav1.ObjectMeta{})
			close(done)
		})

		It("should not call Update EventHandler if the object does not have metadata", func(done Done) {
			instance.OnUpdate(FooRuntimeObject{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, FooRuntimeObject{})
			close(done)
		})

		It("should create a DeleteEvent", func(done Done) {
			funcs.DeleteFunc = func(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))

				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnDelete(pod)
			close(done)
		})

		It("should used Predicates to filter DeleteEvents", func(done Done) {
			instance = internal.EventHandler{
				Queue:        controllertest.Queue{},
				EventHandler: setfuncs,
			}

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return false }},
			}
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return true }},
			}
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return true }},
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return false }},
			}
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return false }},
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return true }},
			}
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance.Predicates = []predicate.Predicate{
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return true }},
				predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return true }},
			}
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())

			close(done)
		})

		It("should not call Delete EventHandler if the object is not a runtime.Object", func(done Done) {
			instance.OnDelete(&metav1.ObjectMeta{})
			close(done)
		})

		It("should not call Delete EventHandler if the object does not have metadata", func(done Done) {
			instance.OnDelete(FooRuntimeObject{})
			close(done)
		})

		It("should create a DeleteEvent from a tombstone", func(done Done) {

			tombstone := cache.DeletedFinalStateUnknown{
				Obj: pod,
			}
			funcs.DeleteFunc = func(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))
				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}

			instance.OnDelete(tombstone)
			close(done)
		})

		It("should ignore tombstone objects without meta", func(done Done) {
			tombstone := cache.DeletedFinalStateUnknown{Obj: Foo{}}
			instance.OnDelete(tombstone)
			close(done)
		})
		It("should ignore objects without meta", func(done Done) {
			instance.OnAdd(Foo{})
			instance.OnUpdate(Foo{}, Foo{})
			instance.OnDelete(Foo{})
			close(done)
		})
	})
})

type Foo struct{}

var _ runtime.Object = FooRuntimeObject{}

type FooRuntimeObject struct{}

func (FooRuntimeObject) GetObjectKind() schema.ObjectKind { return nil }
func (FooRuntimeObject) DeepCopyObject() runtime.Object   { return nil }
