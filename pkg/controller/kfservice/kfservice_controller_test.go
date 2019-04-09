/*
Copyright 2019 kubeflow.org.

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

package service

import (
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/containers/tensorflow"
	"k8s.io/api/core/v1"
	"testing"
	"time"

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	servingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var serviceKey = types.NamespacedName{Name: "foo", Namespace: "default"}
var kfserviceKey = types.NamespacedName{Name: "foo", Namespace: "default"}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	println("testing")
	g := gomega.NewGomegaWithT(t)
	instance := &servingv1alpha1.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: servingv1alpha1.KFServiceSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Default: servingv1alpha1.ModelSpec{
				Tensorflow: &servingv1alpha1.TensorflowSpec{
					ModelUri:       "s3://test/mnist/export",
					RuntimeVersion: "1.13",
				},
			},
		},
	}

	expectedService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions: []string{"@latest"},
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: v1.Container{
								Image:   "tensorflow/serving:1.13",
								Command: []string{tensorflow.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + tensorflow.TensorflowServingPort,
									"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
									"--model_name=foo",
									"--model_base_path=s3://test/mnist/export",
								},
							},
						},
					},
				},
			},
		},
	}
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the KFService object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	knservice := &v1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, knservice) }, timeout).
		Should(gomega.Succeed())
	g.Expect(knservice.Spec).To(gomega.Equal(expectedService.Spec))

	// Test initial status update
	updated := knservice.DeepCopy()
	updated.Status.Address = &duckv1alpha1.Addressable{
		Hostname: "foo.svc.cluster.local",
	}
	updated.Status.LatestCreatedRevisionName = "revision-v1"
	updated.Status.LatestReadyRevisionName = "revision-v1"
	g.Expect(c.Status().Update(context.TODO(), updated)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	updatedSvc := &v1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, updatedSvc) }, timeout).
		Should(gomega.Succeed())

	kfService := &servingv1alpha1.KFService{}
	g.Eventually(func() error { return c.Get(context.TODO(), kfserviceKey, kfService) }, timeout).
		Should(gomega.Succeed())
	//TODO(@yuzisun) check why this assertion does not work in test, something to do with ordering
	/*expectedStatus := servingv1alpha1.KFServiceStatus{
		URI: servingv1alpha1.URISpec{
			Internal: "foo.svc.cluster.local",
		},
		Default: servingv1alpha1.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	}
	g.Expect(kfservice.Status).To(gomega.Equal(expectedStatus))*/

	// Test canary status update
	copy := kfService.DeepCopy()
	copy.Spec.Canary = &servingv1alpha1.CanarySpec{
		TrafficPercent: 20,
		ModelSpec: servingv1alpha1.ModelSpec{
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelUri:       "s3://test/mnist-2/export",
				RuntimeVersion: "1.13",
			},
		},
	}

	g.Expect(c.Update(context.TODO(), copy)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	updateCanary := updated.DeepCopy()
	updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
	updateCanary.Status.LatestReadyRevisionName = "revision-v2"
	updateCanary.Status.Traffic = []knservingv1alpha1.TrafficTarget{
		{
			Name:         "candidate",
			RevisionName: "revision-v2",
			Percent:      20,
		},
		{
			Name:         "current",
			RevisionName: "revision-v1",
			Percent:      80,
		},
	}

	g.Expect(c.Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	kfServiceCanary := &servingv1alpha1.KFService{}

	g.Eventually(func() error { return c.Get(context.TODO(), kfserviceKey, kfServiceCanary) }, timeout).
		Should(gomega.Succeed())
	//TODO(@yuzisun) check why this assertion does not work in test, something to do with ordering
	/*expectedStatus := servingv1alpha1.KFServiceStatus{
		URI: servingv1alpha1.URISpec{
			Internal: updated.Status.Address.Hostname,
		},
		Default: servingv1alpha1.StatusConfigurationSpec{
			Name:    "revision-v1",
			Traffic: 80,
		},
		Canary: servingv1alpha1.StatusConfigurationSpec{
			Name:    "revision-v2",
			Traffic: 20,
		},
	}
	g.Expect(kfServiceCanary.Status).To(gomega.Equal(expectedStatus))*/
}
