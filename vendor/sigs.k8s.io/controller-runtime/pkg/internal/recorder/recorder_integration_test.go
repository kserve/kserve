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

package recorder_test

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("recorder", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		Expect(cfg).NotTo(BeNil())
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("recorder", func() {
		It("should publish events", func(done Done) {
			By("Creating the Manager")
			cm, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating the Controller")
			recorder := cm.GetRecorder("test-recorder")
			instance, err := controller.New("foo-controller", cm, controller.Options{
				Reconciler: reconcile.Func(
					func(request reconcile.Request) (reconcile.Result, error) {
						dp, err := clientset.AppsV1().Deployments(request.Namespace).Get(request.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())
						recorder.Event(dp, corev1.EventTypeNormal, "test-reason", "test-msg")
						return reconcile.Result{}, nil
					}),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Watching Resources")
			err = instance.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{})
			Expect(err).NotTo(HaveOccurred())

			By("Starting the Manager")
			go func() {
				defer GinkgoRecover()
				Expect(cm.Start(stop)).NotTo(HaveOccurred())
			}()

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

			By("Invoking Reconciling")
			deployment, err = clientset.AppsV1().Deployments("default").Create(deployment)
			Expect(err).NotTo(HaveOccurred())

			By("Validate event is published as expected")
			evtWatcher, err := clientset.CoreV1().Events("default").Watch(metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			resultEvent := <-evtWatcher.ResultChan()

			Expect(resultEvent.Type).To(Equal(watch.Added))
			evt, isEvent := resultEvent.Object.(*corev1.Event)
			Expect(isEvent).To(BeTrue())

			dpRef, err := ref.GetReference(scheme.Scheme, deployment)
			Expect(err).NotTo(HaveOccurred())

			Expect(evt.InvolvedObject).To(Equal(*dpRef))
			Expect(evt.Type).To(Equal(corev1.EventTypeNormal))
			Expect(evt.Reason).To(Equal("test-reason"))
			Expect(evt.Message).To(Equal("test-msg"))

			close(done)
		})
	})
})
