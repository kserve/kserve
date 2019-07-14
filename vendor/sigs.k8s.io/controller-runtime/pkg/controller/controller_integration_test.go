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

package controller_test

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("controller", func() {
	var reconciled chan reconcile.Request
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		reconciled = make(chan reconcile.Request)
		Expect(cfg).NotTo(BeNil())
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("controller", func() {
		// TODO(directxman12): write a whole suite of controller-client interaction tests

		It("should reconcile", func(done Done) {
			By("Creating the Manager")
			cm, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating the Controller")
			instance, err := controller.New("foo-controller", cm, controller.Options{
				Reconciler: reconcile.Func(
					func(request reconcile.Request) (reconcile.Result, error) {
						reconciled <- request
						return reconcile.Result{}, nil
					}),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Watching Resources")
			err = instance.Watch(&source.Kind{Type: &appsv1.ReplicaSet{}}, &handler.EnqueueRequestForOwner{
				OwnerType: &appsv1.Deployment{},
			})
			Expect(err).NotTo(HaveOccurred())

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
			expectedReconcileRequest := reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "deployment-name",
			}}

			By("Invoking Reconciling for Create")
			deployment, err = clientset.AppsV1().Deployments("default").Create(deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			By("Invoking Reconciling for Update")
			newDeployment := deployment.DeepCopy()
			newDeployment.Labels = map[string]string{"foo": "bar"}
			newDeployment, err = clientset.AppsV1().Deployments("default").Update(newDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			By("Invoking Reconciling for an OwnedObject when it is created")
			replicaset := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rs-name",
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(deployment, schema.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						}),
					},
				},
				Spec: appsv1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: deployment.Spec.Template,
				},
			}
			replicaset, err = clientset.AppsV1().ReplicaSets("default").Create(replicaset)
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			By("Invoking Reconciling for an OwnedObject when it is updated")
			newReplicaset := replicaset.DeepCopy()
			newReplicaset.Labels = map[string]string{"foo": "bar"}
			newReplicaset, err = clientset.AppsV1().ReplicaSets("default").Update(newReplicaset)
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			By("Invoking Reconciling for an OwnedObject when it is deleted")
			err = clientset.AppsV1().ReplicaSets("default").Delete(replicaset.Name, &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			By("Invoking Reconciling for Delete")
			err = clientset.AppsV1().Deployments("default").
				Delete("deployment-name", &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(<-reconciled).To(Equal(expectedReconcileRequest))

			close(done)
		}, 5)
	})
})
