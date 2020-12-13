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
	"fmt"
	"k8s.io/client-go/util/retry"
	"reflect"
	"sort"
	"time"

	"knative.dev/pkg/network"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kubeflow/kfserving/pkg/constants"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	g "github.com/onsi/gomega"
	"golang.org/x/net/context"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	timeout                    = time.Second * 30
	TensorflowServingImageName = "tensorflow/serving"
	domain                     = "example.com"
)

var (
	containerConcurrency int64 = 0
)

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
	"explainers": `{
        "alibi": {
            "image" : "kfserving/alibi-explainer",
			"defaultImageVersion": "latest"
        }
	}`,
	"ingress": `{
        "ingressGateway" : "knative-serving/knative-ingress-gateway",
        "ingressService" : "test-destination"
    }`,
}

type SimpleEvent struct {
	Reason string
	Count  int32
	Type   string
}

type SimpleEventWithTime struct {
	event         SimpleEvent
	LastTimestamp metav1.Time
}

func getEvents() []SimpleEvent {
	events := &v1.EventList{}
	if err := k8sClient.List(context.TODO(), events); err != nil {
		return nil
	}
	numEvents := len(events.Items)
	if numEvents == 0 {
		return nil
	}
	sortedEvents := make([]SimpleEventWithTime, 0, numEvents)
	for _, event := range events.Items {
		if event.Reason != "Updated" && event.Reason != "InternalError" && event.Reason != "UpdateFailed" { // Not checking for updates or errors
			sortedEvents = append(sortedEvents, SimpleEventWithTime{
				event: SimpleEvent{
					Reason: event.Reason,
					Count:  event.Count,
					Type:   event.Type,
				},
				LastTimestamp: event.LastTimestamp,
			})
		}
	}
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].LastTimestamp.Before(&sortedEvents[j].LastTimestamp)
	})
	simpleEvents := make([]SimpleEvent, 0, len(sortedEvents))
	for _, sEvent := range sortedEvents {
		simpleEvents = append(simpleEvents, sEvent.event)
	}
	return simpleEvents
}

