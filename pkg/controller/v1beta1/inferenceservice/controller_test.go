/*
Copyright 2021 The KServe Authors.

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
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)
	var (
		defaultResource = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}
		configs = map[string]string{
			"predictors": `{
               "tensorflow": {
                  "image": "tensorflow/serving",
				  "defaultTimeout": "60",
 				  "multiModelServer": false
               },
               "sklearn": {
                 "v1": {
                  	"image": "kfserving/sklearnserver",
					"multiModelServer": true
                 },
                 "v2": {
                  	"image": "kfserving/sklearnserver",
					"multiModelServer": true
                 }
               },
               "xgboost": {
				  	"image": "kfserving/xgbserver",
				  	"multiModelServer": true
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
               "ingressService": "test-destination",
               "localGateway": "knative-serving/knative-local-gateway",
               "localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
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
					Namespace: constants.KServeNamespace,
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
									Name:      "kfs",
									Resources: defaultResource,
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
			predictorServiceKey := types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
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
												"--rest_api_timeout_in_ms=60000",
											},
											Resources: defaultResource,
										},
									},
								},
							},
						},
					},
				},
			}
			expectedService.SetDefaults(context.TODO())
			Expect(actualService.Spec.ConfigurationSpec).To(gomega.Equal(expectedService.Spec.ConfigurationSpec))
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
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
						constants.KnativeLocalGateway,
						constants.KnativeIngressGateway,
					},
					Hosts: []string{
						network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
						constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
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
									Destination: &istiov1alpha3.Destination{
										Host: network.GetServiceHostname("knative-local-gateway", "istio-system"),
										Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 100,
								},
							},
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.Headers_HeaderOperations{
									Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace),
									},
								},
							},
						},
					},
				},
			}
			Expect(virtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))

			//get inference service
			time.Sleep(10 * time.Second)
			actualIsvc := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			//update inference service with annotations and labels
			annotations := map[string]string{"testAnnotation": "test"}
			labels := map[string]string{"testLabel": "test"}
			updatedIsvc := actualIsvc.DeepCopy()
			updatedIsvc.Annotations = annotations
			updatedIsvc.Labels = labels

			Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(gomega.HaveOccurred())
			time.Sleep(10 * time.Second)
			updatedVirtualService := &v1alpha3.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, updatedVirtualService)
			}, timeout, interval).Should(gomega.Succeed())

			Expect(updatedVirtualService.Spec).To(gomega.Equal(expectedVirtualService.Spec))
			Expect(updatedVirtualService.Annotations).To(gomega.Equal(annotations))
			Expect(updatedVirtualService.Labels).To(gomega.Equal(labels))
		})
	})

	Context("Inference Service with transformer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-transformer"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName

			var predictorServiceKey = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
				Namespace: namespace}
			var transformerServiceKey = types.NamespacedName{Name: constants.DefaultTransformerServiceName(serviceName),
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
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Image:     "transformer:v1",
									Resources: defaultResource,
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
					Namespace: constants.KServeNamespace,
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
					Name:      constants.DefaultTransformerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kserve.io/inferenceservice": serviceName,
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
												constants.DefaultPredictorServiceName(instance.Name) + "." + instance.Namespace,
												constants.ArgumentHttpPort,
												constants.InferenceServiceDefaultHttpPort,
											},
											Resources: defaultResource,
										},
									},
								},
							},
						},
					},
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{{LatestRevision: proto.Bool(true), Percent: proto.Int64(100)}},
					},
				},
			}
			expectedTransformerService.SetDefaults(context.TODO())
			Expect(cmp.Diff(transformerService.Spec, expectedTransformerService.Spec)).To(gomega.Equal(""))

			// mock update knative service status since knative serving controller is not running in test
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			transformerUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultTransformerServiceName(serviceKey.Name), serviceKey.Namespace, domain))

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
						Path:   constants.PredictPath(serviceKey.Name, constants.ProtocolV1),
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

	Context("Inference Service with explainer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-explainer"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName

			var predictorServiceKey = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
				Namespace: namespace}
			var explainerServiceKey = types.NamespacedName{Name: constants.DefaultExplainerServiceName(serviceName),
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
					Explainer: &v1beta1.ExplainerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						Alibi: &v1beta1.AlibiExplainerSpec{
							Type: v1beta1.AlibiAnchorsTabularExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								StorageURI:     "s3://test/mnist/explainer",
								RuntimeVersion: proto.String("0.4.0"),
								Container: v1.Container{
									Name:      "kfserving-container",
									Resources: defaultResource,
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
					Namespace: constants.KServeNamespace,
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

			explainerService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), explainerServiceKey, explainerService) }, timeout).
				Should(gomega.Succeed())

			expectedExplainerService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultTransformerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Explainer.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									"internal.serving.kserve.io/storage-initializer-sourceuri": "s3://test/mnist/explainer",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  constants.InferenceServiceContainerName,
											Image: "kfserving/alibi-explainer:0.4.0",
											Args: []string{
												"--model_name",
												serviceName,
												constants.ArgumentHttpPort,
												constants.InferenceServiceDefaultHttpPort,
												"--predictor_host",
												constants.DefaultPredictorServiceName(instance.Name) + "." + instance.Namespace,
												"--storage_uri",
												"/mnt/models",
												"AnchorTabular",
											},
											Resources: defaultResource,
										},
									},
								},
							},
						},
					},
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{{LatestRevision: proto.Bool(true), Percent: proto.Int64(100)}},
					},
				},
			}
			expectedExplainerService.SetDefaults(context.TODO())
			Expect(cmp.Diff(explainerService.Spec, expectedExplainerService.Spec)).To(gomega.Equal(""))

			// mock update knative service status since knative serving controller is not running in test
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			explainerUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceKey.Name), serviceKey.Namespace, domain))

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

			// update explainer
			updatedExplainerervice := explainerService.DeepCopy()
			updatedExplainerervice.Status.LatestCreatedRevisionName = "exp-revision-v1"
			updatedExplainerervice.Status.LatestReadyRevisionName = "exp-revision-v1"
			updatedExplainerervice.Status.URL = explainerUrl
			updatedExplainerervice.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedExplainerervice)).NotTo(gomega.HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:     v1beta1.ExplainerReady,
							Severity: "Info",
							Status:   "True",
						},
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
						Path:   constants.PredictPath(serviceKey.Name, constants.ProtocolV1),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestReadyRevision:   "revision-v1",
						LatestCreatedRevision: "revision-v1",
						URL:                   predictorUrl,
					},
					v1beta1.ExplainerComponent: {
						LatestReadyRevision:   "exp-revision-v1",
						LatestCreatedRevision: "exp-revision-v1",
						URL:                   explainerUrl,
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
					Namespace: constants.KServeNamespace,
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
									Name:      "kfs",
									Resources: defaultResource,
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

			updatedService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, updatedService) }, timeout).
				Should(Succeed())

			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			// update predictor status
			updatedService.Status.LatestCreatedRevisionName = "revision-v1"
			updatedService.Status.LatestReadyRevisionName = "revision-v1"
			updatedService.Status.URL = predictorUrl
			updatedService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}
			updatedService.Status.Traffic = []knservingv1.TrafficTarget{
				{
					LatestRevision: proto.Bool(true),
					RevisionName:   "revision-v1",
					Percent:        proto.Int64(100),
				}}
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return k8sClient.Status().Update(context.TODO(), updatedService)
			})).NotTo(gomega.HaveOccurred())

			// assert inference service predictor status
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return ""
				}
				return inferenceService.Status.Components[v1beta1.PredictorComponent].LatestReadyRevision
			}, timeout, interval).Should(Equal("revision-v1"))

			// assert latest rolled out revision
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return ""
				}
				return inferenceService.Status.Components[v1beta1.PredictorComponent].LatestRolledoutRevision
			}, timeout, interval).Should(Equal("revision-v1"))

			// update canary traffic percent to 20%
			updatedIsvc := inferenceService.DeepCopy()
			updatedIsvc.Spec.Predictor.Tensorflow.StorageURI = &storageUri2
			updatedIsvc.Spec.Predictor.CanaryTrafficPercent = proto.Int64(20)
			Expect(k8sClient.Update(context.TODO(), updatedIsvc)).NotTo(gomega.HaveOccurred())

			// update predictor status
			canaryService := &knservingv1.Service{}
			Eventually(func() string {
				k8sClient.Get(context.TODO(), predictorServiceKey, canaryService)
				return canaryService.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
			}, timeout).Should(Equal(storageUri2))
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

			expectedTrafficTarget := []knservingv1.TrafficTarget{
				{
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

			rolloutIsvc := &v1beta1.InferenceService{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, rolloutIsvc)
				if err != nil {
					return ""
				}
				return rolloutIsvc.Status.Components[v1beta1.PredictorComponent].LatestReadyRevision
			}, timeout, interval).Should(Equal("revision-v2"))

			// rollout canary
			rolloutIsvc.Spec.Predictor.CanaryTrafficPercent = nil

			Expect(k8sClient.Update(context.TODO(), rolloutIsvc)).NotTo(gomega.HaveOccurred())
			expectedTrafficTarget = []knservingv1.TrafficTarget{
				{
					LatestRevision: proto.Bool(true),
					Percent:        proto.Int64(100),
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

			// update predictor knative service status
			serviceRevision2 := &knservingv1.Service{}
			Eventually(func() string {
				k8sClient.Get(context.TODO(), predictorServiceKey, serviceRevision2)
				return serviceRevision2.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
			}, timeout).Should(Equal(storageUri2))
			serviceRevision2.Status.Traffic = []knservingv1.TrafficTarget{
				{
					LatestRevision: proto.Bool(true),
					RevisionName:   "revision-v2",
					Percent:        proto.Int64(100),
				}}
			Expect(k8sClient.Status().Update(context.TODO(), serviceRevision2)).NotTo(gomega.HaveOccurred())
			// assert latest rolled out revision
			expectedIsvc := &v1beta1.InferenceService{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, expectedIsvc)
				if err != nil {
					return ""
				}
				return expectedIsvc.Status.Components[v1beta1.PredictorComponent].LatestRolledoutRevision
			}, timeout, interval).Should(Equal("revision-v2"))
			// assert previous rolled out revision
			Eventually(func() string {
				err := k8sClient.Get(ctx, serviceKey, expectedIsvc)
				if err != nil {
					return ""
				}
				return expectedIsvc.Status.Components[v1beta1.PredictorComponent].PreviousRolledoutRevision
			}, timeout, interval).Should(Equal("revision-v1"))
		})
	})

	Context("When creating and deleting inference service without storageUri (multi-model inferenceservice)", func() {
		// Create configmap
		var configMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
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
					SKLearn: &v1beta1.SKLearnSpec{
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							RuntimeVersion: proto.String("1.14.0"),
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

	Context("When creating an inference service using a ServingRuntime", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-servingruntime"
			servingRuntimeName := "tf-serving"
			namespace := "default"

			var predictorServiceKey = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceName),
				Namespace: namespace}

			var servingRuntime = &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      servingRuntimeName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1alpha1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}

			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), servingRuntime)

			var isvc = &v1beta1.InferenceService{
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
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{
								Name: "tensorflow",
							},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: proto.String("s3://test/mnist/export"),
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
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := isvc.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			predictorService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, predictorService) }, timeout).
				Should(gomega.Succeed())

			expectedPredictorService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName(serviceName),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/mnist/export",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:    constants.InferenceServiceContainerName,
											Image:   "tensorflow/serving:1.14.0",
											Command: []string{"/usr/bin/tensorflow_model_server"},
											Args: []string{
												"--port=9000",
												"--rest_api_port=8080",
												"--model_base_path=/mnt/models",
												"--rest_api_timeout_in_ms=60000",
											},
											Resources: defaultResource,
										},
									},
								},
							},
						},
					},
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{{LatestRevision: proto.Bool(true), Percent: proto.Int64(100)}},
					},
				},
			}

			expectedPredictorService.SetDefaults(context.TODO())
			Expect(cmp.Diff(predictorService.Spec, expectedPredictorService.Spec)).To(gomega.Equal(""))

		})
	})
})
