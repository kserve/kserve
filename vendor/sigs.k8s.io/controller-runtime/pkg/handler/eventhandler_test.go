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

package handler_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Eventhandler", func() {
	var q workqueue.RateLimitingInterface
	var instance handler.EnqueueRequestForObject
	var pod *corev1.Pod
	t := true
	BeforeEach(func() {
		q = controllertest.Queue{Interface: workqueue.New()}
		instance = handler.EnqueueRequestForObject{}
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "baz"},
		}
	})

	Describe("EnqueueRequestForObject", func() {
		It("should enqueue a Request with the Name / Namespace of the object in the CreateEvent.", func(done Done) {
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.Request)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		It("should enqueue a Request with the Name / Namespace of the object in the DeleteEvent.", func(done Done) {
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Delete(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.Request)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		It("should enqueue a Request with the Name / Namespace of both objects in the UpdateEvent.",
			func(done Done) {
				newPod := pod.DeepCopy()
				newPod.Name = "baz2"
				newPod.Namespace = "biz2"

				evt := event.UpdateEvent{
					ObjectOld: pod,
					MetaOld:   pod.GetObjectMeta(),
					ObjectNew: newPod,
					MetaNew:   newPod.GetObjectMeta(),
				}
				instance.Update(evt, q)
				Expect(q.Len()).To(Equal(2))

				i, _ := q.Get()
				Expect(i).NotTo(BeNil())
				req, ok := i.(reconcile.Request)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

				i, _ = q.Get()
				Expect(i).NotTo(BeNil())
				req, ok = i.(reconcile.Request)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz2", Name: "baz2"}))

				close(done)
			})

		It("should enqueue a Request with the Name / Namespace of the object in the GenericEvent.", func(done Done) {
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Generic(evt, q)
			Expect(q.Len()).To(Equal(1))
			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.Request)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		Context("for a runtime.Object without Metadata", func() {
			It("should do nothing if the Metadata is missing for a CreateEvent.", func(done Done) {
				evt := event.CreateEvent{
					Object: pod,
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})

			It("should do nothing if the Metadata is missing for a UpdateEvent.", func(done Done) {
				newPod := pod.DeepCopy()
				newPod.Name = "baz2"
				newPod.Namespace = "biz2"

				evt := event.UpdateEvent{
					ObjectNew: newPod,
					MetaNew:   newPod.GetObjectMeta(),
					ObjectOld: pod,
				}
				instance.Update(evt, q)
				Expect(q.Len()).To(Equal(1))
				i, _ := q.Get()
				Expect(i).NotTo(BeNil())
				req, ok := i.(reconcile.Request)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz2", Name: "baz2"}))

				evt.MetaNew = nil
				evt.MetaOld = pod.GetObjectMeta()
				instance.Update(evt, q)
				Expect(q.Len()).To(Equal(1))
				i, _ = q.Get()
				Expect(i).NotTo(BeNil())
				req, ok = i.(reconcile.Request)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

				close(done)
			})

			It("should do nothing if the Metadata is missing for a DeleteEvent.", func(done Done) {
				evt := event.DeleteEvent{
					Object: pod,
				}
				instance.Delete(evt, q)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})

			It("should do nothing if the Metadata is missing for a GenericEvent.", func(done Done) {
				evt := event.GenericEvent{
					Object: pod,
				}
				instance.Generic(evt, q)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})
		})
	})

	Describe("EnqueueRequestsFromMapFunc", func() {
		It("should enqueue a Request with the function applied to the CreateEvent.", func() {
			req := []reconcile.Request{}
			instance := handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
					defer GinkgoRecover()
					Expect(a.Meta).To(Equal(pod.GetObjectMeta()))
					Expect(a.Object).To(Equal(pod))
					req = []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"},
						},
					}
					return req
				}),
			}

			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(2))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}))

			i, _ = q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"}}))
		})

		It("should enqueue a Request with the function applied to the DeleteEvent.", func() {
			req := []reconcile.Request{}
			instance := handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
					defer GinkgoRecover()
					Expect(a.Meta).To(Equal(pod.GetObjectMeta()))
					Expect(a.Object).To(Equal(pod))
					req = []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"},
						},
					}
					return req
				}),
			}

			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Delete(evt, q)
			Expect(q.Len()).To(Equal(2))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}))

			i, _ = q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"}}))
		})

		It("should enqueue a Request with the function applied to both objects in the UpdateEvent.",
			func() {
				newPod := pod.DeepCopy()
				newPod.Name = pod.Name + "2"
				newPod.Namespace = pod.Namespace + "2"

				req := []reconcile.Request{}
				instance := handler.EnqueueRequestsFromMapFunc{
					ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
						defer GinkgoRecover()
						req = []reconcile.Request{
							{
								NamespacedName: types.NamespacedName{Namespace: "foo", Name: a.Meta.GetName() + "-bar"},
							},
							{
								NamespacedName: types.NamespacedName{Namespace: "biz", Name: a.Meta.GetName() + "-baz"},
							},
						}
						return req
					}),
				}

				evt := event.UpdateEvent{
					ObjectOld: pod,
					MetaOld:   pod.GetObjectMeta(),
					ObjectNew: newPod,
					MetaNew:   newPod.GetObjectMeta(),
				}
				instance.Update(evt, q)
				Expect(q.Len()).To(Equal(4))

				i, _ := q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: "foo", Name: "baz-bar"}}))

				i, _ = q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz-baz"}}))

				i, _ = q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: "foo", Name: "baz2-bar"}}))

				i, _ = q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz2-baz"}}))
			})

		It("should enqueue a Request with the function applied to the GenericEvent.", func() {
			req := []reconcile.Request{}
			instance := handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
					defer GinkgoRecover()
					Expect(a.Meta).To(Equal(pod.GetObjectMeta()))
					Expect(a.Object).To(Equal(pod))
					req = []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"},
						},
					}
					return req
				}),
			}

			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Generic(evt, q)
			Expect(q.Len()).To(Equal(2))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}))

			i, _ = q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "biz", Name: "baz"}}))
		})
	})

	Describe("EnqueueRequestForOwner", func() {
		It("should enqueue a Request with the Owner of the object in the CreateEvent.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)

			pod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v1",
				},
			}
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo-parent"}}))
		})

		It("should enqueue a Request with the Owner of the object in the DeleteEvent.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)

			pod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v1",
				},
			}
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Delete(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo-parent"}}))
		})

		It("should enqueue a Request with the Owners of both objects in the UpdateEvent.", func() {
			newPod := pod.DeepCopy()
			newPod.Name = pod.Name + "2"
			newPod.Namespace = pod.Namespace + "2"

			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)

			pod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo1-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v1",
				},
			}
			newPod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo2-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v1",
				},
			}
			evt := event.UpdateEvent{
				ObjectOld: pod,
				MetaOld:   pod.GetObjectMeta(),
				ObjectNew: newPod,
				MetaNew:   newPod.GetObjectMeta(),
			}
			instance.Update(evt, q)
			Expect(q.Len()).To(Equal(2))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo1-parent"}}))

			i, _ = q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: newPod.GetNamespace(), Name: "foo2-parent"}}))
		})

		It("should enqueue a Request with the Owner of the object in the GenericEvent.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)

			pod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v1",
				},
			}
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Generic(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo-parent"}}))
		})

		It("should not enqueue a Request if there are no owners matching Group and Kind.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType:    &appsv1.ReplicaSet{},
				IsController: t,
			}
			instance.InjectScheme(scheme.Scheme)
			pod.OwnerReferences = []metav1.OwnerReference{
				{ // Wrong group
					Name:       "foo1-parent",
					Kind:       "ReplicaSet",
					APIVersion: "extensions/v1",
				},
				{ // Wrong kind
					Name:       "foo2-parent",
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
			}
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(0))
		})

		It("should enqueue a Request if there are owners matching Group "+
			"and Kind with a different version.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)
			pod.OwnerReferences = []metav1.OwnerReference{
				{
					Name:       "foo-parent",
					Kind:       "ReplicaSet",
					APIVersion: "apps/v2",
				},
			}
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo-parent"}}))
		})

		It("should not enqueue a Request if there are no owners.", func() {
			instance := handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.ReplicaSet{},
			}
			instance.InjectScheme(scheme.Scheme)
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			Expect(q.Len()).To(Equal(0))
		})

		Context("with the Controller field set to true", func() {
			It("should enqueue reconcile.Requests for only the first the Controller if there are "+
				"multiple Controller owners.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType:    &appsv1.ReplicaSet{},
					IsController: t,
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo2-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
						Controller: &t,
					},
					{
						Name:       "foo3-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo4-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
						Controller: &t,
					},
					{
						Name:       "foo5-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(1))
				i, _ := q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo2-parent"}}))
			})

			It("should not enqueue reconcile.Requests if there are no Controller owners.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType:    &appsv1.ReplicaSet{},
					IsController: t,
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo2-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo3-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})

			It("should not enqueue reconcile.Requests if there are no owners.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType:    &appsv1.ReplicaSet{},
					IsController: t,
				}
				instance.InjectScheme(scheme.Scheme)
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})

		Context("with the Controller field set to false", func() {
			It("should enqueue a reconcile.Requests for all owners.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType: &appsv1.ReplicaSet{},
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo2-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					{
						Name:       "foo3-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(3))

				i, _ := q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo1-parent"}}))
				i, _ = q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo2-parent"}}))
				i, _ = q.Get()
				Expect(i).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: "foo3-parent"}}))
			})
		})

		Context("with a nil metadata object", func() {
			It("should do nothing.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType: &appsv1.ReplicaSet{},
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})

		Context("with a multiple matching kinds", func() {
			It("should do nothing.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType: &metav1.ListOptions{},
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ListOptions",
						APIVersion: "meta/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})
		Context("with an OwnerType that cannot be resolved", func() {
			It("should do nothing.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType: &controllertest.ErrorType{},
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ListOptions",
						APIVersion: "meta/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})

		Context("with a nil OwnerType", func() {
			It("should do nothing.", func() {
				instance := handler.EnqueueRequestForOwner{}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "OwnerType",
						APIVersion: "meta/v1",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})

		Context("with an invalid APIVersion in the OwnerReference", func() {
			It("should do nothing.", func() {
				instance := handler.EnqueueRequestForOwner{
					OwnerType: &appsv1.ReplicaSet{},
				}
				instance.InjectScheme(scheme.Scheme)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Name:       "foo1-parent",
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1/fail",
					},
				}
				evt := event.CreateEvent{
					Object: pod,
					Meta:   pod.GetObjectMeta(),
				}
				instance.Create(evt, q)
				Expect(q.Len()).To(Equal(0))
			})
		})
	})

	Describe("Funcs", func() {
		failingFuncs := handler.Funcs{
			CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect CreateEvent to be called.")
			},
			DeleteFunc: func(event.DeleteEvent, workqueue.RateLimitingInterface) {
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

		It("should call CreateFunc for a CreateEvent if provided.", func(done Done) {
			instance := failingFuncs
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.CreateFunc = func(evt2 event.CreateEvent, q2 workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q2).To(Equal(q))
				Expect(evt2).To(Equal(evt))
			}
			instance.Create(evt, q)
			close(done)
		})

		It("should NOT call CreateFunc for a CreateEvent if NOT provided.", func(done Done) {
			instance := failingFuncs
			instance.CreateFunc = nil
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(evt, q)
			close(done)
		})

		It("should call UpdateFunc for an UpdateEvent if provided.", func(done Done) {
			newPod := pod.DeepCopy()
			newPod.Name = pod.Name + "2"
			newPod.Namespace = pod.Namespace + "2"
			evt := event.UpdateEvent{
				ObjectOld: pod,
				MetaOld:   pod.GetObjectMeta(),
				ObjectNew: newPod,
				MetaNew:   newPod.GetObjectMeta(),
			}

			instance := failingFuncs
			instance.UpdateFunc = func(evt2 event.UpdateEvent, q2 workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q2).To(Equal(q))
				Expect(evt2).To(Equal(evt))
			}

			instance.Update(evt, q)
			close(done)
		})

		It("should NOT call UpdateFunc for an UpdateEvent if NOT provided.", func(done Done) {
			newPod := pod.DeepCopy()
			newPod.Name = pod.Name + "2"
			newPod.Namespace = pod.Namespace + "2"
			evt := event.UpdateEvent{
				ObjectOld: pod,
				MetaOld:   pod.GetObjectMeta(),
				ObjectNew: newPod,
				MetaNew:   newPod.GetObjectMeta(),
			}
			instance.Update(evt, q)
			close(done)
		})

		It("should call DeleteFunc for a DeleteEvent if provided.", func(done Done) {
			instance := failingFuncs
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.DeleteFunc = func(evt2 event.DeleteEvent, q2 workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q2).To(Equal(q))
				Expect(evt2).To(Equal(evt))
			}
			instance.Delete(evt, q)
			close(done)
		})

		It("should NOT call DeleteFunc for a DeleteEvent if NOT provided.", func(done Done) {
			instance := failingFuncs
			instance.DeleteFunc = nil
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Delete(evt, q)
			close(done)
		})

		It("should call GenericFunc for a GenericEvent if provided.", func(done Done) {
			instance := failingFuncs
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.GenericFunc = func(evt2 event.GenericEvent, q2 workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(q2).To(Equal(q))
				Expect(evt2).To(Equal(evt))
			}
			instance.Generic(evt, q)
			close(done)
		})

		It("should NOT call GenericFunc for a GenericEvent if NOT provided.", func(done Done) {
			instance := failingFuncs
			instance.GenericFunc = nil
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Generic(evt, q)
			close(done)
		})
	})
})
