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
	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	v1 "k8s.io/api/core/v1"
	"testing"
	"time"

	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
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
var serviceKey = expectedRequest.NamespacedName
var configurationKey = types.NamespacedName{Name: constants.DefaultConfigurationName(serviceKey.Name),
	Namespace: serviceKey.Namespace}

var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "bar", Namespace: "default"}}
var canaryServiceKey = expectedCanaryRequest.NamespacedName
var defaultConfigurationKey = types.NamespacedName{Name: constants.DefaultConfigurationName(canaryServiceKey.Name),
	Namespace: canaryServiceKey.Namespace}
var canaryConfigurationKey = types.NamespacedName{Name: constants.CanaryConfigurationName(canaryServiceKey.Name),
	Namespace: canaryServiceKey.Namespace}

var instance = &servingv1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      serviceKey.Name,
		Namespace: serviceKey.Namespace,
	},
	Spec: servingv1alpha1.KFServiceSpec{
		Default: servingv1alpha1.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13.0",
			},
		},
	},
}

var canary = &servingv1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      canaryServiceKey.Name,
		Namespace: canaryServiceKey.Namespace,
	},
	Spec: servingv1alpha1.KFServiceSpec{
		Default: servingv1alpha1.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13.0",
			},
		},
		CanaryTrafficPercent: 20,
		Canary: &servingv1alpha1.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &servingv1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist-2/export",
				RuntimeVersion: "1.13.0",
			},
		},
	},
	Status: servingv1alpha1.KFServiceStatus{
		URL: canaryServiceKey.Name + ".svc.cluster.local",
		Default: servingv1alpha1.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	},
}

var configs = map[string]string{
	"frameworks": `{
        "tensorflow" : {
            "image" : "tensorflow/serving"
        },
        "sklearn" : {
            "image" : "kfserving/sklearnserver"
        },
        "xgboost" : {
            "image" : "kfserving/xgbserver"
        }
    }`,
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

	// Create configmap
	var configMap = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KFServiceConfigMapName,
			Namespace: constants.KFServingNamespace,
		},
		Data: configs,
	}
	g.Expect(c.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), configMap)

	// Create the KFService object and expect the Reconcile and Knative configuration/routes to be created
	g.Expect(c.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	configuration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), configurationKey, configuration) }, timeout).
		Should(gomega.Succeed())
	expectedConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultConfigurationName(instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "foo"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/target":   "1",
						"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/maxScale": "3",
						"autoscaling.knative.dev/minScale": "1",
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
					},
					Container: &v1.Container{
						Image: servingv1alpha1.TensorflowServingImageName + ":" +
							instance.Spec.Default.Tensorflow.RuntimeVersion,
						Command: []string{servingv1alpha1.TensorflowEntrypointCommand},
						Args: []string{
							"--port=" + servingv1alpha1.TensorflowServingGRPCPort,
							"--rest_api_port=" + servingv1alpha1.TensorflowServingRestPort,
							"--model_name=" + instance.Name,
							"--model_base_path=" + instance.Spec.Default.Tensorflow.ModelURI,
						},
					},
				},
			},
		},
	}
	g.Expect(configuration.Spec).To(gomega.Equal(expectedConfiguration.Spec))

	route := &knservingv1alpha1.Route{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, route) }, timeout).
		Should(gomega.Succeed())
	// mock update knative configuration/route status since knative serving controller is not running in test
	updated := configuration.DeepCopy()
	updated.Status.LatestCreatedRevisionName = "revision-v1"
	updated.Status.LatestReadyRevisionName = "revision-v1"
	updated.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ConfigurationConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updated)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	updatedRoute := route.DeepCopy()
	updatedRoute.Status.Domain = serviceKey.Name + ".svc.cluster.local"
	updatedRoute.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.RouteConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updatedRoute)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// verify if KFService status is updated
	expectedKfsvcStatus := servingv1alpha1.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:   servingv1alpha1.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   servingv1alpha1.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: updatedRoute.Status.Domain,
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
	}, timeout).Should(testutils.BeSematicEqual(&expectedKfsvcStatus))
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

	// Create configmap
	var configMap = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KFServiceConfigMapName,
			Namespace: constants.KFServingNamespace,
		},
		Data: configs,
	}
	g.Expect(c.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), configMap)

	// Create the KFService object and expect the Reconcile and knative service to be created
	g.Expect(c.Create(context.TODO(), canary)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), canary)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultConfigurationKey, defaultConfiguration) }, timeout).
		Should(gomega.Succeed())

	canaryConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryConfigurationKey, canaryConfiguration) }, timeout).
		Should(gomega.Succeed())
	expectedCanaryConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name,
			Namespace: canary.Namespace,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "bar"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/target":   "1",
						"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/maxScale": "3",
						"autoscaling.knative.dev/minScale": "1",
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
					},
					Container: &v1.Container{
						Image: servingv1alpha1.TensorflowServingImageName + ":" +
							canary.Spec.Canary.Tensorflow.RuntimeVersion,
						Command: []string{servingv1alpha1.TensorflowEntrypointCommand},
						Args: []string{
							"--port=" + servingv1alpha1.TensorflowServingGRPCPort,
							"--rest_api_port=" + servingv1alpha1.TensorflowServingRestPort,
							"--model_name=" + canary.Name,
							"--model_base_path=" + canary.Spec.Canary.Tensorflow.ModelURI,
						},
					},
				},
			},
		},
	}
	g.Expect(canaryConfiguration.Spec).To(gomega.Equal(expectedCanaryConfiguration.Spec))
	route := &knservingv1alpha1.Route{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryServiceKey, route) }, timeout).
		Should(gomega.Succeed())
	expectedRoute := knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name,
			Namespace: canary.Namespace,
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: []knservingv1alpha1.TrafficTarget{
				{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: constants.DefaultConfigurationName(canary.Name),
						Percent:           80,
					},
				},
				{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: constants.CanaryConfigurationName(canary.Name),
						Percent:           20,
					},
				},
			},
		},
	}
	g.Expect(route.Spec).To(gomega.Equal(expectedRoute.Spec))

	// mock update knative configuration status since knative serving controller is not running in test
	updateDefault := defaultConfiguration.DeepCopy()
	updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
	updateDefault.Status.LatestReadyRevisionName = "revision-v1"
	updateDefault.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ConfigurationConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	updateCanary := canaryConfiguration.DeepCopy()
	updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
	updateCanary.Status.LatestReadyRevisionName = "revision-v2"
	updateCanary.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ConfigurationConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	updatedRoute := route.DeepCopy()
	updatedRoute.Status.Domain = canaryServiceKey.Name + ".svc.cluster.local"
	updatedRoute.Status.Traffic = []knservingv1alpha1.TrafficTarget{
		{
			Name:          "candidate",
			TrafficTarget: v1beta1.TrafficTarget{RevisionName: "revision-v2", Percent: 20},
		},
		{
			Name:          "current",
			TrafficTarget: v1beta1.TrafficTarget{RevisionName: "revision-v1", Percent: 80},
		},
	}
	updatedRoute.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.RouteConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updatedRoute)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	// verify if KFService status is updated
	expectedKfsvcStatus := servingv1alpha1.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:     servingv1alpha1.CanaryPredictorReady,
					Severity: "Info",
					Status:   "True",
				},
				{
					Type:   servingv1alpha1.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   servingv1alpha1.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: updatedRoute.Status.Domain,
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
		err := c.Get(context.TODO(), canaryServiceKey, kfsvc)
		if err != nil {
			return nil
		}
		return &kfsvc.Status
	}, timeout).Should(testutils.BeSematicEqual(&expectedKfsvcStatus))
}
