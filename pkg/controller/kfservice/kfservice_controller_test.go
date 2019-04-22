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
	"github.com/kubeflow/kfserving/pkg/frameworks/tensorflow"
	"k8s.io/api/core/v1"
	"testing"
	"time"

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	servingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

const timeout = time.Second * 5

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var serviceKey = types.NamespacedName{Name: "foo", Namespace: "default"}

var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-canary", Namespace: "default"}}
var canaryKey = types.NamespacedName{Name: "foo-canary", Namespace: "default"}

var instance = &servingv1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	},
	Spec: servingv1alpha1.KFServiceSpec{
		MinReplicas: 1,
		MaxReplicas: 3,
		Default: servingv1alpha1.ModelSpec{
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13",
			},
		},
	},
}

var canary = &servingv1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo-canary",
		Namespace: "default",
	},
	Spec: servingv1alpha1.KFServiceSpec{
		MinReplicas: 1,
		MaxReplicas: 3,
		Default: servingv1alpha1.ModelSpec{
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13",
			},
		},
		Canary: &servingv1alpha1.CanarySpec{
			TrafficPercent: 20,
			ModelSpec: servingv1alpha1.ModelSpec{
				Tensorflow: &servingv1alpha1.TensorflowSpec{
					ModelURI:       "s3://test/mnist-2/export",
					RuntimeVersion: "1.13",
				},
			},
		},
	},
	Status: servingv1alpha1.KFServiceStatus{
		URI: servingv1alpha1.URISpec{
			Internal: "foo-canary.svc.cluster.local",
		},
		Default: servingv1alpha1.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	},
}

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
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

	// Create the KFService object and expect the Reconcile and Knative service to be created
	g.Expect(c.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	ksvc := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, ksvc) }, timeout).
		Should(gomega.Succeed())
	expectedService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions: []string{"@latest"},
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: v1.Container{
								Image: tensorflow.TensorflowServingImageName + ":" +
									instance.Spec.Default.Tensorflow.RuntimeVersion,
								Command: []string{tensorflow.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + tensorflow.TensorflowServingGRPCPort,
									"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
									"--model_name=" + instance.Name,
									"--model_base_path=" + instance.Spec.Default.Tensorflow.ModelURI,
								},
							},
						},
					},
				},
			},
		},
	}
	g.Expect(ksvc.Spec).To(gomega.Equal(expectedService.Spec))

	// mock update knative service status since knative serving controller is not running in test
	updated := ksvc.DeepCopy()
	updated.Status.Address = &duckv1alpha1.Addressable{
		Hostname: "foo.svc.cluster.local",
	}
	updated.Status.LatestCreatedRevisionName = "revision-v1"
	updated.Status.LatestReadyRevisionName = "revision-v1"
	g.Expect(c.Status().Update(context.TODO(), updated)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// verify if kf serving service status is updated
	expectedKfsvcStatus := servingv1alpha1.KFServiceStatus{
		URI: servingv1alpha1.URISpec{
			Internal: updated.Status.Address.Hostname,
		},
		Default: servingv1alpha1.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	}
	g.Eventually(func() *servingv1alpha1.KFServiceStatus {
		kfsvc := &servingv1alpha1.KFService{}
		err := c.Get(context.TODO(), serviceKey, kfsvc)
		if err != nil {
			return nil
		}
		return &kfsvc.Status
	}, timeout).Should(gomega.Equal(&expectedKfsvcStatus))
}

func TestCanaryReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

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

	// Create the KFService object and expect the Reconcile and knative service to be created
	g.Expect(c.Create(context.TODO(), canary)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), canary)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	ksvc := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryKey, ksvc) }, timeout).
		Should(gomega.Succeed())
	expectedCanaryService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name,
			Namespace: canary.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions:      []string{canary.Status.Default.Name, "@latest"},
				RolloutPercent: 20,
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: v1.Container{
								Image: tensorflow.TensorflowServingImageName + ":" +
									canary.Spec.Canary.Tensorflow.RuntimeVersion,
								Command: []string{tensorflow.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + tensorflow.TensorflowServingGRPCPort,
									"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
									"--model_name=" + canary.Name,
									"--model_base_path=" + canary.Spec.Canary.Tensorflow.ModelURI,
								},
							},
						},
					},
				},
			},
		},
	}
	g.Expect(ksvc.Spec).To(gomega.Equal(expectedCanaryService.Spec))

	// mock update knative service status since knative serving controller is not running in test
	updateCanary := ksvc.DeepCopy()
	updateCanary.Status.Address = &duckv1alpha1.Addressable{
		Hostname: "foo-canary.svc.cluster.local",
	}
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
	g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	// verify if kf serving service status is updated
	expectedKfsvcStatus := servingv1alpha1.KFServiceStatus{
		URI: servingv1alpha1.URISpec{
			Internal: updateCanary.Status.Address.Hostname,
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
	g.Eventually(func() *servingv1alpha1.KFServiceStatus {
		kfsvc := &servingv1alpha1.KFService{}
		err := c.Get(context.TODO(), canaryKey, kfsvc)
		if err != nil {
			return nil
		}
		return &kfsvc.Status
	}, timeout).Should(gomega.Equal(&expectedKfsvcStatus))
}
