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

package builder

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("application", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		getConfig = func() (*rest.Config, error) { return cfg, nil }
		newController = controller.New
		newManager = manager.New
		getGvk = apiutil.GVKForObject
	})

	AfterEach(func() {
		close(stop)
	})

	noop := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) { return reconcile.Result{}, nil })

	Describe("New", func() {
		It("should return success if given valid objects", func() {
			instance, err := SimpleController().
				ForType(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance).NotTo(BeNil())
		})

		It("should return an error if the Config is invalid", func() {
			getConfig = func() (*rest.Config, error) { return cfg, fmt.Errorf("expected error") }
			instance, err := SimpleController().
				ForType(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if there is no GVK for an object", func() {
			instance, err := SimpleController().
				ForType(&fakeType{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type builder.fakeType"))
			Expect(instance).To(BeNil())

			instance, err = SimpleController().
				ForType(&appsv1.ReplicaSet{}).
				Owns(&fakeType{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type builder.fakeType"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if it cannot create the manager", func() {
			newManager = func(config *rest.Config, options manager.Options) (manager.Manager, error) {
				return nil, fmt.Errorf("expected error")
			}
			instance, err := SimpleController().
				ForType(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if it cannot create the controller", func() {
			newController = func(name string, mgr manager.Manager, options controller.Options) (
				controller.Controller, error) {
				return nil, fmt.Errorf("expected error")
			}
			instance, err := SimpleController().
				ForType(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})
	})

	Describe("Start", func() {
		It("should Reconcile objects", func(done Done) {
			By("Creating the application")
			ch := make(chan reconcile.Request)
			fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				ch <- req
				return reconcile.Result{}, nil
			})

			instance, err := SimpleController().ForType(&appsv1.Deployment{}).
				WithConfig(cfg).
				Owns(&appsv1.ReplicaSet{}).
				Build(fn)
			Expect(err).NotTo(HaveOccurred())

			By("Starting the application")
			go func() {
				defer GinkgoRecover()
				Expect(instance.Start(stop)).NotTo(HaveOccurred())
				By("Stopping the application")
			}()

			By("Creating a Deployment")
			// Expect a Reconcile when the Deployment is managedObjects.
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "deploy-name",
				},
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
			err = instance.GetClient().Create(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the Deployment Reconcile")
			Expect(<-ch).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "default", Name: "deploy-name"}}))

			By("Creating a ReplicaSet")
			// Expect a Reconcile when an Owned object is managedObjects.
			t := true
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "rs-name",
					Labels:    dep.Spec.Selector.MatchLabels,
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "deploy-name",
							Kind:       "Deployment",
							APIVersion: "apps/v1",
							Controller: &t,
							UID:        dep.UID,
						},
					},
				},
				Spec: appsv1.ReplicaSetSpec{
					Selector: dep.Spec.Selector,
					Template: dep.Spec.Template,
				},
			}
			err = instance.GetClient().Create(context.TODO(), rs)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the ReplicaSet Reconcile")
			Expect(<-ch).To(Equal(reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "default", Name: "deploy-name"}}))

			close(done)
		}, 10)
	})
})

var _ runtime.Object = &fakeType{}

type fakeType struct{}

func (*fakeType) GetObjectKind() schema.ObjectKind { return nil }
func (*fakeType) DeepCopyObject() runtime.Object   { return nil }
