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
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	istiov1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	operatorv1beta1 "knative.dev/operator/pkg/apis/operator/v1beta1"

	"k8s.io/utils/ptr"

	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)
	var (
		defaultResource = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}
		configs = map[string]string{
			"explainers": `{
				"art": {
					"image": "kserve/art-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
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
	Context("with knative configured to allow zero initial scale", func() {
		When("an InferenceService with minReplicas=0 is created", func() {
			It("should create a knative service with initial-scale:0 and min-scale:0 annotations", func() {
				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(context.TODO(), configMap)

				// Create InferenceService
				serviceName := "initialscale1"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				storageUri := "s3://test/mnist/export"
				ctx := context.Background()
				isvc := &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
								MinReplicas: ptr.To(int32(0)),
							},
							Tensorflow: &v1beta1.TFServingSpec{
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     &storageUri,
									RuntimeVersion: proto.String("1.14.0"),
									Container: corev1.Container{
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

				actualService := &knservingv1.Service{}
				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceKey.Name),
					Namespace: serviceKey.Namespace,
				}
				Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[constants.InitialScaleAnnotationKey]).To(Equal("0"))
				Expect(actualService.Spec.Template.Annotations[constants.MinScaleAnnotationKey]).To(Equal("0"))
			})
		})
		When("an InferenceService with minReplicas!=0 is created", func() {
			It("should create a knative service with no initial-scale annotation and min-scale:<minReplicas> annotation", func() {
				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(context.TODO(), configMap)

				// Create InferenceService
				serviceName := "initialscale2"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				storageUri := "s3://test/mnist/export"
				ctx := context.Background()
				isvc := &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
								MinReplicas: ptr.To(int32(1)),
							},
							Tensorflow: &v1beta1.TFServingSpec{
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     &storageUri,
									RuntimeVersion: proto.String("1.14.0"),
									Container: corev1.Container{
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

				actualService := &knservingv1.Service{}
				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceKey.Name),
					Namespace: serviceKey.Namespace,
				}
				Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[constants.MinScaleAnnotationKey]).To(Equal("1"))
				Expect(constants.InitialScaleAnnotationKey).ShouldNot(BeKeyOf(actualService.Spec.Template.Annotations))
			})
		})
	})
	Context("with knative configured to not allow zero initial scale", func() {
		BeforeEach(func() {
			// Update the existing knativeserving custom resource to set allow-zero-initial-scale to false
			knativeService := &operatorv1beta1.KnativeServing{}
			Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultKnServingName, Namespace: constants.DefaultKnServingNamespace}, knativeService)).ToNot(HaveOccurred())
			knativeService.Spec.CommonSpec.Config["autoscaler"]["allow-zero-initial-scale"] = "false"
			Eventually(func() error {
				return k8sClient.Update(context.TODO(), knativeService)
			}, timeout).Should(Succeed())
		})
		AfterEach(func() {
			// Restore the original knativeserving custom resource configuration
			knativeServiceRestored := &operatorv1beta1.KnativeServing{}
			Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultKnServingName, Namespace: constants.DefaultKnServingNamespace}, knativeServiceRestored)).ToNot(HaveOccurred())
			knativeServiceRestored.Spec.CommonSpec.Config["autoscaler"]["allow-zero-initial-scale"] = "true"
			Eventually(func() error {
				return k8sClient.Update(context.TODO(), knativeServiceRestored)
			}, timeout).Should(Succeed())
		})
		When("an InferenceService with minReplicas=0 is created", func() {
			It("should create a knative service with no initial-scale annotation and min-scale:0 annotation", func() {
				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(context.TODO(), configMap)

				// Create InferenceService
				serviceName := "initialscale3"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				storageUri := "s3://test/mnist/export"
				ctx := context.Background()
				isvc := &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
								MinReplicas: ptr.To(int32(0)),
							},
							Tensorflow: &v1beta1.TFServingSpec{
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     &storageUri,
									RuntimeVersion: proto.String("1.14.0"),
									Container: corev1.Container{
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

				actualService := &knservingv1.Service{}
				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceKey.Name),
					Namespace: serviceKey.Namespace,
				}
				Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[constants.MinScaleAnnotationKey]).To(Equal("0"))
				Expect(constants.InitialScaleAnnotationKey).ShouldNot(BeKeyOf(actualService.Spec.Template.Annotations))
			})
		})
		When("an InferenceService with minReplicas!=0 is created", func() {
			It("should create a knative service with no initial-scale annotation and min-scale:<minReplicas> annotation", func() {
				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(context.TODO(), configMap)

				// Create InferenceService
				serviceName := "initialscale4"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				storageUri := "s3://test/mnist/export"
				ctx := context.Background()
				isvc := &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
								MinReplicas: ptr.To(int32(1)),
							},
							Tensorflow: &v1beta1.TFServingSpec{
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     &storageUri,
									RuntimeVersion: proto.String("1.14.0"),
									Container: corev1.Container{
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

				actualService := &knservingv1.Service{}
				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceKey.Name),
					Namespace: serviceKey.Namespace,
				}
				Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[constants.MinScaleAnnotationKey]).To(Equal("1"))
				Expect(constants.InitialScaleAnnotationKey).ShouldNot(BeKeyOf(actualService.Spec.Template.Annotations))
			})
		})
	})

	Context("an annotation is applied to an InferenceService resource", func() {
		When("the annotation is a member of the serviceAnnotationDisallowedList in the inferenceservice-config configmap", func() {
			It("should not be propagated to any knative revisions", func() {
				// Add the annotation to the inferenceservice-config inferenceService serviceAnnotationDisallowedList
				sampleAnnotation := "test.dev/testing"
				configs["inferenceService"] = fmt.Sprintf("{\"serviceAnnotationDisallowedList\": [\"%s\"]}", sampleAnnotation)
				defer delete(configs, "inferenceService")

				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(context.TODO(), configMap)

				// Create InferenceService
				serviceName := "annotated"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				storageUri := "s3://test/mnist/export"
				ctx := context.Background()
				isvc := &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							sampleAnnotation: "test",
						},
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
								MinReplicas: ptr.To(int32(1)),
								MaxReplicas: 1,
							},
							Tensorflow: &v1beta1.TFServingSpec{
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     &storageUri,
									RuntimeVersion: proto.String("1.14.0"),
									Container: corev1.Container{
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

				actualService := &knservingv1.Service{}
				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceKey.Name),
					Namespace: serviceKey.Namespace,
				}
				Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
					Should(Succeed())

				Expect(sampleAnnotation).ShouldNot(BeKeyOf(actualService.Annotations))
				Expect(sampleAnnotation).ShouldNot(BeKeyOf(actualService.Spec.Template.Annotations))
			})
		})
	})

	Context("When creating inference service with predictor", func() {
		It("Should have knative service created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			configMap := &corev1.ConfigMap{
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
						Containers: []corev1.Container{
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
						ImagePullSecrets: []corev1.LocalObjectReference{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
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
							MinReplicas: ptr.To(int32(1)),
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
								Container: corev1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)
			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
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
								PodSpec: corev1.PodSpec{
									ImagePullSecrets: []corev1.LocalObjectReference{
										{Name: "sr-image-pull-secret"},
									},
									Containers: []corev1.Container{
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
									AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(err).ShouldNot(HaveOccurred())
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))
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
				Expect(k8sClient.Status().Update(context.TODO(), updatedService)).NotTo(HaveOccurred())
			}
			// assert ingress
			virtualService := &istioclientv1beta1.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				}, virtualService)
			}, timeout).
				Should(Succeed())
			expectedVirtualService := &istioclientv1beta1.VirtualService{
				Spec: istiov1beta1.VirtualService{
					Gateways: []string{
						constants.KnativeLocalGateway,
						constants.IstioMeshGateway,
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
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
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
										"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": serviceKey.Namespace,
									},
								},
							},
						},
					},
				},
			}
			Expect(virtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))

			// get inference service
			time.Sleep(10 * time.Second)
			actualIsvc := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			// update inference service with annotations and labels
			annotations := map[string]string{"testAnnotation": "test"}
			labels := map[string]string{"testLabel": "test"}
			updatedIsvc := actualIsvc.DeepCopy()
			updatedIsvc.Annotations = annotations
			updatedIsvc.Labels = labels

			Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(HaveOccurred())
			time.Sleep(10 * time.Second)
			updatedVirtualService := &istioclientv1beta1.VirtualService{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				}, updatedVirtualService)
			}, timeout, interval).Should(Succeed())

			Expect(updatedVirtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))
			Expect(updatedVirtualService.Annotations).To(Equal(annotations))
			Expect(updatedVirtualService.Labels).To(Equal(labels))
		})
		It("Should fail if Knative Serving is not installed", func() {
			// Simulate Knative Serving is absent by setting to false the relevant item in utils.gvResourcesCache variable
			servingResources, getServingResourcesErr := utils.GetAvailableResourcesForApi(cfg, knservingv1.SchemeGroupVersion.String())
			Expect(getServingResourcesErr).ToNot(HaveOccurred())
			defer utils.SetAvailableResourcesForApi(knservingv1.SchemeGroupVersion.String(), servingResources)
			utils.SetAvailableResourcesForApi(knservingv1.SchemeGroupVersion.String(), nil)

			// Create configmap
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create InferenceService
			serviceName := "serverless-isvc"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": "Serverless",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)

			ctx := context.Background()
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			Eventually(func() bool {
				events := &corev1.EventList{}
				err := k8sClient.List(ctx, events, client.InNamespace(serviceKey.Namespace))
				if err != nil {
					return false
				}

				for _, event := range events.Items {
					if event.InvolvedObject.Kind == "InferenceService" &&
						event.InvolvedObject.Name == serviceKey.Name &&
						event.Reason == "ServerlessModeRejected" {
						return true
					}
				}

				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Inference Service with transformer", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-transformer"
			namespace := "default"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			serviceKey := expectedRequest.NamespacedName

			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: namespace,
			}
			transformerServiceKey := types.NamespacedName{
				Name:      constants.TransformerServiceName(serviceName),
				Namespace: namespace,
			}
			transformer := &v1beta1.InferenceService{
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
							MinReplicas: ptr.To(int32(1)),
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
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
							Labels: map[string]string{
								"key2": "val2FromTransformer",
							},
							Annotations: map[string]string{
								"key2": "val2FromTransformer",
							},
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []corev1.Container{
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
			configMap := &corev1.ConfigMap{
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
						Containers: []corev1.Container{
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
			transformer.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			instance := transformer.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			predictorService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, predictorService) }, timeout).
				Should(Succeed())

			transformerService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerServiceKey, transformerService) }, timeout).
				Should(Succeed())
			expectedTransformerService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.TransformerServiceName(instance.Name),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel:     constants.Transformer.String(),
									"key1":                               "val1FromISVC",
									"key2":                               "val2FromTransformer",
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
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
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
									AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cmp.Diff(transformerService.Spec, expectedTransformerService.Spec)).To(Equal(""))

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
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorService)).NotTo(HaveOccurred())

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
			Expect(k8sClient.Status().Update(context.TODO(), updatedTransformerService)).NotTo(HaveOccurred())

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
			}, timeout).Should(BeEmpty())
		})
	})

	Context("Inference Service with transforemer and predictor collocation", func() {
		Context("When predictor and transformer are collocated", func() {
			It("Should create knative service and ingress successfully", func() {
				ctx, cancel := context.WithCancel(context.Background())
				DeferCleanup(cancel)
				serviceName := "isvc-with-collocated-transformer"
				namespace := "default"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
				serviceKey := expectedRequest.NamespacedName
				httpPort := int32(8060)

				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceName),
					Namespace: namespace,
				}

				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, configMap)
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
							Containers: []corev1.Container{
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
				Expect(k8sClient.Create(ctx, servingRuntime)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, servingRuntime)

				isvc := &v1beta1.InferenceService{
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
								MinReplicas: ptr.To(int32(1)),
								MaxReplicas: 3,
							},
							Model: &v1beta1.ModelSpec{
								ModelFormat: v1beta1.ModelFormat{
									Name: "tensorflow",
								},
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     proto.String("s3://test/mnist/export"),
									RuntimeVersion: proto.String("1.13.0"),
								},
							},
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:      constants.TransformerContainerName,
										Image:     "transformer:v1",
										Resources: defaultResource,
										Ports: []corev1.ContainerPort{
											{
												ContainerPort: httpPort,
											},
										},
									},
								},
							},
						},
					},
				}

				//  Create the InferenceService object and expect the Reconcile and knative service to be created
				isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
				Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
				defer k8sClient.Delete(ctx, isvc)
				inferenceService := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, serviceKey, inferenceService)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				actualService := &knservingv1.Service{}
				Eventually(func() error { return k8sClient.Get(ctx, predictorServiceKey, actualService) }, timeout).
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
										"key1":                                "val1FromISVC",
										"key2":                                "val2FromISVC",
									},
									Annotations: map[string]string{
										"serving.kserve.io/deploymentMode":                         "Serverless",
										constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
										"autoscaling.knative.dev/max-scale":                        "3",
										"autoscaling.knative.dev/min-scale":                        "1",
										"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
										"key1":                                                     "val1FromISVC",
										"key2":                                                     "val2FromISVC",
									},
								},
								Spec: knservingv1.RevisionSpec{
									ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
									TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
									PodSpec: corev1.PodSpec{
										Containers: []corev1.Container{
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
											{
												Name:      constants.TransformerContainerName,
												Image:     "transformer:v1",
												Resources: defaultResource,
												Ports: []corev1.ContainerPort{
													{
														ContainerPort: httpPort,
													},
												},
											},
										},
										AutomountServiceAccountToken: proto.Bool(false),
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
				err := k8sClient.Update(ctx, expectedService, client.DryRunAll)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))
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
					Expect(k8sClient.Status().Update(ctx, updatedService)).NotTo(HaveOccurred())
				}
				// assert ingress
				virtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, virtualService)
				}, timeout).
					Should(Succeed())
				expectedVirtualService := &istioclientv1beta1.VirtualService{
					Spec: istiov1beta1.VirtualService{
						Gateways: []string{
							constants.KnativeLocalGateway,
							constants.IstioMeshGateway,
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
										Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
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
											"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											"KServe-Isvc-Name":      serviceName,
											"KServe-Isvc-Namespace": serviceKey.Namespace,
										},
									},
								},
							},
						},
					},
				}
				Expect(virtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))

				// get inference service
				time.Sleep(10 * time.Second)
				actualIsvc := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				// update inference service with annotations and labels
				annotations := map[string]string{"testAnnotation": "test"}
				labels := map[string]string{"testLabel": "test"}
				updatedIsvc := actualIsvc.DeepCopy()
				updatedIsvc.Annotations = annotations
				updatedIsvc.Labels = labels

				Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(HaveOccurred())
				time.Sleep(10 * time.Second)
				updatedVirtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, updatedVirtualService)
				}, timeout, interval).Should(Succeed())

				Expect(updatedVirtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))
				Expect(updatedVirtualService.Annotations).To(Equal(annotations))
				Expect(updatedVirtualService.Labels).To(Equal(labels))
			})
		})
		Context("When predictor and transformer container is collocated in serving runtime", func() {
			It("Should create knative service and ingress successfully", func() {
				ctx, cancel := context.WithCancel(context.Background())
				DeferCleanup(cancel)
				serviceName := "isvc-with-collocation-transformer-runtime"
				namespace := "default"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
				serviceKey := expectedRequest.NamespacedName
				httpPort := int32(8060)

				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceName),
					Namespace: namespace,
				}

				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, configMap)
				// Create ServingRuntime
				servingRuntime := &v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tf-serving-collocation",
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
							Containers: []corev1.Container{
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
								{
									Name:  constants.TransformerContainerName,
									Image: "transformer:v1",
									Args: []string{
										"--model_name={{.Name}}",
									},
									Resources: defaultResource,
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: httpPort,
										},
									},
								},
							},
						},
						Disabled: proto.Bool(false),
					},
				}
				Expect(k8sClient.Create(ctx, servingRuntime)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, servingRuntime)

				isvc := &v1beta1.InferenceService{
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
								MinReplicas: ptr.To(int32(1)),
								MaxReplicas: 3,
							},
							Model: &v1beta1.ModelSpec{
								ModelFormat: v1beta1.ModelFormat{
									Name: "tensorflow",
								},
								Runtime: ptr.To("tf-serving-collocation"),
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     proto.String("s3://test/mnist/export"),
									RuntimeVersion: proto.String("1.13.0"),
								},
							},
						},
					},
				}

				//  Create the InferenceService object and expect the Reconcile and knative service to be created
				isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
				Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
				defer k8sClient.Delete(ctx, isvc)
				inferenceService := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, serviceKey, inferenceService)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				actualService := &knservingv1.Service{}
				Eventually(func() error { return k8sClient.Get(ctx, predictorServiceKey, actualService) }, timeout).
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
										"key1":                                "val1FromISVC",
										"key2":                                "val2FromISVC",
									},
									Annotations: map[string]string{
										"serving.kserve.io/deploymentMode":                         "Serverless",
										constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
										"autoscaling.knative.dev/max-scale":                        "3",
										"autoscaling.knative.dev/min-scale":                        "1",
										"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
										"key1":                                                     "val1FromISVC",
										"key2":                                                     "val2FromISVC",
									},
								},
								Spec: knservingv1.RevisionSpec{
									ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
									TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
									PodSpec: corev1.PodSpec{
										Containers: []corev1.Container{
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
											{
												Name:      constants.TransformerContainerName,
												Image:     "transformer:v1",
												Args:      []string{"--model_name=" + serviceName},
												Resources: defaultResource,
												Ports: []corev1.ContainerPort{
													{
														ContainerPort: httpPort,
													},
												},
											},
										},
										AutomountServiceAccountToken: proto.Bool(false),
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
				err := k8sClient.Update(ctx, expectedService, client.DryRunAll)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))
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
					Expect(k8sClient.Status().Update(ctx, updatedService)).NotTo(HaveOccurred())
				}
				// assert ingress
				virtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, virtualService)
				}, timeout).
					Should(Succeed())
				expectedVirtualService := &istioclientv1beta1.VirtualService{
					Spec: istiov1beta1.VirtualService{
						Gateways: []string{
							constants.KnativeLocalGateway,
							constants.IstioMeshGateway,
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
										Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
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
											"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											"KServe-Isvc-Name":      serviceName,
											"KServe-Isvc-Namespace": serviceKey.Namespace,
										},
									},
								},
							},
						},
					},
				}
				Expect(virtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))

				// get inference service
				time.Sleep(10 * time.Second)
				actualIsvc := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				// update inference service with annotations and labels
				annotations := map[string]string{"testAnnotation": "test"}
				labels := map[string]string{"testLabel": "test"}
				updatedIsvc := actualIsvc.DeepCopy()
				updatedIsvc.Annotations = annotations
				updatedIsvc.Labels = labels

				Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(HaveOccurred())
				time.Sleep(10 * time.Second)
				updatedVirtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, updatedVirtualService)
				}, timeout, interval).Should(Succeed())

				Expect(updatedVirtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))
				Expect(updatedVirtualService.Annotations).To(Equal(annotations))
				Expect(updatedVirtualService.Labels).To(Equal(labels))
			})
		})
		Context("When transformer container is present in both serving runtime and inference service", func() {
			It("Transformer container should be merged successfully", func() {
				ctx, cancel := context.WithCancel(context.Background())
				DeferCleanup(cancel)
				serviceName := "isvc-with-collocated-transformer-runtime"
				namespace := "default"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
				serviceKey := expectedRequest.NamespacedName
				httpPort := int32(8060)

				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceName),
					Namespace: namespace,
				}

				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, configMap)
				// Create ServingRuntime
				servingRuntime := &v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tf-serving-collocation",
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
							Containers: []corev1.Container{
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
								{
									Name:  constants.TransformerContainerName,
									Image: "transformer:v1",
									Args: []string{
										"--model_name={{.Name}}",
									},
									Resources: defaultResource,
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: httpPort,
										},
									},
								},
							},
						},
						Disabled: proto.Bool(false),
					},
				}
				Expect(k8sClient.Create(ctx, servingRuntime)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, servingRuntime)

				isvc := &v1beta1.InferenceService{
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
								MinReplicas: ptr.To(int32(1)),
								MaxReplicas: 3,
							},
							Model: &v1beta1.ModelSpec{
								ModelFormat: v1beta1.ModelFormat{
									Name: "tensorflow",
								},
								Runtime: ptr.To("tf-serving-collocation"),
								PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
									StorageURI:     proto.String("s3://test/mnist/export"),
									RuntimeVersion: proto.String("1.13.0"),
								},
							},
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  constants.TransformerContainerName,
										Image: "transformer:v1",
										Command: []string{
											"transformer",
										},
										Args: []string{
											"--http-port",
											strconv.Itoa(int(httpPort)),
										},
										Resources: defaultResource,
									},
								},
							},
						},
					},
				}

				//  Create the InferenceService object and expect the Reconcile and knative service to be created
				isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
				Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
				defer k8sClient.Delete(ctx, isvc)
				inferenceService := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, serviceKey, inferenceService)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				actualService := &knservingv1.Service{}
				Eventually(func() error { return k8sClient.Get(ctx, predictorServiceKey, actualService) }, timeout).
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
										"key1":                                "val1FromISVC",
										"key2":                                "val2FromISVC",
									},
									Annotations: map[string]string{
										"serving.kserve.io/deploymentMode":                         "Serverless",
										constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
										"autoscaling.knative.dev/max-scale":                        "3",
										"autoscaling.knative.dev/min-scale":                        "1",
										"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
										"key1":                                                     "val1FromISVC",
										"key2":                                                     "val2FromISVC",
									},
								},
								Spec: knservingv1.RevisionSpec{
									ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
									TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
									PodSpec: corev1.PodSpec{
										Containers: []corev1.Container{
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
											{
												Name:  constants.TransformerContainerName,
												Image: "transformer:v1",
												Command: []string{
													"transformer",
												},
												Args:      []string{"--model_name=" + serviceName, "--http-port", strconv.Itoa(int(httpPort))},
												Resources: defaultResource,
												Ports: []corev1.ContainerPort{
													{
														ContainerPort: httpPort,
													},
												},
											},
										},
										AutomountServiceAccountToken: proto.Bool(false),
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
				err := k8sClient.Update(ctx, expectedService, client.DryRunAll)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))
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
					Expect(k8sClient.Status().Update(ctx, updatedService)).NotTo(HaveOccurred())
				}
				// assert ingress
				virtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, virtualService)
				}, timeout).
					Should(Succeed())
				expectedVirtualService := &istioclientv1beta1.VirtualService{
					Spec: istiov1beta1.VirtualService{
						Gateways: []string{
							constants.KnativeLocalGateway,
							constants.IstioMeshGateway,
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
										Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
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
											"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											"KServe-Isvc-Name":      serviceName,
											"KServe-Isvc-Namespace": serviceKey.Namespace,
										},
									},
								},
							},
						},
					},
				}
				Expect(virtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))

				// get inference service
				time.Sleep(10 * time.Second)
				actualIsvc := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				// update inference service with annotations and labels
				annotations := map[string]string{"testAnnotation": "test"}
				labels := map[string]string{"testLabel": "test"}
				updatedIsvc := actualIsvc.DeepCopy()
				updatedIsvc.Annotations = annotations
				updatedIsvc.Labels = labels

				Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(HaveOccurred())
				time.Sleep(10 * time.Second)
				updatedVirtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, updatedVirtualService)
				}, timeout, interval).Should(Succeed())

				Expect(updatedVirtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))
				Expect(updatedVirtualService.Annotations).To(Equal(annotations))
				Expect(updatedVirtualService.Labels).To(Equal(labels))
			})
		})

		Context("When custom predictor and transformer are collocated", func() {
			It("Should create knative service and ingress successfully", func() {
				ctx, cancel := context.WithCancel(context.Background())
				DeferCleanup(cancel)
				serviceName := "isvc-custom-collocated-transformer"
				namespace := "default"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
				serviceKey := expectedRequest.NamespacedName
				httpPort := int32(8060)

				predictorServiceKey := types.NamespacedName{
					Name:      constants.PredictorServiceName(serviceName),
					Namespace: namespace,
				}

				// Create configmap
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: configs,
				}
				Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
				defer k8sClient.Delete(ctx, configMap)

				isvc := &v1beta1.InferenceService{
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
								MinReplicas: ptr.To(int32(1)),
								MaxReplicas: 3,
							},
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    constants.InferenceServiceContainerName,
										Image:   "tensorflow/serving:1.14.0",
										Command: []string{"/usr/bin/tensorflow_model_server"},
										Args: []string{
											"--port=9000",
											"--rest_api_port=8080",
											"--model_base_path=/mnt/models",
										},
										Resources: defaultResource,
									},
									{
										Name:      constants.TransformerContainerName,
										Image:     "transformer:v1",
										Resources: defaultResource,
										Ports: []corev1.ContainerPort{
											{
												ContainerPort: httpPort,
											},
										},
									},
								},
							},
						},
					},
				}

				//  Create the InferenceService object and expect the Reconcile and knative service to be created
				isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
				Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
				defer k8sClient.Delete(ctx, isvc)
				inferenceService := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, serviceKey, inferenceService)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				actualService := &knservingv1.Service{}
				Eventually(func() error { return k8sClient.Get(ctx, predictorServiceKey, actualService) }, timeout).
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
										"key1":                                "val1FromISVC",
										"key2":                                "val2FromISVC",
									},
									Annotations: map[string]string{
										"serving.kserve.io/deploymentMode":  "Serverless",
										"autoscaling.knative.dev/max-scale": "3",
										"autoscaling.knative.dev/min-scale": "1",
										"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
										"key1":                              "val1FromISVC",
										"key2":                              "val2FromISVC",
									},
								},
								Spec: knservingv1.RevisionSpec{
									ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
									TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
									PodSpec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Image:   "tensorflow/serving:1.14.0",
												Name:    constants.InferenceServiceContainerName,
												Command: []string{v1beta1.TensorflowEntrypointCommand},
												Args: []string{
													"--port=" + v1beta1.TensorflowServingGRPCPort,
													"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
													"--model_base_path=" + constants.DefaultModelLocalMountPath,
												},
												Resources: defaultResource,
											},
											{
												Name:      constants.TransformerContainerName,
												Image:     "transformer:v1",
												Resources: defaultResource,
												Ports: []corev1.ContainerPort{
													{
														ContainerPort: httpPort,
													},
												},
											},
										},
										AutomountServiceAccountToken: proto.Bool(false),
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
				err := k8sClient.Update(ctx, expectedService, client.DryRunAll)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))
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
					Expect(k8sClient.Status().Update(ctx, updatedService)).NotTo(HaveOccurred())
				}
				// assert ingress
				virtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, virtualService)
				}, timeout).
					Should(Succeed())
				expectedVirtualService := &istioclientv1beta1.VirtualService{
					Spec: istiov1beta1.VirtualService{
						Gateways: []string{
							constants.KnativeLocalGateway,
							constants.IstioMeshGateway,
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
										Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
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
											"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace),
											"KServe-Isvc-Name":      serviceName,
											"KServe-Isvc-Namespace": serviceKey.Namespace,
										},
									},
								},
							},
						},
					},
				}
				Expect(virtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))

				// get inference service
				time.Sleep(10 * time.Second)
				actualIsvc := &v1beta1.InferenceService{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				// update inference service with annotations and labels
				annotations := map[string]string{"testAnnotation": "test"}
				labels := map[string]string{"testLabel": "test"}
				updatedIsvc := actualIsvc.DeepCopy()
				updatedIsvc.Annotations = annotations
				updatedIsvc.Labels = labels

				Expect(k8sClient.Update(ctx, updatedIsvc)).NotTo(HaveOccurred())
				time.Sleep(10 * time.Second)
				updatedVirtualService := &istioclientv1beta1.VirtualService{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
					}, updatedVirtualService)
				}, timeout, interval).Should(Succeed())

				Expect(updatedVirtualService.Spec.DeepCopy()).To(Equal(expectedVirtualService.Spec.DeepCopy()))
				Expect(updatedVirtualService.Annotations).To(Equal(annotations))
				Expect(updatedVirtualService.Labels).To(Equal(labels))
			})
		})
	})

	Context("When doing canary out with inference service", func() {
		It("Should have traffic split between two revisions", func() {
			By("By moving canary traffic percent to the latest revision")
			// Create configmap
			configMap := &corev1.ConfigMap{
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
						Containers: []corev1.Container{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			storageUri2 := "s3://test/mnist/export/v2"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			updatedService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
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
				},
			}
			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				return k8sClient.Status().Update(context.TODO(), updatedService)
			})).NotTo(HaveOccurred())

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
			Expect(k8sClient.Update(context.TODO(), updatedIsvc)).NotTo(HaveOccurred())

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
			Expect(k8sClient.Status().Update(context.TODO(), canaryService)).NotTo(HaveOccurred())

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
			}, timeout).Should(Equal(expectedTrafficTarget))

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

			Expect(k8sClient.Update(context.TODO(), rolloutIsvc)).NotTo(HaveOccurred())
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
			}, timeout).Should(Equal(expectedTrafficTarget))

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
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), serviceRevision2)).NotTo(HaveOccurred())
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
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: configs,
		}

		serviceName := "bar"
		expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
		serviceKey := expectedRequest.NamespacedName
		modelConfigMapKey := types.NamespacedName{
			Name:      constants.ModelConfigName(serviceName, 0),
			Namespace: serviceKey.Namespace,
		}
		ctx := context.Background()

		instance := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceKey.Name,
				Namespace: serviceKey.Namespace,
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						MinReplicas: ptr.To(int32(1)),
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
				// Check if InferenceService is created
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			modelConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				// Check if modelconfig is created
				err := k8sClient.Get(ctx, modelConfigMapKey, modelConfigMap)
				if err != nil {
					return false
				}

				// Verify that this configmap's ownerreference is it's parent InferenceService
				Expect(modelConfigMap.OwnerReferences[0].Name).To(Equal(serviceKey.Name))

				return true
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When creating an inference service using a ServingRuntime", func() {
		It("Should create successfully", func() {
			serviceName := "svc-with-servingruntime"
			namespace := "default"

			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: namespace,
			}
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
						Containers: []corev1.Container{
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
						ImagePullSecrets: []corev1.LocalObjectReference{
							{Name: "sr-image-pull-secret"},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), servingRuntime)

			isvc := &v1beta1.InferenceService{
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
							MinReplicas: ptr.To(int32(1)),
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
							ImagePullSecrets: []corev1.LocalObjectReference{
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
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			instance := isvc.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

			predictorService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, predictorService) }, timeout).
				Should(Succeed())

			expectedPredictorService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName(serviceName),
					Namespace: instance.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferenceservice": serviceName,
									constants.KServiceComponentLabel:     constants.Predictor.String(),
									"key1":                               "val1FromSR",
									"key2":                               "val2FromISVC",
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
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
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
									ImagePullSecrets: []corev1.LocalObjectReference{
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
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cmp.Diff(predictorService.Spec, expectedPredictorService.Spec)).To(Equal(""))
		})
	})

	Context("When creating an inference service with a ServingRuntime which does not exists", func() {
		It("Should fail with reason RuntimeNotRecognized", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			serviceName := "svc-with-unknown-servingruntime"
			servingRuntimeName := "tf-serving-unknown"
			namespace := "default"

			predictorServiceKey := types.NamespacedName{Name: serviceName, Namespace: namespace}

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
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
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
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

			failureInfo := v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeNotRecognized,
				Message: "Waiting for runtime to become available",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(Equal(""))
		})
	})

	Context("When creating an inference service with a ServingRuntime which is disabled", func() {
		It("Should fail with reason RuntimeDisabled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			serviceName := "svc-with-disabled-servingruntime"
			servingRuntimeName := "tf-serving-disabled"
			namespace := "default"

			predictorServiceKey := types.NamespacedName{Name: serviceName, Namespace: namespace}

			servingRuntime := &v1alpha1.ServingRuntime{
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
						Containers: []corev1.Container{
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

			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), servingRuntime)

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
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
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the InferenceService object and expect the Reconcile and knative service to be created
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
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

			failureInfo := v1beta1.FailureInfo{
				Reason:  v1beta1.RuntimeDisabled,
				Message: "Specified runtime is disabled",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(Equal(""))
		})
	})

	Context("When creating an inference service with a ServingRuntime which does not support specified model format", func() {
		It("Should fail with reason NoSupportingRuntime", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			serviceName := "svc-with-unsupported-servingruntime"
			servingRuntimeName := "tf-serving-unsupported"
			namespace := "default"

			predictorServiceKey := types.NamespacedName{Name: serviceName, Namespace: namespace}

			// Create configmap
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := &v1alpha1.ServingRuntime{
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
						Containers: []corev1.Container{
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

			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), servingRuntime)

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
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
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
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

			failureInfo := v1beta1.FailureInfo{
				Reason:  v1beta1.NoSupportingRuntime,
				Message: "Specified runtime does not support specified framework/version",
			}
			Expect(inferenceService.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			Expect(inferenceService.Status.ModelStatus.ModelRevisionStates.TargetModelState).To(Equal(v1beta1.FailedToLoad))
			Expect(cmp.Diff(&failureInfo, inferenceService.Status.ModelStatus.LastFailureInfo)).To(Equal(""))
		})
	})

	Context("When creating inference service with storage.kserve.io/readonly", func() {
		defaultIsvc := func(namespace string, name string, storageUri string) *v1beta1.InferenceService {
			predictor := v1beta1.PredictorSpec{
				ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 3,
				},
				Tensorflow: &v1beta1.TFServingSpec{
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI:     &storageUri,
						RuntimeVersion: proto.String("1.14.0"),
						Container: corev1.Container{
							Name:      constants.InferenceServiceContainerName,
							Resources: defaultResource,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "predictor-volume"},
							},
						},
					},
				},
			}
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},

				Spec: v1beta1.InferenceServiceSpec{
					Predictor: predictor,
				},
			}
			return isvc
		}

		createServingRuntime := func(namespace string, name string) *v1alpha1.ServingRuntime {
			// Define and create serving runtime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
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
						Containers: []corev1.Container{
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
						ImagePullSecrets: []corev1.LocalObjectReference{
							{Name: "sr-image-pull-secret"},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(HaveOccurred())
			return servingRuntime
		}

		createInferenceServiceConfigMap := func() *corev1.ConfigMap {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}

			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			return configMap
		}

		It("should have the readonly annotation set to true in the knative serving pod spec", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			configMap := createInferenceServiceConfigMap()
			defer k8sClient.Delete(ctx, configMap)

			serviceName := "readonly-true-isvc"
			serviceNamespace := "default"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: serviceNamespace}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"

			servingRuntime := createServingRuntime(serviceKey.Namespace, "tf-serving")
			defer k8sClient.Delete(ctx, servingRuntime)

			// Define InferenceService
			isvc := defaultIsvc(serviceKey.Namespace, serviceKey.Name, storageUri)
			isvc.Annotations = map[string]string{}
			isvc.Annotations[constants.StorageReadonlyAnnotationKey] = "true"
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, isvc)

			// Knative service
			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			// Check readonly value
			Expect(actualService.Spec.Template.Annotations[constants.StorageReadonlyAnnotationKey]).
				To(Equal("true"))
		})

		It("should have the readonly annotation set to false in the knative serving pod spec", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			configMap := createInferenceServiceConfigMap()
			defer k8sClient.Delete(ctx, configMap)

			serviceName := "readonly-false-isvc"
			serviceNamespace := "default"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: serviceNamespace}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"

			servingRuntime := createServingRuntime(serviceKey.Namespace, "tf-serving")
			defer k8sClient.Delete(ctx, servingRuntime)

			// Define InferenceService
			isvc := defaultIsvc(serviceKey.Namespace, serviceKey.Name, storageUri)
			isvc.Annotations = map[string]string{}
			isvc.Annotations[constants.StorageReadonlyAnnotationKey] = "false"
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, isvc)

			// Knative service
			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			// Check readonly value
			Expect(actualService.Spec.Template.Annotations[constants.StorageReadonlyAnnotationKey]).
				To(Equal("false"))
		})
	})

	Context("When creating an inference service with invalid Storage URI", func() {
		It("Should fail with reason ModelLoadFailed", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			serviceName := "servingruntime-test"
			servingRuntimeName := "tf-serving"
			namespace := "default"
			inferenceServiceKey := types.NamespacedName{Name: serviceName, Namespace: namespace}

			// Create configmap
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := &v1alpha1.ServingRuntime{
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
						Containers: []corev1.Container{
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

			Expect(k8sClient.Create(context.TODO(), servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), servingRuntime)

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
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
			Expect(k8sClient.Create(context.TODO(), isvc)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), isvc)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName + "-predictor-" + namespace + "-00001-deployment-76464ds2zpv",
					Namespace: namespace,
					Labels:    map[string]string{"serving.knative.dev/revision": serviceName + "-predictor-" + namespace + "-00001"},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
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
					Containers: []corev1.Container{
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
							Env: []corev1.EnvVar{
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
				return err == nil
			}, timeout).Should(BeTrue())
			defer k8sClient.Delete(context.TODO(), pod)

			podStatusPatch := []byte(`{"status":{"containerStatuses":[{"image":"tensorflow/serving:1.14.0","name":"kserve-container","lastState":{},"state":{"waiting":{"reason":"PodInitializing"}}}],"initContainerStatuses":[{"image":"kserve/storage-initializer:latest","name":"storage-initializer","lastState":{"terminated":{"exitCode":1,"message":"Invalid Storage URI provided","reason":"Error"}},"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`)
			Eventually(func() bool {
				err := k8sClient.Status().Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, podStatusPatch))
				return err == nil
			}, timeout).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: namespace,
			}
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
			})).NotTo(HaveOccurred())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, inferenceServiceKey, inferenceService)
				if err != nil {
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
						"kserveIngressGateway": "kserve/kserve-ingress-gateway",
						"disableIstioVirtualHost": true,
						"ingressGateway": "knative-serving/knative-ingress-gateway",
						"localGateway": "knative-serving/knative-local-gateway",
						"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
					}`
				} else {
					copiedConfigs[key] = value
				}
			}
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			serviceName := "foo-disable-istio"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
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
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
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
			Expect(k8sClient.Status().Update(context.TODO(), updatedService)).NotTo(HaveOccurred())
			// get inference service
			time.Sleep(10 * time.Second)
			actualIsvc := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, expectedRequest.NamespacedName, actualIsvc)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(actualIsvc.Status.URL).To(Equal(&apis.URL{
				Scheme: "http",
				Host:   constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain),
			}))
			Expect(actualIsvc.Status.Address.URL).To(Equal(&apis.URL{
				Scheme: "https",
				Host:   network.GetServiceHostname(fmt.Sprintf("%s-%s", serviceKey.Name, string(constants.Predictor)), serviceKey.Namespace),
			}))
		})
	})
	Context("Set CaBundle ConfigMap in inferenceservice-config confimap", func() {
		It("Should not create a global cabundle configMap in a user namespace when CaBundleConfigMapName set ''", func() {
			// Create configmap
			configMap := &corev1.ConfigMap{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			caBundleConfigMap := &corev1.ConfigMap{}
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

			configMap := &corev1.ConfigMap{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
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

			caBundleConfigMap := &corev1.ConfigMap{}
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
			configMap := &corev1.ConfigMap{
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
			originalCabundleConfigMap := &corev1.ConfigMap{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
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

			caBundleConfigMap := &corev1.ConfigMap{}
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
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: copiedConfigs,
			}

			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create original cabundle configmap with right file name
			cabundleConfigMapData := make(map[string]string)
			// cabundle data
			cabundleConfigMapData["cabundle.crt"] = "SAMPLE_CA_BUNDLE"
			originalCabundleConfigMap := &corev1.ConfigMap{
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
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
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

			caBundleConfigMap := &corev1.ConfigMap{}
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
	Context("If the InferenceService occurred any error", func() {
		It("InferenceService should generate event message about non-ready conditions", func() {
			// Create configmap
			configMap := &corev1.ConfigMap{
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
						Containers: []corev1.Container{
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
			serviceName := "test-err-msg"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			storageUri := "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: corev1.Container{
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

			updatedService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, updatedService) }, timeout).
				Should(Succeed())

			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.PredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			// update predictor status
			updatedService.Status.URL = predictorUrl
			updatedService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "True",
				},
			}

			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				return k8sClient.Status().Update(context.TODO(), updatedService)
			})).NotTo(HaveOccurred())

			// assert inference service predictor status
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// should turn to fail
			time.Sleep(10 * time.Second)
			updatedService.Status.Conditions = duckv1.Conditions{
				{
					Type:   knservingv1.ServiceConditionReady,
					Status: "False",
				},
			}
			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				return k8sClient.Status().Update(context.TODO(), updatedService)
			})).NotTo(HaveOccurred())

			r := &InferenceServiceReconciler{
				Client: k8sClient,
			}

			Eventually(func() bool {
				events := &corev1.EventList{}
				err := k8sClient.List(ctx, events, client.InNamespace(serviceKey.Namespace))
				if err != nil {
					return false
				}

				notReadyEventFound := false
				for _, event := range events.Items {
					if event.InvolvedObject.Kind == "InferenceService" &&
						event.InvolvedObject.Name == serviceKey.Name &&
						event.Reason == string(InferenceServiceNotReadyState) {

						notReadyEventFound = true
						break
					}
				}

				err = k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				failConditions := r.GetFailConditions(inferenceService)
				expectedConditions := strings.Split(failConditions, ", ")

				for _, expectedCondition := range expectedConditions {
					found := false
					for _, cond := range inferenceService.Status.Conditions {
						if string(cond.Type) == expectedCondition && cond.Status == corev1.ConditionFalse {
							found = true
							break
						}
					}
					if !found {
						return false
					}
				}

				return notReadyEventFound
			}, timeout, interval).Should(BeTrue())
		})
	})
})
