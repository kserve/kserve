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
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	istiov1alpha1 "knative.dev/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

const timeout = time.Second * 10

var configs = map[string]string{
	"predictors": `{
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
	"ingress": `{
        "ingressGateway" : "test-gateway",
        "ingressService" : "test-destination"
    }`,
}

func TestKFServiceWithOnlyPredictor(t *testing.T) {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
	var serviceKey = expectedRequest.NamespacedName
	var predictorService = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
		Namespace: serviceKey.Namespace}
	var virtualServiceName = types.NamespacedName{Name: constants.VirtualServiceName(serviceKey.Name),
		Namespace: serviceKey.Namespace}

	var instance = &kfserving.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceKey.Name,
			Namespace: serviceKey.Namespace,
		},
		Spec: kfserving.KFServiceSpec{
			Default: kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
		},
	}
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

	// Create the KFService object and expect the Reconcile and Knative service/routes to be created
	defaultInstance := instance.DeepCopy()
	g.Expect(c.Create(context.TODO(), defaultInstance)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), defaultInstance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	service := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), predictorService, service) }, timeout).
		Should(gomega.Succeed())
	expectedService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultPredictorServiceName(defaultInstance.Name),
			Namespace: defaultInstance.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "foo"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/target":                           "1",
							"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/maxScale":                         "3",
							"autoscaling.knative.dev/minScale":                         "1",
							constants.StorageInitializerSourceUriInternalAnnotationKey: defaultInstance.Spec.Default.Predictor.Tensorflow.StorageURI,
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: kfserving.TensorflowServingImageName + ":" +
											defaultInstance.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
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
		},
	}
	g.Expect(service.Spec).To(gomega.Equal(expectedService.Spec))

	// mock update knative service status since knative serving controller is not running in test
	updateDefault := service.DeepCopy()
	updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
	updateDefault.Status.LatestReadyRevisionName = "revision-v1"
	updateDefault.Status.URL, _ = apis.ParseURL("http://revision-v1.myns.myingress.com")
	updateDefault.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ServiceConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	virtualService := &istiov1alpha3.VirtualService{}
	g.Eventually(func() error { return c.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
		Should(gomega.Succeed())

	expectedVirtualService := &istiov1alpha3.VirtualService{
		Spec: istiov1alpha3.VirtualServiceSpec{
			Gateways: []string{
				"test-gateway",
			},
			Hosts: []string{
				"foo.myns.myingress.com",
			},
			HTTP: []istiov1alpha3.HTTPRoute{
				istiov1alpha3.HTTPRoute{
					Match: []istiov1alpha3.HTTPMatchRequest{
						istiov1alpha3.HTTPMatchRequest{
							URI: &istiov1alpha1.StringMatch{
								Prefix: "/v1/models/foo:predict",
							},
						},
					},
					Route: []istiov1alpha3.HTTPRouteDestination{
						istiov1alpha3.HTTPRouteDestination{
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.HeaderOperations{
									Set: map[string]string{
										"Host": "revision-v1.myns.myingress.com",
									},
								},
							},
							Destination: istiov1alpha3.Destination{
								Host: "test-destination",
							},
							Weight: 100,
						},
					},
				},
			},
		},
	}
	g.Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))

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
		URL:           "http://foo.myns.myingress.com/v1/models/foo",
		Traffic:       100,
		CanaryTraffic: 0,
		Default: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name:     "revision-v1",
				Hostname: "revision-v1.myns.myingress.com",
			},
		},
		Canary: &kfserving.EndpointStatusMap{},
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

func TestKFServiceWithDefaultAndCanaryPredictor(t *testing.T) {
	var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "bar", Namespace: "default"}}
	var canaryServiceKey = expectedCanaryRequest.NamespacedName

	var canary = &kfserving.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canaryServiceKey.Name,
			Namespace: canaryServiceKey.Namespace,
		},
		Spec: kfserving.KFServiceSpec{
			Default: kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
			CanaryTrafficPercent: 20,
			Canary: &kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist-2/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
		},
		Status: kfserving.KFServiceStatus{
			URL: canaryServiceKey.Name + ".svc.cluster.local",
			Default: &kfserving.EndpointStatusMap{
				constants.Predictor: &kfserving.StatusConfigurationSpec{
					Name: "revision-v1",
				},
			},
		},
	}
	var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(canaryServiceKey.Name),
		Namespace: canaryServiceKey.Namespace}
	var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(canaryServiceKey.Name),
		Namespace: canaryServiceKey.Namespace}
	var virtualServiceName = types.NamespacedName{Name: constants.VirtualServiceName(canaryServiceKey.Name),
		Namespace: canaryServiceKey.Namespace}
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

	defaultService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
		Should(gomega.Succeed())

	canaryService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryPredictor, canaryService) }, timeout).
		Should(gomega.Succeed())
	expectedCanaryService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryPredictorServiceName(canaryInstance.Name),
			Namespace: canaryInstance.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "bar"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/target":                           "1",
							"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/maxScale":                         "3",
							"autoscaling.knative.dev/minScale":                         "1",
							constants.StorageInitializerSourceUriInternalAnnotationKey: canary.Spec.Canary.Predictor.Tensorflow.StorageURI,
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: kfserving.TensorflowServingImageName + ":" +
											canary.Spec.Canary.Predictor.Tensorflow.RuntimeVersion,
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
		},
	}
	g.Expect(cmp.Diff(canaryService.Spec, expectedCanaryService.Spec)).To(gomega.Equal(""))
	g.Expect(canaryService.Name).To(gomega.Equal(expectedCanaryService.Name))

	// mock update knative service status since knative serving controller is not running in test
	updateDefault := defaultService.DeepCopy()
	updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
	updateDefault.Status.LatestReadyRevisionName = "revision-v1"
	updateDefault.Status.URL, _ = apis.ParseURL("http://revision-v1.myns.myingress.com")
	updateDefault.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ServiceConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	updateCanary := canaryService.DeepCopy()
	updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
	updateCanary.Status.LatestReadyRevisionName = "revision-v2"
	updateCanary.Status.URL, _ = apis.ParseURL("http://revision-v2.myns.myingress.com")
	updateCanary.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ServiceConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	virtualService := &istiov1alpha3.VirtualService{}
	g.Eventually(func() error { return c.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
		Should(gomega.Succeed())

	expectedVirtualService := &istiov1alpha3.VirtualService{
		Spec: istiov1alpha3.VirtualServiceSpec{
			Gateways: []string{
				"test-gateway",
			},
			Hosts: []string{
				"bar.myns.myingress.com",
			},
			HTTP: []istiov1alpha3.HTTPRoute{
				istiov1alpha3.HTTPRoute{
					Match: []istiov1alpha3.HTTPMatchRequest{
						istiov1alpha3.HTTPMatchRequest{
							URI: &istiov1alpha1.StringMatch{
								Prefix: "/v1/models/bar:predict",
							},
						},
					},
					Route: []istiov1alpha3.HTTPRouteDestination{
						istiov1alpha3.HTTPRouteDestination{
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.HeaderOperations{
									Set: map[string]string{
										"Host": "revision-v1.myns.myingress.com",
									},
								},
							},
							Destination: istiov1alpha3.Destination{
								Host: "test-destination",
							},
							Weight: 80,
						},
						istiov1alpha3.HTTPRouteDestination{
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.HeaderOperations{
									Set: map[string]string{
										"Host": "revision-v2.myns.myingress.com",
									},
								},
							},
							Destination: istiov1alpha3.Destination{
								Host: "test-destination",
							},
							Weight: 20,
						},
					},
				},
			},
		},
	}
	g.Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))

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
		URL:           "http://bar.myns.myingress.com/v1/models/bar",
		Traffic:       80,
		CanaryTraffic: 20,
		Default: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name:     "revision-v1",
				Hostname: "revision-v1.myns.myingress.com",
			},
		},
		Canary: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name:     "revision-v2",
				Hostname: "revision-v2.myns.myingress.com",
			},
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
	serviceName := fmt.Sprintf("canary-delete-%v", time.Now().UnixNano())
	serviceURL := "http://"+serviceName+".myns.myingress.com/v1/models/"+serviceName
	namespace := "default"
	var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
		Namespace: namespace}
	var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(serviceName),
		Namespace: namespace}
	var virtualServiceName = types.NamespacedName{Name: constants.VirtualServiceName(serviceName),
		Namespace: namespace}
	var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
	var canaryServiceKey = expectedCanaryRequest.NamespacedName

	var canary = &kfserving.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canaryServiceKey.Name,
			Namespace: canaryServiceKey.Namespace,
		},
		Spec: kfserving.KFServiceSpec{
			Default: kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
			CanaryTrafficPercent: 20,
			Canary: &kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist-2/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
		},
		Status: kfserving.KFServiceStatus{
			URL: canaryServiceKey.Name + ".svc.cluster.local",
			Default: &kfserving.EndpointStatusMap{
				constants.Predictor: &kfserving.StatusConfigurationSpec{
					Name: "revision-v1",
				},
			},
		},
	}
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
	// Default and Canary service should be present
	canaryInstance := canary.DeepCopy()
	canaryInstance.Name = serviceName
	g.Expect(c.Create(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), canaryInstance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
		Should(gomega.Succeed())

	canaryService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryPredictor, canaryService) }, timeout).
		Should(gomega.Succeed())

	// mock update knative service status since knative serving controller is not running in test
	updateDefault := defaultService.DeepCopy()
	updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
	updateDefault.Status.LatestReadyRevisionName = "revision-v1"
	updateDefault.Status.URL, _ = apis.ParseURL("http://revision-v1.myns.myingress.com")
	updateDefault.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ServiceConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	updateCanary := canaryService.DeepCopy()
	updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
	updateCanary.Status.LatestReadyRevisionName = "revision-v2"
	updateCanary.Status.URL, _ = apis.ParseURL("http://revision-v2.myns.myingress.com")
	updateCanary.Status.Conditions = duckv1beta1.Conditions{
		{
			Type:   knservingv1alpha1.ServiceConditionReady,
			Status: "True",
		},
	}
	g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	virtualService := &istiov1alpha3.VirtualService{}
	g.Eventually(func() error { return c.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
		Should(gomega.Succeed())

	// Verify if KFService status is updated
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
		URL:           serviceURL,
		Traffic:       80,
		CanaryTraffic: 20,
		Default: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name:     "revision-v1",
				Hostname: "revision-v1.myns.myingress.com",
			},
		},
		Canary: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name:     "revision-v2",
				Hostname: "revision-v2.myns.myingress.com",
			},
		},
	}

	canaryUpdate := &kfserving.KFService{}
	g.Eventually(func() string {
		if err := c.Get(context.TODO(), canaryServiceKey, canaryUpdate); err != nil {
			return err.Error()
		}
		return cmp.Diff(&expectedKfsvcStatus, &canaryUpdate.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
	}, timeout).Should(gomega.BeEmpty())

	// Update instance to remove Canary Spec
	// Canary service should be removed during reconcile
	canaryUpdate.Spec.Canary = nil
	canaryUpdate.Spec.CanaryTrafficPercent = 0
	g.Expect(c.Update(context.TODO(), canaryUpdate)).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedCanaryRequest)))

	defaultService = &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
		Should(gomega.Succeed())

	canaryService = &knservingv1alpha1.Service{}
	g.Eventually(func() bool {
		err := c.Get(context.TODO(), canaryPredictor, canaryService)
		return errors.IsNotFound(err)
	}, timeout).Should(gomega.BeTrue())

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
		URL: serviceURL,
		Traffic: 100,
		Default: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name: "revision-v1",
				Hostname: "revision-v1.myns.myingress.com",
			},
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

func TestKFServiceWithTransformer(t *testing.T) {
	serviceName := "transformer"
	namespace := "default"
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
	var serviceKey = expectedRequest.NamespacedName

	var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
		Namespace: namespace}
	var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(serviceName),
		Namespace: namespace}
	var defaultTransformer = types.NamespacedName{Name: constants.DefaultTransformerServiceName(serviceName),
		Namespace: namespace}
	var canaryTransformer = types.NamespacedName{Name: constants.CanaryTransformerServiceName(serviceName),
		Namespace: namespace}
	var routeName = types.NamespacedName{Name: constants.PredictRouteName(serviceName),
		Namespace: namespace}
	var transformer = &kfserving.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: kfserving.KFServiceSpec{
			Default: kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
				Transformer: &kfserving.TransformerSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Custom: &kfserving.CustomSpec{
						Container: v1.Container{
							Image: "transformer:v1",
						},
					},
				},
			},
			CanaryTrafficPercent: 20,
			Canary: &kfserving.EndpointSpec{
				Predictor: kfserving.PredictorSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &kfserving.TensorflowSpec{
						StorageURI:     "s3://test/mnist-2/export",
						RuntimeVersion: "1.13.0",
					},
				},
				Transformer: &kfserving.TransformerSpec{
					DeploymentSpec: kfserving.DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Custom: &kfserving.CustomSpec{
						Container: v1.Container{
							Image: "transformer:v2",
						},
					},
				},
			},
		},
		Status: kfserving.KFServiceStatus{
			URL: serviceName + ".svc.cluster.local",
			Default: &kfserving.EndpointStatusMap{
				constants.Predictor: &kfserving.StatusConfigurationSpec{
					Name: "revision-v1",
				},
			},
		},
	}

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
	instance := transformer.DeepCopy()
	g.Expect(c.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	defaultPredictorService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultPredictor, defaultPredictorService) }, timeout).
		Should(gomega.Succeed())

	canaryPredictorService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryPredictor, canaryPredictorService) }, timeout).
		Should(gomega.Succeed())

	defaultTransformerService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), defaultTransformer, defaultTransformerService) }, timeout).
		Should(gomega.Succeed())

	canaryTransformerService := &knservingv1alpha1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), canaryTransformer, canaryTransformerService) }, timeout).
		Should(gomega.Succeed())
	expectedCanaryService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryTransformerServiceName(instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": serviceName},
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "transformer:v2",
										Args: []string{
											"--model_name",
											serviceName,
											"--predictor_host",
											constants.CanaryPredictorServiceName(instance.Name) + "." + instance.Namespace,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	g.Expect(cmp.Diff(canaryTransformerService.Spec, expectedCanaryService.Spec)).To(gomega.Equal(""))
	route := &knservingv1alpha1.Route{}
	g.Eventually(func() error { return c.Get(context.TODO(), routeName, route) }, timeout).
		Should(gomega.Succeed())
	expectedRoute := knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PredictRouteName(instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: []knservingv1alpha1.TrafficTarget{
				{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: constants.DefaultTransformerServiceName(instance.Name),
						Percent:           80,
					},
				},
				{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: constants.CanaryTransformerServiceName(instance.Name),
						Percent:           20,
					},
				},
			},
		},
	}
	g.Expect(route.Spec).To(gomega.Equal(expectedRoute.Spec))

	// mock update knative service status since knative serving controller is not running in test

	// update predictor
	{
		updateDefault := defaultPredictorService.DeepCopy()
		updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
		updateDefault.Status.LatestReadyRevisionName = "revision-v1"
		updateDefault.Status.Conditions = duckv1beta1.Conditions{
			{
				Type:   knservingv1alpha1.ServiceConditionReady,
				Status: "True",
			},
		}
		g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

		updateCanary := canaryPredictorService.DeepCopy()
		updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
		updateCanary.Status.LatestReadyRevisionName = "revision-v2"
		updateCanary.Status.Conditions = duckv1beta1.Conditions{
			{
				Type:   knservingv1alpha1.ServiceConditionReady,
				Status: "True",
			},
		}
		g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	}

	// update transformer
	{
		updateDefault := defaultTransformerService.DeepCopy()
		updateDefault.Status.LatestCreatedRevisionName = "t-revision-v1"
		updateDefault.Status.LatestReadyRevisionName = "t-revision-v1"
		updateDefault.Status.Conditions = duckv1beta1.Conditions{
			{
				Type:   knservingv1alpha1.ServiceConditionReady,
				Status: "True",
			},
		}
		g.Expect(c.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

		updateCanary := canaryTransformerService.DeepCopy()
		updateCanary.Status.LatestCreatedRevisionName = "t-revision-v2"
		updateCanary.Status.LatestReadyRevisionName = "t-revision-v2"
		updateCanary.Status.Conditions = duckv1beta1.Conditions{
			{
				Type:   knservingv1alpha1.ServiceConditionReady,
				Status: "True",
			},
		}
		g.Expect(c.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	}

	// update route
	updatedRoute := route.DeepCopy()
	updatedRoute.Status.URL = &apis.URL{Scheme: "http", Host: serviceName + ".svc.cluster.local"}
	updatedRoute.Status.Traffic = []knservingv1alpha1.TrafficTarget{
		{
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "t-revision-v2",
				Percent:      20,
			},
		},
		{
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "t-revision-v1",
				Percent:      80,
			},
		},
	}
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
					Type:     kfserving.CanaryPredictorReady,
					Severity: "Info",
					Status:   "True",
				},
				{
					Type:     kfserving.CanaryTransformerReady,
					Severity: "Info",
					Status:   "True",
				},
				{
					Type:   kfserving.DefaultPredictorReady,
					Status: "True",
				},
				{
					Type:     kfserving.DefaultTransformerReady,
					Severity: "Info",
					Status:   "True",
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
		Default: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name: "revision-v1",
			},
			constants.Transformer: &kfserving.StatusConfigurationSpec{
				Name: "t-revision-v1",
				// TODO rakelkar Hostname?
			},
		},
		Canary: &kfserving.EndpointStatusMap{
			constants.Predictor: &kfserving.StatusConfigurationSpec{
				Name: "revision-v2",
			},
			constants.Transformer: &kfserving.StatusConfigurationSpec{
				Name: "t-revision-v2",
				// TODO rakelkar Hostname?
			},
		},
	}
	g.Eventually(func() string {
		kfsvc := &kfserving.KFService{}
		if err := c.Get(context.TODO(), serviceKey, kfsvc); err != nil {
			return err.Error()
		}
		return cmp.Diff(&expectedKfsvcStatus, &kfsvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
	}, timeout).Should(gomega.BeEmpty())
}