var _ = Describe("test inference service controller", func() {

	Context("InferenceService with Predictor", func() {
		It("Should create successfully", func() {
			serviceName := "foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorService = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			var virtualServiceName = types.NamespacedName{Name: serviceKey.Name, Namespace: serviceKey.Namespace}

			var instance = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
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

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and Knative service/routes to be created
			defaultInstance := instance.DeepCopy()
			g.Expect(k8sClient.Create(context.TODO(), defaultInstance)).NotTo(gomega.HaveOccurred())

			defer k8sClient.Delete(context.TODO(), defaultInstance)

			service := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), predictorService, service) }, timeout).
				Should(gomega.Succeed())
			expectedService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName(defaultInstance.Name),
					Namespace: defaultInstance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": serviceName,
									constants.KServiceEndpointLabel:  constants.InferenceServiceDefault,
									constants.KServiceModelLabel:     defaultInstance.Name,
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/target":                           "1",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									constants.StorageInitializerSourceUriInternalAnnotationKey: defaultInstance.Spec.Default.Predictor.Tensorflow.StorageURI,
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultPredictorTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: TensorflowServingImageName + ":" +
												defaultInstance.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{kfserving.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + kfserving.TensorflowServingGRPCPort,
												"--rest_api_port=" + kfserving.TensorflowServingRestPort,
												"--model_name=" + defaultInstance.Name,
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
											},
											LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/" + defaultInstance.Name,
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
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
			updateDefault.Status.URL, _ = apis.ParseURL(
				"http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			updateDefault.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

			virtualService := &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())

			expectedVirtualService := &v1alpha3.VirtualService{
				Spec: istiov1alpha3.VirtualService{
					Gateways: []string{
						constants.KnativeIngressGateway,
						constants.KnativeLocalGateway,
					},
					Hosts: []string{
						constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
						network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
					},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: []*istiov1alpha3.HTTPMatchRequest{
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: network.GetServiceHostname("cluster-local-gateway", "istio-system"),
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 100,
								},
							},
							Retries: &istiov1alpha3.HTTPRetry{
								Attempts:      0,
								PerTryTimeout: nil,
							},
						},
					},
				},
			}
			g.Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))

			// verify if InferenceService status is updated
			expectedKfsvcStatus := kfserving.InferenceServiceStatus{
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
				URL: "http://" + constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(serviceKey.Name, constants.ProtocolV1),
						Host:   network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
					},
				},
				Traffic:       100,
				CanaryTraffic: 0,
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{},
			}
			g.Eventually(func() *kfserving.InferenceServiceStatus {
				isvc := &kfserving.InferenceService{}
				err := k8sClient.Get(context.TODO(), serviceKey, isvc)
				if err != nil {
					return nil
				}
				return &isvc.Status
			}, timeout).Should(testutils.BeSematicEqual(&expectedKfsvcStatus))
			// We are testing for a Ready event
			expectedReadyEvents := []SimpleEvent{
				{Count: 1, Type: v1.EventTypeNormal, Reason: string(kfserving.InferenceServiceReadyState)},
			}
			g.Eventually(func() error {
				events := getEvents()
				if reflect.DeepEqual(events, expectedReadyEvents) {
					return nil
				}
				return fmt.Errorf("test %q failed: [%v] did not equal [%v]", serviceName, events, expectedReadyEvents)
			}, timeout).Should(gomega.Succeed())
		})
	})

	Context("Inference Service with Canary", func() {
		It("Should create successfully", func() {
			var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "bar", Namespace: "default"}}
			var canaryServiceKey = expectedCanaryRequest.NamespacedName
			domain := "example.com"
			var canary = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      canaryServiceKey.Name,
					Namespace: canaryServiceKey.Namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
					},
					CanaryTrafficPercent: kfserving.GetIntReference(20),
					Canary: &kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
					},
				},
				Status: kfserving.InferenceServiceStatus{
					URL: canaryServiceKey.Name + "." + domain,
					Address: &duckv1beta1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   network.GetServiceHostname(canaryServiceKey.Name, canaryServiceKey.Namespace),
							Path:   constants.PredictPath(canaryServiceKey.Name, constants.ProtocolV1),
						},
					},
					Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
						constants.Predictor: kfserving.StatusConfigurationSpec{
							Name: "revision-v1",
						},
					},
				},
			}
			var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(canaryServiceKey.Name),
				Namespace: canaryServiceKey.Namespace}
			var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(canaryServiceKey.Name),
				Namespace: canaryServiceKey.Namespace}
			var virtualServiceName = types.NamespacedName{Name: canaryServiceKey.Name, Namespace: canaryServiceKey.Namespace}

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			canaryInstance := canary.DeepCopy()
			g.Expect(k8sClient.Create(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), canaryInstance)

			defaultService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
				Should(gomega.Succeed())

			canaryService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryService) }, timeout).
				Should(gomega.Succeed())
			expectedCanaryService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CanaryPredictorServiceName(canaryInstance.Name),
					Namespace: canaryInstance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "bar",
									constants.KServiceEndpointLabel:  constants.InferenceServiceCanary,
									constants.KServiceModelLabel:     "bar",
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/target":                           "1",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									constants.StorageInitializerSourceUriInternalAnnotationKey: canary.Spec.Canary.Predictor.Tensorflow.StorageURI,
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultPredictorTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: TensorflowServingImageName + ":" +
												canary.Spec.Canary.Predictor.Tensorflow.RuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{kfserving.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + kfserving.TensorflowServingGRPCPort,
												"--rest_api_port=" + kfserving.TensorflowServingRestPort,
												"--model_name=" + canary.Name,
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
											},
											LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/" + canary.Name,
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
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
			updateDefault.Status.URL, _ = apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(canaryServiceKey.Name),
				canaryServiceKey.Namespace, domain))
			updateDefault.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

			updateCanary := canaryService.DeepCopy()
			updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
			updateCanary.Status.LatestReadyRevisionName = "revision-v2"
			updateCanary.Status.URL, _ = apis.ParseURL(
				"http://" + constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(canaryServiceKey.Name), canaryServiceKey.Namespace, domain))
			updateCanary.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())

			// verify if InferenceService status is updated first then virtual service
			expectedKfsvcStatus := kfserving.InferenceServiceStatus{
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
				URL: "http://" + constants.InferenceServiceHostName(canaryServiceKey.Name,
					canaryService.Namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(canaryServiceKey.Name, constants.ProtocolV1),
						Host:   network.GetServiceHostname(canaryServiceKey.Name, canaryServiceKey.Namespace),
					},
				},
				Traffic:       80,
				CanaryTraffic: 20,
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name: "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(canaryServiceKey.Name), canaryServiceKey.Namespace,
							domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name: "revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(canaryServiceKey.Name), canaryServiceKey.Namespace,
							domain),
					},
				},
			}
			g.Eventually(func() string {
				isvc := &kfserving.InferenceService{}
				if err := k8sClient.Get(context.TODO(), canaryServiceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedKfsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(gomega.BeEmpty())

			virtualService := &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())

			expectedVirtualService := &v1alpha3.VirtualService{
				Spec: istiov1alpha3.VirtualService{
					Gateways: []string{
						constants.KnativeIngressGateway,
						constants.KnativeLocalGateway,
					},
					Hosts: []string{
						constants.InferenceServiceHostName(canaryServiceKey.Name, canaryServiceKey.Namespace, domain),
						network.GetServiceHostname(canaryServiceKey.Name, canaryServiceKey.Namespace),
					},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: []*istiov1alpha3.HTTPMatchRequest{
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(canaryServiceKey.Name, canaryServiceKey.Namespace, domain)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(canaryServiceKey.Name, canaryServiceKey.Namespace)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(canaryServiceKey.Name), canaryServiceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 80,
								},
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.CanaryPredictorServiceName(canaryServiceKey.Name), canaryServiceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 20,
								},
							},
							Retries: &istiov1alpha3.HTTPRetry{
								Attempts:      0,
								PerTryTimeout: nil,
							},
						},
					},
				},
			}
			g.Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))
		})
	})

	Context("Remove canary spec", func() {
		It("Should delete canary service successfully", func() {
			serviceName := fmt.Sprintf("canary-delete-%v", time.Now().UnixNano())
			namespace := "default"
			var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
				Namespace: namespace}
			var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(serviceName),
				Namespace: namespace}
			var virtualServiceName = types.NamespacedName{Name: serviceName, Namespace: namespace}
			var expectedCanaryRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var canaryServiceKey = expectedCanaryRequest.NamespacedName

			var canary = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      canaryServiceKey.Name,
					Namespace: canaryServiceKey.Namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
					},
					CanaryTrafficPercent: kfserving.GetIntReference(20),
					Canary: &kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
					},
				},
				Status: kfserving.InferenceServiceStatus{
					URL: canaryServiceKey.Name + ".svc.cluster.local",
					Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
						constants.Predictor: kfserving.StatusConfigurationSpec{
							Name: "revision-v1",
						},
					},
				},
			}

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile
			// Default and Canary service should be present
			canaryInstance := canary.DeepCopy()
			canaryInstance.Name = serviceName
			g.Expect(k8sClient.Create(context.TODO(), canaryInstance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), canaryInstance)

			defaultService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
				Should(gomega.Succeed())

			canaryService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryService) }, timeout).
				Should(gomega.Succeed())

			// mock update knative service status since knative serving controller is not running in test
			updateDefault := defaultService.DeepCopy()
			updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
			updateDefault.Status.LatestReadyRevisionName = "revision-v1"
			updateDefault.Status.URL, _ = apis.ParseURL(
				"http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain))
			updateDefault.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

			updateCanary := canaryService.DeepCopy()
			updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
			updateCanary.Status.LatestReadyRevisionName = "revision-v2"
			updateCanary.Status.URL, _ = apis.ParseURL(
				"http://" + constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceName), namespace, domain))
			updateCanary.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())

			// Verify if InferenceService status is updated
			expectedKfsvcStatus := kfserving.InferenceServiceStatus{
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
				URL: "http://" + constants.InferenceServiceHostName(serviceName, namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(serviceName, constants.ProtocolV1),
						Host:   network.GetServiceHostname(serviceName, canaryServiceKey.Namespace),
					},
				},
				Traffic:       80,
				CanaryTraffic: 20,
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceName), namespace, domain),
					},
				},
			}

			canaryUpdate := &kfserving.InferenceService{}
			g.Eventually(func() string {
				if err := k8sClient.Get(context.TODO(), canaryServiceKey, canaryUpdate); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedKfsvcStatus, &canaryUpdate.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(gomega.BeEmpty())

			// should see a virtual service with 2 routes
			virtualService := &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())
			g.Expect(len(virtualService.Spec.Http)).To(gomega.Equal(1))
			g.Expect(len(virtualService.Spec.Http[0].Route)).To(gomega.Equal(2))

			// Update instance to remove Canary Spec
			// Canary service should be removed during reconcile
			canaryUpdate.Spec.Canary = nil
			canaryUpdate.Spec.CanaryTrafficPercent = kfserving.GetIntReference(0)
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return k8sClient.Update(context.TODO(), canaryUpdate)
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Need to wait for update propagate back to controller before checking
			canaryDelete := &kfserving.InferenceService{}
			g.Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), canaryServiceKey, canaryDelete); err != nil {
					return false
				}
				return canaryDelete.Spec.Canary == nil
			}, timeout).Should(gomega.BeTrue())

			defaultService = &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultService) }, timeout).
				Should(gomega.Succeed())

			canaryService = &knservingv1.Service{}
			g.Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), canaryPredictor, canaryService)
				return errors.IsNotFound(err)
			}, timeout).Should(gomega.BeTrue())

			expectedKfsvcStatus = kfserving.InferenceServiceStatus{
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
				URL: "http://" + constants.InferenceServiceHostName(serviceName, namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(canaryServiceKey.Name, constants.ProtocolV1),
						Host:   network.GetServiceHostname(canaryServiceKey.Name, canaryServiceKey.Namespace),
					},
				},
				Traffic: 100,
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{},
			}
			g.Eventually(func() *kfserving.InferenceServiceStatus {
				isvc := &kfserving.InferenceService{}
				err := k8sClient.Get(context.TODO(), canaryServiceKey, isvc)
				if err != nil {
					return nil
				}
				return &isvc.Status
			}, timeout).Should(testutils.BeSematicEqual(&expectedKfsvcStatus))

			// should see a virtual service with only 1 route
			virtualService = &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())
			g.Expect(len(virtualService.Spec.Http)).To(gomega.Equal(1))
			g.Expect(len(virtualService.Spec.Http[0].Route)).To(gomega.Equal(1))

		})
	})

	Context("Inference Service with transformer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-transformer"
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
			var virtualServiceName = types.NamespacedName{Name: serviceName, Namespace: namespace}
			var transformer = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Transformer: &kfserving.TransformerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Custom: &kfserving.CustomSpec{
								Container: v1.Container{
									Image: "transformer:v1",
								},
							},
						},
					},
					CanaryTrafficPercent: kfserving.GetIntReference(20),
					Canary: &kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Transformer: &kfserving.TransformerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
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
				Status: kfserving.InferenceServiceStatus{
					URL: serviceName + ".svc.cluster.local",
					Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
						constants.Predictor: kfserving.StatusConfigurationSpec{
							Name: "revision-v1",
						},
					},
				},
			}

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := transformer.DeepCopy()
			g.Expect(k8sClient.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			defaultPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultPredictorService) }, timeout).
				Should(gomega.Succeed())

			canaryPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryPredictorService) }, timeout).
				Should(gomega.Succeed())

			defaultTransformerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultTransformer, defaultTransformerService) }, timeout).
				Should(gomega.Succeed())

			canaryTransformerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryTransformer, canaryTransformerService) }, timeout).
				Should(gomega.Succeed())
			expectedCanaryService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CanaryTransformerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": serviceName,
									constants.KServiceEndpointLabel:  constants.InferenceServiceCanary,
									constants.KServiceModelLabel:     instance.Name,
									constants.KServiceComponentLabel: constants.Transformer.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/target":   "1",
									"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale": "3",
									"autoscaling.knative.dev/minScale": "1",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultTransformerTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "transformer:v2",
											Args: []string{
												"--model_name",
												serviceName,
												"--predictor_host",
												constants.CanaryPredictorServiceName(instance.Name) + "." + instance.Namespace,
												constants.ArgumentHttpPort,
												constants.InferenceServiceDefaultHttpPort,
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

			// mock update knative service status since knative serving controller is not running in test

			// update predictor
			{
				updateDefault := defaultPredictorService.DeepCopy()
				updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
				updateDefault.Status.LatestReadyRevisionName = "revision-v1"
				updateDefault.Status.URL, _ = apis.ParseURL(
					constants.InferenceServiceURL("http", constants.DefaultPredictorServiceName(serviceKey.Name), namespace, domain))
				updateDefault.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

				updateCanary := canaryPredictorService.DeepCopy()
				updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
				updateCanary.Status.LatestReadyRevisionName = "revision-v2"
				updateCanary.Status.URL, _ = apis.ParseURL(
					constants.InferenceServiceURL("http", constants.CanaryPredictorServiceName(serviceKey.Name), namespace, domain))
				updateCanary.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
			}

			// update transformer
			{
				updateDefault := defaultTransformerService.DeepCopy()
				updateDefault.Status.LatestCreatedRevisionName = "t-revision-v1"
				updateDefault.Status.LatestReadyRevisionName = "t-revision-v1"
				updateDefault.Status.URL, _ = apis.ParseURL(
					constants.InferenceServiceURL("http", constants.DefaultTransformerServiceName(serviceKey.Name), namespace, domain))
				updateDefault.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

				updateCanary := canaryTransformerService.DeepCopy()
				updateCanary.Status.LatestCreatedRevisionName = "t-revision-v2"
				updateCanary.Status.LatestReadyRevisionName = "t-revision-v2"
				updateCanary.Status.URL, _ = apis.ParseURL(
					constants.InferenceServiceURL("http", constants.CanaryTransformerServiceName(serviceKey.Name), namespace, domain))
				updateCanary.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
			}

			// verify if InferenceService status is updated
			expectedKfsvcStatus := kfserving.InferenceServiceStatus{
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
				Traffic:       80,
				CanaryTraffic: 20,
				URL:           "http://" + constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(serviceKey.Name, constants.ProtocolV1),
						Host:   network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
					},
				},
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
					constants.Transformer: kfserving.StatusConfigurationSpec{
						Name:     "t-revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultTransformerServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
					constants.Transformer: kfserving.StatusConfigurationSpec{
						Name:     "t-revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryTransformerServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
				},
			}
			g.Eventually(func() string {
				isvc := &kfserving.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedKfsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(gomega.BeEmpty())

			// verify virtual service points to transformer
			virtualService := &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())

			expectedVirtualService := &v1alpha3.VirtualService{
				Spec: istiov1alpha3.VirtualService{
					Gateways: []string{
						constants.KnativeIngressGateway,
						constants.KnativeLocalGateway,
					},
					Hosts: []string{
						constants.InferenceServiceHostName(serviceName, serviceKey.Namespace, domain),
						network.GetServiceHostname(serviceName, serviceKey.Namespace),
					},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: []*istiov1alpha3.HTTPMatchRequest{
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 80,
								},
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.CanaryTransformerServiceName(serviceName), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 20,
								},
							},
							Retries: &istiov1alpha3.HTTPRetry{
								Attempts:      0,
								PerTryTimeout: nil,
							},
						},
					},
				},
			}
			g.Expect(cmp.Diff(virtualService.Spec, expectedVirtualService.Spec)).To(gomega.Equal(""))
		})
	})

	Context("Remove InferenceService component", func() {
		It("Should delete the service successfully", func() {
			serviceName := "svc-with-two-components"
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
			var transformer = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Transformer: &kfserving.TransformerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Custom: &kfserving.CustomSpec{
								Container: v1.Container{
									Image: "transformer:v1",
								},
							},
						},
					},
					CanaryTrafficPercent: kfserving.GetIntReference(20),
					Canary: &kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Transformer: &kfserving.TransformerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
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
				Status: kfserving.InferenceServiceStatus{
					URL: serviceName + ".svc.cluster.local",
					Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
						constants.Predictor: kfserving.StatusConfigurationSpec{
							Name: "revision-v1",
						},
					},
				},
			}

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := transformer.DeepCopy()
			g.Expect(k8sClient.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			defaultPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultPredictorService) }, timeout).
				Should(gomega.Succeed())

			canaryPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryPredictorService) }, timeout).
				Should(gomega.Succeed())

			defaultTransformerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultTransformer, defaultTransformerService) }, timeout).
				Should(gomega.Succeed())

			canaryTransformerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryTransformer, canaryTransformerService) }, timeout).
				Should(gomega.Succeed())

			// Update instance to remove transformer endpoint
			// transformer services should be removed during reconcile
			g.Eventually(func() error {
				updateInstance := &kfserving.InferenceService{}
				g.Eventually(func() error { return k8sClient.Get(context.TODO(), serviceKey, updateInstance) }, timeout).
					Should(gomega.Succeed())
				updateInstance.Spec.Canary.Transformer = nil
				updateInstance.Spec.Default.Transformer = nil
				err := k8sClient.Update(context.TODO(), updateInstance)
				return err
			}, timeout).Should(gomega.BeNil())

			defaultTransformerServiceShouldBeDeleted := &knservingv1.Service{}
			g.Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), defaultTransformer, defaultTransformerServiceShouldBeDeleted)
				return errors.IsNotFound(err)
			}, timeout).Should(gomega.BeTrue())

			canaryTransformerServiceShouldBeDeleted := &knservingv1.Service{}
			g.Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), canaryTransformer, canaryTransformerServiceShouldBeDeleted)
				return errors.IsNotFound(err)
			}, timeout).Should(gomega.BeTrue())
			defaultPredictorServiceShouldExist := &knservingv1.Service{}
			g.Eventually(func() error {
				return k8sClient.Get(context.TODO(), defaultPredictor, defaultPredictorServiceShouldExist)
			}, timeout).
				Should(gomega.Succeed())

			canaryPredictorServiceShouldExist := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryPredictorServiceShouldExist) }, timeout).
				Should(gomega.Succeed())

			g.Eventually(func() bool {
				isvc := &kfserving.InferenceService{}
				err := k8sClient.Get(context.TODO(), serviceKey, isvc)
				if err != nil {
					return false
				}
				if _, ok := (*isvc.Status.Default)[constants.Transformer]; ok {
					return false
				}
				if _, ok := (*isvc.Status.Canary)[constants.Transformer]; ok {
					return false
				}
				if defaultTransformerReady := isvc.Status.GetCondition(kfserving.DefaultTransformerReady); defaultTransformerReady != nil {
					return false
				}
				if canaryTransformerReady := isvc.Status.GetCondition(kfserving.CanaryTransformerReady); canaryTransformerReady != nil {
					return false
				}
				return true
			}, timeout).Should(gomega.BeTrue())
		})
	})

	Context("InferenceService with Explainer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-explainer"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName

			var defaultPredictor = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
				Namespace: namespace}
			var canaryPredictor = types.NamespacedName{Name: constants.CanaryPredictorServiceName(serviceName),
				Namespace: namespace}
			var defaultExplainer = types.NamespacedName{Name: constants.DefaultExplainerServiceName(serviceName),
				Namespace: namespace}
			var canaryExplainer = types.NamespacedName{Name: constants.CanaryExplainerServiceName(serviceName),
				Namespace: namespace}
			var virtualServiceName = types.NamespacedName{Name: serviceName, Namespace: namespace}
			var explainer = &kfserving.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: kfserving.InferenceServiceSpec{
					Default: kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Explainer: &kfserving.ExplainerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Alibi: &v1alpha2.AlibiExplainerSpec{
								Type:           v1alpha2.AlibiAnchorsTabularExplainer,
								RuntimeVersion: "latest",
							},
						},
					},
					CanaryTrafficPercent: kfserving.GetIntReference(20),
					Canary: &kfserving.EndpointSpec{
						Predictor: kfserving.PredictorSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Tensorflow: &kfserving.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Explainer: &kfserving.ExplainerSpec{
							DeploymentSpec: kfserving.DeploymentSpec{
								MinReplicas: v1alpha2.GetIntReference(1),
								MaxReplicas: 3,
							},
							Alibi: &v1alpha2.AlibiExplainerSpec{
								Type:           v1alpha2.AlibiAnchorsTabularExplainer,
								RuntimeVersion: "latest",
							},
						},
					},
				},
				Status: kfserving.InferenceServiceStatus{
					URL: serviceName + ".svc.cluster.local",
					Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
						constants.Predictor: kfserving.StatusConfigurationSpec{
							Name: "revision-v1",
						},
					},
				},
			}

			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			g.Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := explainer.DeepCopy()
			g.Expect(k8sClient.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			defaultPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultPredictor, defaultPredictorService) }, timeout).
				Should(gomega.Succeed())

			canaryPredictorService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryPredictor, canaryPredictorService) }, timeout).
				Should(gomega.Succeed())

			defaultExplainerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), defaultExplainer, defaultExplainerService) }, timeout).
				Should(gomega.Succeed())

			canaryExplainerService := &knservingv1.Service{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), canaryExplainer, canaryExplainerService) }, timeout).
				Should(gomega.Succeed())
			expectedCanaryService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CanaryExplainerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": serviceName,
									constants.KServiceModelLabel:     instance.Name,
									constants.KServiceComponentLabel: constants.Explainer.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/target":   "1",
									"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale": "3",
									"autoscaling.knative.dev/minScale": "1",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultExplainerTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "kfserving/alibi-explainer:latest",
											Name:  constants.InferenceServiceContainerName,
											Args: []string{
												"--model_name",
												serviceName,
												"--predictor_host",
												constants.CanaryPredictorServiceName(instance.Name) + "." + instance.Namespace,
												"--http_port",
												"8080",
												"AnchorTabular",
											},
										},
									},
								},
							},
						},
					},
				},
			}
			g.Expect(cmp.Diff(canaryExplainerService.Spec, expectedCanaryService.Spec)).To(gomega.Equal(""))

			// mock update knative service status since knative serving controller is not running in test

			// update predictor
			{
				updateDefault := defaultPredictorService.DeepCopy()
				updateDefault.Status.LatestCreatedRevisionName = "revision-v1"
				updateDefault.Status.LatestReadyRevisionName = "revision-v1"
				updateDefault.Status.URL, _ = apis.ParseURL(
					constants.InferenceServiceURL("http", constants.DefaultPredictorServiceName(serviceName), namespace, domain))
				updateDefault.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

				updateCanary := canaryPredictorService.DeepCopy()
				updateCanary.Status.LatestCreatedRevisionName = "revision-v2"
				updateCanary.Status.LatestReadyRevisionName = "revision-v2"
				updateCanary.Status.URL, _ = apis.ParseURL(
					"http://" + constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceName), namespace, domain))
				updateCanary.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
			}

			// update explainer
			{
				updateDefault := defaultExplainerService.DeepCopy()
				updateDefault.Status.LatestCreatedRevisionName = "e-revision-v1"
				updateDefault.Status.LatestReadyRevisionName = "e-revision-v1"
				updateDefault.Status.URL, _ = apis.ParseURL(
					"http://" + constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceName), namespace, domain))
				updateDefault.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateDefault)).NotTo(gomega.HaveOccurred())

				updateCanary := canaryExplainerService.DeepCopy()
				updateCanary.Status.LatestCreatedRevisionName = "e-revision-v2"
				updateCanary.Status.LatestReadyRevisionName = "e-revision-v2"
				updateCanary.Status.URL, _ = apis.ParseURL(
					"http://" + constants.InferenceServiceHostName(constants.CanaryExplainerServiceName(serviceName), namespace, domain))
				updateCanary.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				g.Expect(k8sClient.Status().Update(context.TODO(), updateCanary)).NotTo(gomega.HaveOccurred())
			}

			// verify if InferenceService status is updated
			expectedKfsvcStatus := kfserving.InferenceServiceStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{
						{
							Type:     kfserving.CanaryExplainerReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     kfserving.CanaryPredictorReady,
							Status:   "True",
							Severity: "Info",
						},
						{
							Type:     kfserving.DefaultExplainerReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     kfserving.DefaultPredictorReady,
							Status:   "True",
							Severity: "",
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
				Traffic:       80,
				CanaryTraffic: 20,
				URL:           "http://" + constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
				Address: &duckv1beta1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Path:   constants.PredictPath(serviceKey.Name, constants.ProtocolV1),
						Host:   network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
					},
				},
				Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
					constants.Explainer: kfserving.StatusConfigurationSpec{
						Name:     "e-revision-v1",
						Hostname: constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
				},
				Canary: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
					constants.Predictor: kfserving.StatusConfigurationSpec{
						Name:     "revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
					constants.Explainer: kfserving.StatusConfigurationSpec{
						Name:     "e-revision-v2",
						Hostname: constants.InferenceServiceHostName(constants.CanaryExplainerServiceName(serviceKey.Name), serviceKey.Namespace, domain),
					},
				},
			}
			g.Eventually(func() string {
				isvc := &kfserving.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedKfsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(gomega.BeEmpty())

			// verify virtual service creation
			virtualService := &v1alpha3.VirtualService{}
			g.Eventually(func() error { return k8sClient.Get(context.TODO(), virtualServiceName, virtualService) }, timeout).
				Should(gomega.Succeed())

			expectedVirtualService := &v1alpha3.VirtualService{
				Spec: istiov1alpha3.VirtualService{
					Gateways: []string{
						constants.KnativeIngressGateway,
						constants.KnativeLocalGateway,
					},
					Hosts: []string{
						constants.InferenceServiceHostName(serviceName, serviceKey.Namespace, domain),
						network.GetServiceHostname(serviceName, serviceKey.Namespace),
					},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: []*istiov1alpha3.HTTPMatchRequest{
								{
									Uri: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
								{
									Uri: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      80,
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
											"Host": network.GetServiceHostname(constants.DefaultExplainerServiceName(serviceName), serviceKey.Namespace)}},
									},
								},
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      20,
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
											"Host": network.GetServiceHostname(constants.CanaryExplainerServiceName(serviceName), serviceKey.Namespace)}},
									},
								},
							},
							Retries: &istiov1alpha3.HTTPRetry{
								Attempts:      0,
								PerTryTimeout: nil,
							},
						},
						{
							Match: []*istiov1alpha3.HTTPMatchRequest{
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 80,
								},
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.CanaryPredictorServiceName(serviceName), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: constants.LocalGatewayHost,
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 20,
								},
							},
							Retries: &istiov1alpha3.HTTPRetry{
								Attempts:      0,
								PerTryTimeout: nil,
							},
						},
					},
				},
			}

			g.Expect(cmp.Diff(virtualService.Spec, expectedVirtualService.Spec)).To(gomega.Equal(""))
		})
	})
})
