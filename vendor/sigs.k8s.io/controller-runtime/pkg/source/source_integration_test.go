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
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("Source", func() {
	var instance1, instance2 *source.Kind
	var obj runtime.Object
	var q workqueue.RateLimitingInterface
	var c1, c2 chan interface{}
	var ns string
	count := 0

	BeforeEach(func(done Done) {
		// Create the namespace for the test
		ns = fmt.Sprintf("controller-source-kindsource-%v", count)
		count++
		_, err := clientset.CoreV1().Namespaces().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		q = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
		c1 = make(chan interface{})
		c2 = make(chan interface{})

		close(done)
	})

	JustBeforeEach(func() {
		instance1 = &source.Kind{Type: obj}
		inject.CacheInto(icache, instance1)

		instance2 = &source.Kind{Type: obj}
		inject.CacheInto(icache, instance2)
	})

	AfterEach(func(done Done) {
		err := clientset.CoreV1().Namespaces().Delete(ns, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
		close(c1)
		close(c2)

		close(done)
	})

	Describe("Kind", func() {
		Context("for a Deployment resource", func() {
			obj = &appsv1.Deployment{}

			It("should provide Deployment Events", func(done Done) {
				var created, updated, deleted *appsv1.Deployment
				var err error

				// Get the client and Deployment used to create events
				client := clientset.AppsV1().Deployments(ns)
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-name"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"foo": "bar"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				// Create an event handler to verify the events
				newHandler := func(c chan interface{}) handler.Funcs {
					return handler.Funcs{
						CreateFunc: func(evt event.CreateEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
						UpdateFunc: func(evt event.UpdateEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
						DeleteFunc: func(evt event.DeleteEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
					}
				}
				handler1 := newHandler(c1)
				handler2 := newHandler(c2)

				// Create 2 instances
				instance1.Start(handler1, q)
				instance2.Start(handler2, q)

				By("Creating a Deployment and expecting the CreateEvent.")
				created, err = client.Create(deployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(created).NotTo(BeNil())

				// Check first CreateEvent
				evt := <-c1
				createEvt, ok := evt.(event.CreateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.CreateEvent{}))
				Expect(createEvt.Meta).To(Equal(created))
				Expect(createEvt.Object).To(Equal(created))

				// Check second CreateEvent
				evt = <-c2
				createEvt, ok = evt.(event.CreateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.CreateEvent{}))
				Expect(createEvt.Meta).To(Equal(created))
				Expect(createEvt.Object).To(Equal(created))

				By("Updating a Deployment and expecting the UpdateEvent.")
				updated = created.DeepCopy()
				updated.Labels = map[string]string{"biz": "buz"}
				updated, err = client.Update(updated)
				Expect(err).NotTo(HaveOccurred())

				// Check first UpdateEvent
				evt = <-c1
				updateEvt, ok := evt.(event.UpdateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.UpdateEvent{}))

				Expect(updateEvt.MetaNew).To(Equal(updated))
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.MetaOld).To(Equal(created))
				Expect(updateEvt.ObjectOld).To(Equal(created))

				// Check second UpdateEvent
				evt = <-c2
				updateEvt, ok = evt.(event.UpdateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.UpdateEvent{}))

				Expect(updateEvt.MetaNew).To(Equal(updated))
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.MetaOld).To(Equal(created))
				Expect(updateEvt.ObjectOld).To(Equal(created))

				By("Deleting a Deployment and expecting the Delete.")
				deleted = updated.DeepCopy()
				err = client.Delete(created.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				deleted.SetResourceVersion("")
				evt = <-c1
				deleteEvt, ok := evt.(event.DeleteEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.DeleteEvent{}))
				deleteEvt.Meta.SetResourceVersion("")
				Expect(deleteEvt.Meta).To(Equal(deleted))
				Expect(deleteEvt.Object).To(Equal(deleted))

				evt = <-c2
				deleteEvt, ok = evt.(event.DeleteEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.DeleteEvent{}))
				deleteEvt.Meta.SetResourceVersion("")
				Expect(deleteEvt.Meta).To(Equal(deleted))
				Expect(deleteEvt.Object).To(Equal(deleted))

				close(done)
			}, 5)
		})

		// TODO(pwittrock): Write this test
		Context("for a Foo CRD resource", func() {
			It("should provide Foo Events", func() {

			})
		})
	})

	Describe("Informer", func() {
		var c chan struct{}
		var rs *appsv1.ReplicaSet
		var depInformer toolscache.SharedIndexInformer
		var informerFactory kubeinformers.SharedInformerFactory
		var stopTest chan struct{}

		BeforeEach(func(done Done) {
			stopTest = make(chan struct{})
			informerFactory = kubeinformers.NewSharedInformerFactory(clientset, time.Second*30)
			depInformer = informerFactory.Apps().V1().ReplicaSets().Informer()
			informerFactory.Start(stopTest)
			Eventually(depInformer.HasSynced).Should(BeTrue())

			c = make(chan struct{})
			rs = &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "informer-rs-name"},
				Spec: appsv1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			}
			close(done)
		})

		AfterEach(func(done Done) {
			close(stopTest)
			close(done)
		})

		Context("for a ReplicaSet resource", func() {
			It("should provide a ReplicaSet CreateEvent", func(done Done) {
				c := make(chan struct{})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err := instance.Start(handler.Funcs{
					CreateFunc: func(evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						var err error
						rs, err = clientset.AppsV1().ReplicaSets("default").Get(rs.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())

						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Meta).To(Equal(rs))
						Expect(evt.Object).To(Equal(rs))
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

				rs, err = clientset.AppsV1().ReplicaSets("default").Create(rs)
				Expect(err).NotTo(HaveOccurred())
				<-c
				close(done)
			}, 30)

			It("should provide a ReplicaSet UpdateEvent", func(done Done) {
				var err error
				rs, err = clientset.AppsV1().ReplicaSets("default").Get(rs.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				rs2 := rs.DeepCopy()
				rs2.SetLabels(map[string]string{"biz": "baz"})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err = instance.Start(handler.Funcs{
					CreateFunc: func(evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
					},
					UpdateFunc: func(evt event.UpdateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						var err error
						rs2, err = clientset.AppsV1().ReplicaSets("default").Get(rs.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())

						Expect(q2).To(Equal(q))
						Expect(evt.MetaOld).To(Equal(rs))
						Expect(evt.ObjectOld).To(Equal(rs))

						Expect(evt.MetaNew).To(Equal(rs2))
						Expect(evt.ObjectNew).To(Equal(rs2))

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

				rs2, err = clientset.AppsV1().ReplicaSets("default").Update(rs2)
				Expect(err).NotTo(HaveOccurred())
				<-c
				close(done)
			})

			It("should provide a ReplicaSet DeletedEvent", func(done Done) {
				c := make(chan struct{})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err := instance.Start(handler.Funcs{
					CreateFunc: func(event.CreateEvent, workqueue.RateLimitingInterface) {
					},
					UpdateFunc: func(event.UpdateEvent, workqueue.RateLimitingInterface) {
					},
					DeleteFunc: func(evt event.DeleteEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.Meta.GetName()).To(Equal(rs.Name))
						close(c)
					},
					GenericFunc: func(event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				err = clientset.AppsV1().ReplicaSets("default").Delete(rs.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				<-c
				close(done)
			})
		})
	})
})
