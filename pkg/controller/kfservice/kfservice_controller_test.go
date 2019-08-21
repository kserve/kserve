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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kubeflow/kfserving/pkg/constants"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/serving/v1beta1"

	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

const timeout = time.Second * 10

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

var instance = &kfserving.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      serviceKey.Name,
		Namespace: serviceKey.Namespace,
	},
	Spec: kfserving.KFServiceSpec{
		Default: kfserving.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &kfserving.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13.0",
			},
		},
	},
}

var canary = &kfserving.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      canaryServiceKey.Name,
		Namespace: canaryServiceKey.Namespace,
	},
	Spec: kfserving.KFServiceSpec{
		Default: kfserving.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &kfserving.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13.0",
			},
		},
		CanaryTrafficPercent: 20,
		Canary: &kfserving.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &kfserving.TensorflowSpec{
				ModelURI:       "s3://test/mnist-2/export",
				RuntimeVersion: "1.13.0",
			},
		},
	},
	Status: kfserving.KFServiceStatus{
		URL: canaryServiceKey.Name + ".svc.cluster.local",
		Default: kfserving.StatusConfigurationSpec{
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
	defaultInstance := instance.DeepCopy()
	g.Expect(c.Create(context.TODO(), defaultInstance)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), defaultInstance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	configuration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), configurationKey, configuration) }, timeout).
		Should(gomega.Succeed())
	expectedConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultConfigurationName(defaultInstance.Name),
			Namespace: defaultInstance.Namespace,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "foo"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/target":                         "1",
						"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/maxScale":                       "3",
						"autoscaling.knative.dev/minScale":                       "1",
						constants.ModelInitializerSourceUriInternalAnnotationKey: defaultInstance.Spec.Default.Tensorflow.ModelURI,
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: kfserving.TensorflowServingImageName + ":" +
										defaultInstance.Spec.Default.Tensorflow.RuntimeVersion,
									Command: []string{kfserving.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + kfserving.TensorflowServingGRPCPort,
										"--rest_api_port=" + kfserving.TensorflowServingRestPort,
										"--model_name=" + defaultInstance.Name,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
									},
								},
							},
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
	updatedRoute.Status.URL = &apis.URL{Scheme: "http", Host: serviceKey.Name + ".svc.cluster.local"}
	updatedRoute.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.RouteConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updatedRoute)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// verify if KFService status is updated
	expectedKfsvcStatus := kfserving.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:   kfserving.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   kfserving.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: updatedRoute.Status.URL.String(),
		Default: kfserving.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	}
	g.Eventually(func() *kfserving.KFServiceStatus {
		kfsvc := &kfserving.KFService{}
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
	canaryInstance := canary.DeepCopy()
	g.Expect(c.Create(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), canaryInstance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultConfigurationKey, defaultConfiguration) }, timeout).
		Should(gomega.Succeed())

	canaryConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryConfigurationKey, canaryConfiguration) }, timeout).
		Should(gomega.Succeed())
	expectedCanaryConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canaryInstance.Name,
			Namespace: canaryInstance.Namespace,
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "bar"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/target":                         "1",
						"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/maxScale":                       "3",
						"autoscaling.knative.dev/minScale":                       "1",
						constants.ModelInitializerSourceUriInternalAnnotationKey: canary.Spec.Canary.Tensorflow.ModelURI,
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: kfserving.TensorflowServingImageName + ":" +
										canary.Spec.Canary.Tensorflow.RuntimeVersion,
									Command: []string{kfserving.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + kfserving.TensorflowServingGRPCPort,
										"--rest_api_port=" + kfserving.TensorflowServingRestPort,
										"--model_name=" + canary.Name,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
									},
								},
							},
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
			Name:      canaryInstance.Name,
			Namespace: canaryInstance.Namespace,
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
	updatedRoute.Status.URL = &apis.URL{Scheme: "http", Host: canaryServiceKey.Name + ".svc.cluster.local"}
	updatedRoute.Status.Traffic = []knservingv1alpha1.TrafficTarget{
		{
			TrafficTarget: v1beta1.TrafficTarget{RevisionName: "revision-v2", Percent: 20},
		},
		{
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
	expectedKfsvcStatus := kfserving.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:     kfserving.CanaryPredictorReady,
					Severity: "Info",
					Status:   "True",
				},
				{
					Type:   kfserving.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   kfserving.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: updatedRoute.Status.URL.String(),
		Default: kfserving.StatusConfigurationSpec{
			Name:    "revision-v1",
			Traffic: 80,
		},
		Canary: kfserving.StatusConfigurationSpec{
			Name:    "revision-v2",
			Traffic: 20,
		},
	}
	g.Eventually(func() string {
		kfsvc := &kfserving.KFService{}
		if err := c.Get(context.TODO(), canaryServiceKey, kfsvc); err != nil {
			return err.Error()
		}
		return cmp.Diff(&expectedKfsvcStatus, &kfsvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
	}, timeout).Should(gomega.BeEmpty())
}

func TestCanaryDelete(t *testing.T) {
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

	// Create the KFService object and expect the Reconcile
	// Default and Canary configuration should be present
	canaryInstance := canary.DeepCopy()
	g.Expect(c.Create(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), canaryInstance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultConfigurationKey, defaultConfiguration) }, timeout).
		Should(gomega.Succeed())

	canaryConfiguration := &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryConfigurationKey, canaryConfiguration) }, timeout).
		Should(gomega.Succeed())

	// Verify if KFService status is updated
	routeUrl := &apis.URL{Scheme: "http", Host: canaryServiceKey.Name + ".svc.cluster.local"}
	expectedKfsvcStatus := kfserving.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:     kfserving.CanaryPredictorReady,
					Severity: "Info",
					Status:   "True",
				},
				{
					Type:   kfserving.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   kfserving.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: routeUrl.String(),
		Default: kfserving.StatusConfigurationSpec{
			Name:    "revision-v1",
			Traffic: 80,
		},
		Canary: kfserving.StatusConfigurationSpec{
			Name:    "revision-v2",
			Traffic: 20,
		},
	}
	g.Eventually(func() string {
		if err := c.Get(context.TODO(), canaryServiceKey, canaryInstance); err != nil {
			return err.Error()
		}
		return cmp.Diff(&expectedKfsvcStatus, &canaryInstance.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
	}, timeout).Should(gomega.BeEmpty())

	// Update instance to remove Canary Spec
	// Canary configuration should be removed during reconcile
	canaryInstance.Spec.Canary = nil
	canaryInstance.Spec.CanaryTrafficPercent = 0
	g.Expect(c.Update(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultConfiguration = &knservingv1alpha1.Configuration{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultConfigurationKey, defaultConfiguration) }, timeout).
		Should(gomega.Succeed())

	canaryConfiguration = &knservingv1alpha1.Configuration{}
	g.Eventually(func() bool {
		err := c.Get(context.TODO(), canaryConfigurationKey, canaryConfiguration)
		return errors.IsNotFound(err)
	}, timeout).Should(gomega.BeTrue())

	// Verify if KFService status is updated with right status
	// Canary status should be removed with condition set to unknown
	route := &knservingv1alpha1.Route{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryServiceKey, route) }, timeout).
		Should(gomega.Succeed())

	updatedRoute := route.DeepCopy()
	updatedRoute.Status.Traffic = []knservingv1alpha1.TrafficTarget{
		{
			TrafficTarget: v1beta1.TrafficTarget{RevisionName: "revision-v1"},
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updatedRoute)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	expectedKfsvcStatus = kfserving.KFServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{
				{
					Type:   kfserving.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:   kfserving.RoutesReady,
					Status: "True",
				},
			},
		},
		URL: routeUrl.String(),
		Default: kfserving.StatusConfigurationSpec{
			Name: "revision-v1",
		},
	}
	g.Eventually(func() *kfserving.KFServiceStatus {
		kfsvc := &kfserving.KFService{}
		err := c.Get(context.TODO(), canaryServiceKey, kfsvc)
		if err != nil {
			return nil
		}
		return &kfsvc.Status
	}, timeout).Should(testutils.BeSematicEqual(&expectedKfsvcStatus))
}
