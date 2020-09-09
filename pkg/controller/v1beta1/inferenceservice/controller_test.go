/*
Copyright 2020 kubeflow.org.
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

package inferenceservice

import (
	"context"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)
	var (
		configs = map[string]string{
			"predictors": `{
               "tensorflow": {
                  "image": "tensorflow/serving"
               },
               "sklearn": {
                  "image": "kfserving/sklearnserver"
               },
               "xgboost": {
                  "image": "kfserving/xgbserver"
               }
	         }`,
			"explainers": `{
               "alibi": {
                  "image": "kfserving/alibi-explainer",
			      "defaultImageVersion": "latest"
               }
            }`,
			"ingress": `{
               "ingressGateway": "knative-serving/knative-ingress-gateway",
               "ingressService": "test-destination"
            }`,
		}
	)
	Context("When creating inference service with predictor", func() {
		It("Should have knative service created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			serviceName := "foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name: "kfs",
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									constants.KServiceComponentLabel:      constants.Predictor.String(),
									constants.InferenceServicePodLabelKey: serviceName,
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Tensorflow.StorageURI,
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
								TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "tensorflow/serving:" +
												*isvc.Spec.Predictor.Tensorflow.RuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{v1beta1.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + v1beta1.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
												"--model_name=" + isvc.Name,
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
											},
											//TODO default liveness probe
											/*LivenessProbe: &v1.Probe{
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
											},*/
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(actualService.Spec.ConfigurationSpec).To(gomega.Equal(expectedService.Spec.ConfigurationSpec))
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			// update predictor
			{
				updatedService := actualService.DeepCopy()
				updatedService.Status.LatestCreatedRevisionName = "revision-v1"
				updatedService.Status.LatestReadyRevisionName = "revision-v1"
				updatedService.Status.URL = predictorUrl
				updatedService.Status.Conditions = duckv1.Conditions{
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: "True",
					},
				}
				Expect(k8sClient.Status().Update(context.TODO(), updatedService)).NotTo(gomega.HaveOccurred())
			}
			//assert ingress
			virtualService := &v1alpha3.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, virtualService)
			}, timeout).
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
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1alpha3.StringMatch{
										MatchType: &istiov1alpha3.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Headers: &istiov1alpha3.Headers{
										Request: &istiov1alpha3.Headers_HeaderOperations{
											Set: map[string]string{
												"Host": network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											},
										},
									},
									Destination: &istiov1alpha3.Destination{
										Host: network.GetServiceHostname("cluster-local-gateway", "istio-system"),
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
								},
							},
						},
					},
				},
			}
			Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))
		})
	})

	Context("Inference Service with transformer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-transformer"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName

			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var transformerServiceKey = types.NamespacedName{Name: constants.TransformerServiceName(serviceName),
				Namespace: namespace}
			var transformer = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.13.0"),
							},
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						CustomTransformer: &v1beta1.CustomTransformer{
							PodTemplateSpec: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "transformer:v1",
										},
									},
								},
							},
						},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							LatestReadyRevision: "revision-v1",
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
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := transformer.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			predictorService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, predictorService) }, timeout).
				Should(gomega.Succeed())

			transformerService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerServiceKey, transformerService) }, timeout).
				Should(gomega.Succeed())

			expectedTransformerService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.TransformerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Transformer.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale": "3",
									"autoscaling.knative.dev/minScale": "1",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "transformer:v1",
											Args: []string{
												"--model_name",
												serviceName,
												"--predictor_host",
												constants.PredictorServiceName(instance.Name) + "." + instance.Namespace,
												constants.ArgumentHttpPort,
												constants.InferenceServiceDefaultHttpPort,
											},
										},
									},
								},
							},
						},
					},
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{{Tag: "latest", LatestRevision: proto.Bool(true), Percent: proto.Int64(100)}},
					},
				},
			}

			Expect(cmp.Diff(transformerService.Spec, expectedTransformerService.Spec)).To(gomega.Equal(""))

			// mock update knative service status since knative serving controller is not running in test
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			transformerUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.TransformerServiceName(serviceKey.Name), serviceKey.Namespace, domain))

			// update predictor
			updatedPredictorService := predictorService.DeepCopy()
			updatedPredictorService.Status.LatestCreatedRevisionName = "revision-v1"
			updatedPredictorService.Status.LatestReadyRevisionName = "revision-v1"
			updatedPredictorService.Status.URL = predictorUrl
			updatedPredictorService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorService)).NotTo(gomega.HaveOccurred())

			// update transformer
			updatedTransformerService := transformerService.DeepCopy()
			updatedTransformerService.Status.LatestCreatedRevisionName = "t-revision-v1"
			updatedTransformerService.Status.LatestReadyRevisionName = "t-revision-v1"
			updatedTransformerService.Status.URL = transformerUrl
			updatedTransformerService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedTransformerService)).NotTo(gomega.HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
						{
							Type:     v1beta1.TransformerReady,
							Severity: "Info",
							Status:   "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestReadyRevision:   "revision-v1",
						LatestCreatedRevision: "revision-v1",
						URL:                   predictorUrl,
					},
					v1beta1.TransformerComponent: {
						LatestReadyRevision:   "t-revision-v1",
						LatestCreatedRevision: "t-revision-v1",
						URL:                   transformerUrl,
					},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(gomega.BeEmpty())
		})
	})

	Context("When doing canary out with inference service", func() {
		It("Should have traffic split between two revisions", func() {
			By("By moving canary traffic percent to the latest revision")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			serviceName := "foo-canary"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			var storageUri2 = "s3://test/mnist/export/v2"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name: "kfs",
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			// update predictor status
			updatedService := actualService.DeepCopy()
			updatedService.Status.LatestCreatedRevisionName = "revision-v1"
			updatedService.Status.LatestReadyRevisionName = "revision-v1"
			updatedService.Status.URL = predictorUrl
			updatedService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedService)).NotTo(gomega.HaveOccurred())

			// assert inference service predictor status
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return ""
				}
				return inferenceService.Status.Components[v1beta1.PredictorComponent].LatestReadyRevision
			}, timeout, interval).Should(Equal("revision-v1"))

			// update canary traffic percent to 20%
			updatedIsvc := inferenceService.DeepCopy()
			updatedIsvc.Spec.Predictor.Tensorflow.StorageURI = &storageUri2
			updatedIsvc.Spec.Predictor.CanaryTrafficPercent = proto.Int64(20)
			Expect(k8sClient.Update(context.TODO(), updatedIsvc)).NotTo(gomega.HaveOccurred())

			// update predictor status
			canaryService := updatedService.DeepCopy()
			canaryService.Status.LatestCreatedRevisionName = "revision-v2"
			canaryService.Status.LatestReadyRevisionName = "revision-v2"
			canaryService.Status.URL = predictorUrl
			canaryService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), canaryService)).NotTo(gomega.HaveOccurred())

			// assert predictor service spec
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())
			// assert inference service predictor status
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return ""
				}
				return inferenceService.Status.Components[v1beta1.PredictorComponent].PreviousReadyRevision
			}, timeout, interval).Should(Equal("revision-v1"))

			expectedTrafficTarget := []knservingv1.TrafficTarget{
				{
					Tag:            "latest",
					LatestRevision: proto.Bool(true),
					Percent:        proto.Int64(20),
				},
				{
					Tag:            "prev",
					RevisionName:   "revision-v1",
					LatestRevision: proto.Bool(false),
					Percent:        proto.Int64(80),
				},
			}
			Eventually(func() []knservingv1.TrafficTarget {
				actualService := &knservingv1.Service{}
				err := k8sClient.Get(context.TODO(), predictorServiceKey, actualService)
				if err != nil {
					return []knservingv1.TrafficTarget{}
				} else {
					return actualService.Spec.Traffic
				}
			}, timeout).Should(gomega.Equal(expectedTrafficTarget))
		})
	})

	Context("When creating and deleting inference service without storageUri (multi-model inferenceservice)", func() {
		// Create configmap
		var configMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KFServingNamespace,
			},
			Data: configs,
		}

		serviceName := "bar"
		var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
		var serviceKey = expectedRequest.NamespacedName
		var modelConfigMapKey = types.NamespacedName{Name: constants.ModelConfigName(serviceName, 0),
			Namespace: serviceKey.Namespace}
		ctx := context.Background()

		instance := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceKey.Name,
				Namespace: serviceKey.Namespace,
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						MinReplicas: v1beta1.GetIntReference(1),
						MaxReplicas: 3,
					},
					Tensorflow: &v1beta1.TFServingSpec{
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							RuntimeVersion: proto.String("1.14.0"),
							Container: v1.Container{
								Name: "kfs",
							},
						},
					},
				},
			},
		}

		It("Should have model config created and mounted", func() {
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			By("By creating a new InferenceService")
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				//Check if InferenceService is created
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			modelConfigMap := &v1.ConfigMap{}
			Eventually(func() bool {
				//Check if modelconfig is created
				err := k8sClient.Get(ctx, modelConfigMapKey, modelConfigMap)
				if err != nil {
					return false
				}

				//Verify that this configmap's ownerreference is it's parent InferenceService
				Expect(modelConfigMap.OwnerReferences[0].Name).To(Equal(serviceKey.Name))

				return true
			}, timeout, interval).Should(BeTrue())
		})
	})
})
