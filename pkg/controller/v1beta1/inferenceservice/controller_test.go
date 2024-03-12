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
	"fmt"
	"time"

	"knative.dev/pkg/kmp"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	istiov1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"ingressService": "test-destination",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
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
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving",
					Namespace: "default",
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
						Labels: map[string]string{
							"key1": "val1FromSR",
							"key2": "val2FromSR",
							"key3": "val3FromSR",
						},
						Annotations: map[string]string{
							"key1": "val1FromSR",
							"key2": "val2FromSR",
							"key3": "val3FromSR",
						},
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
						ImagePullSecrets: []v1.LocalObjectReference{
							{Name: "sr-image-pull-secret"},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			// Create InferenceService
			serviceName := "foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Labels: map[string]string{
						"key2": "val2FromISVC",
						"key3": "val3FromISVC",
					},
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": "Serverless",
						"key2":                             "val2FromISVC",
						"key3":                             "val3FromISVC",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
							Labels: map[string]string{
								"key3": "val3FromPredictor",
							},
							Annotations: map[string]string{
								"key3": "val3FromPredictor",
							},
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)
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
									"key1":                                "val1FromSR",
									"key2":                                "val2FromISVC",
									"key3":                                "val3FromPredictor",
								},
								Annotations: map[string]string{
									"serving.kserve.io/deploymentMode":                         "Serverless",
									constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
									"autoscaling.knative.dev/max-scale":                        "3",
									"autoscaling.knative.dev/min-scale":                        "1",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"key1":                                                     "val1FromSR",
									"key2":                                                     "val2FromISVC",
									"key3":                                                     "val3FromPredictor",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
								TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
								PodSpec: v1.PodSpec{
									ImagePullSecrets: []v1.LocalObjectReference{
										{Name: "sr-image-pull-secret"},
									},
									Containers: []v1.Container{
										{
											Image: "tensorflow/serving:" +
												*isvc.Spec.Predictor.Model.RuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{v1beta1.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + v1beta1.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
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
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{{LatestRevision: proto.Bool(true), Percent: proto.Int64(100)}},
					},
				},
			}
			// Set ResourceVersion which is required for update operation.
			expectedService.ResourceVersion = actualService.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedService, client.DryRunAll)
			Expect(err).Should(BeNil())
			Expect(kmp.SafeDiff(actualService.Spec, expectedService.Spec)).To(Equal(""))
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
					{
						Type:   knservingv1.ServiceConditionRoutesReady,
						Status: "True",
					},
					{
						Type:   knservingv1.ServiceConditionConfigurationsReady,
						Status: "True",
					},
				}
				Expect(k8sClient.Status().Update(context.TODO(), updatedService)).NotTo(gomega.HaveOccurred())
			}
			//assert ingress
			virtualService := &istioclientv1beta1.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, virtualService)
			}, timeout).
				Should(gomega.Succeed())
			expectedVirtualService := &istioclientv1beta1.VirtualService{
				Spec: istiov1beta1.VirtualService{
					Gateways: []string{
						constants.KnativeLocalGateway,
						constants.KnativeIngressGateway,
					},
					Hosts: []string{
						network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace),
						constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain),
					},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Gateways: []string{constants.KnativeLocalGateway},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceKey.Name, serviceKey.Namespace)),
										},
									},
								},
								{
									Gateways: []string{constants.KnativeIngressGateway},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceKey.Name, serviceKey.Namespace, domain)),
										},
									},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{
										Host: network.GetServiceHostname("knative-local-gateway", "istio-system"),
										Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host": network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
									},
								},
							},
						},
					},
				},
			}
			Expect(virtualService.Spec.DeepCopy()).To(gomega.Equal(expectedVirtualService.Spec.DeepCopy()))

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
			updatedVirtualService := &istioclientv1beta1.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, updatedVirtualService)
			}, timeout, interval).Should(gomega.Succeed())

			Expect(updatedVirtualService.Spec.DeepCopy()).To(gomega.Equal(expectedVirtualService.Spec.DeepCopy()))
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

			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var transformerServiceKey = types.NamespacedName{Name: constants.TransformerServiceName(serviceName),
				Namespace: namespace}
			var transformer = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
					Labels: map[string]string{
						"key1": "val1FromISVC",
						"key2": "val2FromISVC",
					},
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": "Serverless",
						"key1":                             "val1FromISVC",
						"key2":                             "val2FromISVC",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
							Labels: map[string]string{
								"key2": "val2FromPredictor",
							},
							Annotations: map[string]string{
								"key2": "val2FromPredictor",
							},
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
							Labels: map[string]string{
								"key2": "val2FromTransformer",
							},
							Annotations: map[string]string{
								"key2": "val2FromTransformer",
							},
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
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving",
					Namespace: "default",
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
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			// Create the InferenceService object and expect the Reconcile and knative service to be created
			transformer.DefaultInferenceService(nil, nil)
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
								Labels: map[string]string{"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Transformer.String(),
									"key1":                           "val1FromISVC",
									"key2":                           "val2FromTransformer",
								},
								Annotations: map[string]string{
									"serving.kserve.io/deploymentMode":  "Serverless",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/max-scale": "3",
									"autoscaling.knative.dev/min-scale": "1",
									"key1":                              "val1FromISVC",
									"key2":                              "val2FromTransformer",
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
											Name:      constants.InferenceServiceContainerName,
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
			// Set ResourceVersion which is required for update operation.
			expectedTransformerService.ResourceVersion = transformerService.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedTransformerService, client.DryRunAll)
			Expect(err).Should(BeNil())
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
				{
					Type:   knservingv1.ServiceConditionRoutesReady,
					Status: "True",
				},
				{
					Type:   knservingv1.ServiceConditionConfigurationsReady,
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
				{
					Type:   knservingv1.ServiceConditionRoutesReady,
					Status: "True",
				},
				{
					Type:   knservingv1.ServiceConditionConfigurationsReady,
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
							Type:     v1beta1.PredictorRouteReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.PredictorConfigurationReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
						{
							Type:     v1beta1.RoutesReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.LatestDeploymentReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.TransformerReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.TransformerRouteReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.TransformerConfigurationReady,
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
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.Condition{}, "LastTransitionTime", "Severity"))
			}, timeout).Should(gomega.BeEmpty())
		})
	})

	Context("Inference Service with explainer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-explainer"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName

			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var explainerServiceKey = types.NamespacedName{Name: constants.ExplainerServiceName(serviceName),
				Namespace: namespace}
			var explainer = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
					Labels: map[string]string{
						"key1": "val1FromISVC",
						"key2": "val2FromISVC",
					},
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": "Serverless",
						"key1":                             "val1FromISVC",
						"key2":                             "val2FromISVC",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
							Labels: map[string]string{
								"key2": "val2FromPredictor",
							},
							Annotations: map[string]string{
								"key2": "val2FromPredictor",
							},
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
							Labels: map[string]string{
								"key2": "val2FromExplainer",
							},
							Annotations: map[string]string{
								"key2": "val2FromExplainer",
							},
						},
						Alibi: &v1beta1.AlibiExplainerSpec{
							Type: v1beta1.AlibiAnchorsTabularExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								StorageURI:     "s3://test/mnist/explainer",
								RuntimeVersion: proto.String("0.4.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
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
			instance := explainer.DeepCopy()
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
					Name:      constants.DefaultExplainerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Explainer.String(),
									"key1":                           "val1FromISVC",
									"key2":                           "val2FromExplainer",
								},
								Annotations: map[string]string{
									"serving.kserve.io/deploymentMode":                         "Serverless",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/max-scale":                        "3",
									"autoscaling.knative.dev/min-scale":                        "1",
									"internal.serving.kserve.io/storage-initializer-sourceuri": "s3://test/mnist/explainer",
									"key1": "val1FromISVC",
									"key2": "val2FromExplainer",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  constants.InferenceServiceContainerName,
											Image: "kserve/alibi-explainer:0.4.0",
											Args: []string{
												"--model_name",
												serviceName,
												constants.ArgumentHttpPort,
												constants.InferenceServiceDefaultHttpPort,
												"--predictor_host",
												constants.PredictorServiceName(instance.Name) + "." + instance.Namespace,
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
			// Set ResourceVersion which is required for update operation.
			expectedExplainerService.ResourceVersion = explainerService.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), explainerService, client.DryRunAll)
			Expect(err).Should(BeNil())
			Expect(cmp.Diff(explainerService.Spec, expectedExplainerService.Spec)).To(gomega.Equal(""))

			// mock update knative service status since knative serving controller is not running in test
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			explainerUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.ExplainerServiceName(serviceKey.Name), serviceKey.Namespace, domain))

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
				{
					Type:   knservingv1.ServiceConditionRoutesReady,
					Status: "True",
				},
				{
					Type:   knservingv1.ServiceConditionConfigurationsReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorService)).NotTo(gomega.HaveOccurred())

			// update explainer
			updatedExplainerService := explainerService.DeepCopy()
			updatedExplainerService.Status.LatestCreatedRevisionName = "exp-revision-v1"
			updatedExplainerService.Status.LatestReadyRevisionName = "exp-revision-v1"
			updatedExplainerService.Status.URL = explainerUrl
			updatedExplainerService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
				{
					Type:   knservingv1.ServiceConditionRoutesReady,
					Status: "True",
				},
				{
					Type:   knservingv1.ServiceConditionConfigurationsReady,
					Status: "True",
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedExplainerService)).NotTo(gomega.HaveOccurred())

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
							Type:     v1beta1.ExplainerRoutesReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.ExplainerConfigurationReady,
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
							Type:     v1beta1.PredictorRouteReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.PredictorConfigurationReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
						{
							Type:     v1beta1.RoutesReady,
							Severity: "Info",
							Status:   "True",
						},
						{
							Type:     v1beta1.LatestDeploymentReady,
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
					v1beta1.ExplainerComponent: {
						LatestReadyRevision:   "exp-revision-v1",
						LatestCreatedRevision: "exp-revision-v1",
						URL:                   explainerUrl,
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.Condition{}, "LastTransitionTime", "Severity"))
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
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving",
					Namespace: "default",
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
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			// Create Canary InferenceService
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil)
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
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, updatedService) }, timeout).
				Should(Succeed())

			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
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
			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
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
			updatedIsvc.Spec.Predictor.Model.StorageURI = &storageUri2
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
			namespace := "default"

			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving",
					Namespace: "default",
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
						Labels: map[string]string{
							"key1": "val1FromSR",
							"key2": "val2FromSR",
						},
						Annotations: map[string]string{
							"key1": "val1FromSR",
							"key2": "val2FromSR",
						},
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
						ImagePullSecrets: []v1.LocalObjectReference{
							{Name: "sr-image-pull-secret"},
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
					Labels: map[string]string{
						"key2": "val2FromISVC",
					},
					Annotations: map[string]string{
						"key2": "val2FromISVC",
					},
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
						PodSpec: v1beta1.PodSpec{
							ImagePullSecrets: []v1.LocalObjectReference{
								{Name: "isvc-image-pull-secret"},
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
									"key1":                           "val1FromSR",
									"key2":                           "val2FromISVC",
								},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/mnist/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/max-scale":                        "3",
									"autoscaling.knative.dev/min-scale":                        "1",
									"key1":                                                     "val1FromSR",
									"key2":                                                     "val2FromISVC",
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
									ImagePullSecrets: []v1.LocalObjectReference{
										{Name: "isvc-image-pull-secret"},
										{Name: "sr-image-pull-secret"},
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

			// Set ResourceVersion which is required for update operation.
			expectedPredictorService.ResourceVersion = predictorService.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), predictorService, client.DryRunAll)
			Expect(err).Should(BeNil())
			Expect(cmp.Diff(predictorService.Spec, expectedPredictorService.Spec)).To(gomega.Equal(""))

		})
	})

	Context("When creating an inference service with a ServingRuntime which does not exists", func() {
		It("Should fail with reason RuntimeNotRecognized", func() {
			serviceName := "svc-with-unknown-servingruntime"
			servingRuntimeName := "tf-serving-unknown"
			namespace := "default"

			var predictorServiceKey = types.NamespacedName{Name: serviceName, Namespace: namespace}

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
							Runtime: &servingRuntimeName,
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: proto.String("s3://test/mnist/export"),
							},
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
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), isvc)

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, predictorServiceKey, inferenceService)
				if err != nil {
					return false
				}
				if inferenceService.Status.ModelStatus.LastFailureInfo == nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			var failureInfo = v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeNotRecognized,
				Message: "Waiting for runtime to become available",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(gomega.Equal(""))
		})
	})

	Context("When creating an inference service with a ServingRuntime which is disabled", func() {
		It("Should fail with reason RuntimeDisabled", func() {
			serviceName := "svc-with-disabled-servingruntime"
			servingRuntimeName := "tf-serving-disabled"
			namespace := "default"

			var predictorServiceKey = types.NamespacedName{Name: serviceName, Namespace: namespace}

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
					Disabled: proto.Bool(true),
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
							Runtime: &servingRuntimeName,
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: proto.String("s3://test/mnist/export"),
							},
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
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), isvc)

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, predictorServiceKey, inferenceService)
				if err != nil {
					return false
				}
				if inferenceService.Status.ModelStatus.LastFailureInfo == nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			var failureInfo = v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeDisabled,
				Message: "Specified runtime is disabled",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(gomega.Equal(""))
		})
	})

	Context("When creating an inference service with a ServingRuntime which does not support specified model format", func() {
		It("Should fail with reason NoSupportingRuntime", func() {
			serviceName := "svc-with-unsupported-servingruntime"
			servingRuntimeName := "tf-serving-unsupported"
			namespace := "default"

			var predictorServiceKey = types.NamespacedName{Name: serviceName, Namespace: namespace}

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
								Name: "sklearn",
							},
							Runtime: &servingRuntimeName,
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: proto.String("s3://test/mnist/export"),
							},
						},
					},
				},
			}

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), isvc)

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, predictorServiceKey, inferenceService)
				if err != nil {
					return false
				}
				if inferenceService.Status.ModelStatus.LastFailureInfo == nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			var failureInfo = v1beta1.FailureInfo{
				Reason:  v1beta1.NoSupportingRuntime,
				Message: "Specified runtime does not support specified framework/version",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(gomega.Equal(""))
		})
	})

	Context("When creating an inference service with invalid Storage URI", func() {
		It("Should fail with reason ModelLoadFailed", func() {
			serviceName := "servingruntime-test"
			servingRuntimeName := "tf-serving"
			namespace := "default"
			var inferenceServiceKey = types.NamespacedName{Name: serviceName, Namespace: namespace}

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
							Runtime: &servingRuntimeName,
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: proto.String("s3://test/mnist/invalid"),
							},
						},
					},
				},
			}

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(gomega.HaveOccurred())
			defer k8sClient.Delete(context.TODO(), isvc)

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName + "-predictor-" + namespace + "-00001-deployment-76464ds2zpv",
					Namespace: namespace,
					Labels:    map[string]string{"serving.knative.dev/revision": serviceName + "-predictor-" + namespace + "-00001"},
				},
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{
							Name:  "storage-initializer",
							Image: "kserve/storage-initializer:latest",
							Args: []string{
								"gs://kserve-invalid/models/sklearn/1.0/model",
								"/mnt/models",
							},
							Resources: defaultResource,
						},
					},
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
							Env: []v1.EnvVar{
								{
									Name:  "PORT",
									Value: "8080",
								},
								{
									Name:  "K_REVISION",
									Value: serviceName + "-predictor-" + namespace + "-00001",
								},
								{
									Name:  "K_CONFIGURATION",
									Value: serviceName + "-predictor-" + namespace,
								},
								{
									Name:  "K_SERVICE",
									Value: serviceName + "-predictor-" + namespace,
								},
							},
							Resources: defaultResource,
						},
					},
				},
			}
			Eventually(func() bool {
				err := k8sClient.Create(context.TODO(), pod)
				if err != nil {
					fmt.Printf("Error #%v\n", err)
					return false
				}
				return true
			}, timeout).Should(BeTrue())
			defer k8sClient.Delete(context.TODO(), pod)

			podStatusPatch := []byte(`{"status":{"containerStatuses":[{"image":"tensorflow/serving:1.14.0","name":"kserve-container","lastState":{},"state":{"waiting":{"reason":"PodInitializing"}}}],"initContainerStatuses":[{"image":"kserve/storage-initializer:latest","name":"storage-initializer","lastState":{"terminated":{"exitCode":1,"message":"Invalid Storage URI provided","reason":"Error"}},"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`)
			Eventually(func() bool {
				err := k8sClient.Status().Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, podStatusPatch))
				if err != nil {
					fmt.Printf("Error #%v\n", err)
					return false
				}
				return true
			}, timeout).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceName), namespace, domain))
			// update predictor status
			updatedService := actualService.DeepCopy()
			updatedService.Status.LatestCreatedRevisionName = serviceName + "-predictor-" + namespace + "-00001"
			updatedService.Status.URL = predictorUrl
			updatedService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "False",
				},
			}
			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				return k8sClient.Status().Update(context.TODO(), updatedService)
			})).NotTo(gomega.HaveOccurred())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, inferenceServiceKey, inferenceService)
				if err != nil {
					fmt.Printf("Error %#v\n", err)
					return false
				}
				if inferenceService.Status.ModelStatus.LastFailureInfo == nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.BlockedByFailedLoad))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(inferenceService.Status.ModelStatus.LastFailureInfo.Reason).To(Equal(v1beta1.ModelLoadFailed))
			Expect(inferenceService.Status.ModelStatus.LastFailureInfo.Message).To(Equal("Invalid Storage URI provided"))
		})
	})

	Context("When creating inference service with predictor and without top level istio virtual service", func() {
		It("Should have knative service created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			copiedConfigs := make(map[string]string)
			for key, value := range configs {
				if key == "ingress" {
					copiedConfigs[key] = `{
						"disableIstioVirtualHost": true,
						"ingressGateway": "knative-serving/knative-ingress-gateway",
						"ingressService": "test-destination",
						"localGateway": "knative-serving/knative-local-gateway",
						"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
					}`
				} else {
					copiedConfigs[key] = value
				}
			}
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			serviceName := "foo-disable-istio"
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)
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
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), predictorServiceKey, actualService)
			}, timeout).Should(Succeed())
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			// update predictor
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
			// get inference service
			time.Sleep(10 * time.Second)
			actualIsvc := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(actualIsvc.Status.URL).To(gomega.Equal(&apis.URL{
				Scheme: "http",
				Host:   constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
			}))
			Expect(actualIsvc.Status.Address.URL).To(gomega.Equal(&apis.URL{
				Scheme: "http",
				Host:   network.GetServiceHostname(fmt.Sprintf("%s-%s", serviceKey.Name, string(constants.Predictor)), serviceKey.Namespace),
			}))
		})
	})
	Context("Set CaBundle ConfigMap in inferenceservice-config confimap", func() {
		It("Should not create a global cabundle configMap in a user namespace when CaBundleConfigMapName set ''", func() {
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

			By("By creating a new InferenceService")
			serviceName := "sample-isvc"
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			caBundleConfigMap := &v1.ConfigMap{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: constants.DefaultGlobalCaBundleConfigMapName, Namespace: "default"}, caBundleConfigMap)
				if err != nil {
					if apierr.IsNotFound(err) {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

		})
		It("Should not create a global cabundle configmap in a user namespace when the target cabundle configmap in the 'inferenceservice-config' configmap does not exist", func() {
			// Create configmap
			copiedConfigs := make(map[string]string)
			for key, value := range configs {
				if key == "storageInitializer" {
					copiedConfigs[key] = `{
							"image" : "kserve/storage-initializer:latest",
							"memoryRequest": "100Mi",
							"memoryLimit": "1Gi",
							"cpuRequest": "100m",
							"cpuLimit": "1",
							"CaBundleConfigMapName": "not-exist-configmap",
							"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
							"enableDirectPvcVolumeMount": false						
					}`
				} else {
					copiedConfigs[key] = value
				}
			}

			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}

			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			By("By creating a new InferenceService")
			serviceName := "sample-isvc-2"
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			caBundleConfigMap := &v1.ConfigMap{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: constants.DefaultGlobalCaBundleConfigMapName, Namespace: "default"}, caBundleConfigMap)
				if err != nil {
					if apierr.IsNotFound(err) {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

		})
		It("Should not create a global cabundle configmap in a user namespace when the cabundle.crt file data does not exist in the target cabundle configmap in the 'inferenceservice-config' configmap", func() {
			// Create configmap
			copiedConfigs := make(map[string]string)
			for key, value := range configs {
				if key == "storageInitializer" {
					copiedConfigs[key] = `{
							"image" : "kserve/storage-initializer:latest",
							"memoryRequest": "100Mi",
							"memoryLimit": "1Gi",
							"cpuRequest": "100m",
							"cpuLimit": "1",
							"CaBundleConfigMapName": "test-cabundle-with-wrong-file-name",
							"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
							"enableDirectPvcVolumeMount": false						
					}`
				} else {
					copiedConfigs[key] = value
				}
			}
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}

			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create original cabundle configmap with wrong file name
			cabundleConfigMapData := make(map[string]string)
			cabundleConfigMapData["wrong-cabundle-name.crt"] = "SAMPLE_CA_BUNDLE"
			var originalCabundleConfigMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cabundle-with-wrong-file-name",
					Namespace: constants.KServeNamespace,
				},
				Data: cabundleConfigMapData,
			}

			Expect(k8sClient.Create(context.TODO(), originalCabundleConfigMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), originalCabundleConfigMap)

			By("By creating a new InferenceService")
			serviceName := "sample-isvc-3"
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			caBundleConfigMap := &v1.ConfigMap{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: constants.DefaultGlobalCaBundleConfigMapName, Namespace: "default"}, caBundleConfigMap)
				if err != nil {
					if apierr.IsNotFound(err) {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should create a global cabundle configmap in a user namespace when it meets all conditions and an inferenceservice is created", func() {
			// Create configmap
			copiedConfigs := make(map[string]string)
			for key, value := range configs {
				if key == "storageInitializer" {
					copiedConfigs[key] = `{
					"image" : "kserve/storage-initializer:latest",
					"memoryRequest": "100Mi",
					"memoryLimit": "1Gi",
					"cpuRequest": "100m",
					"cpuLimit": "1",
					"CaBundleConfigMapName": "test-cabundle-with-right-file-name",
					"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
					"enableDirectPvcVolumeMount": false						
			}`
				} else {
					copiedConfigs[key] = value
				}
			}
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}

			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			//Create original cabundle configmap with right file name
			cabundleConfigMapData := make(map[string]string)
			// cabundle data
			cabundleConfigMapData["cabundle.crt"] = "SAMPLE_CA_BUNDLE"
			var originalCabundleConfigMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cabundle-with-right-file-name",
					Namespace: constants.KServeNamespace,
				},
				Data: cabundleConfigMapData,
			}

			Expect(k8sClient.Create(context.TODO(), originalCabundleConfigMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), originalCabundleConfigMap)

			By("By creating a new InferenceService")
			serviceName := "sample-isvc-4"
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			caBundleConfigMap := &v1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: constants.DefaultGlobalCaBundleConfigMapName, Namespace: "default"}, caBundleConfigMap)
				if err != nil {
					if apierr.IsNotFound(err) {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})
})
