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

package inferencegraph

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var _ = Describe("Inference Graph controller test", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)

	configs := map[string]string{
		"router": `{
					  "image": "kserve/router:v0.10.0",
					  "memoryRequest": "100Mi",
					  "memoryLimit": "500Mi",
					  "cpuRequest": "100m",
					  "cpuLimit": "100m",
					  "headers": {
						"propagate": [
						  "Authorization",
						  "Intuit_tid"
						]
					  }
				}`,
	}

	expectedReadinessProbe := constants.GetRouterReadinessProbe()

	Context("with knative configured to not allow zero initial scale", func() {
		When("a Serverless InferenceGraph is created with an initial scale annotation and value of zero", func() {
			It("should ignore the annotation", func() {
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

				// Create InferenceGraph
				graphName := "initialscale1"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "0",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations).NotTo(HaveKey(autoscaling.InitialScaleAnnotationKey))
			})
		})
		When("a Serverless InferenceGraph is created with an initial scale annotation and valid non-zero integer value", func() {
			It("should override the default initial scale value with the annotation value", func() {
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

				// Create InferenceGraph
				graphName := "initialscale2"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "3",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[autoscaling.InitialScaleAnnotationKey]).To(Equal("3"))
			})
		})
		When("a Serverless InferenceGraph is created with an initial scale annotation and invalid non-integer value", func() {
			It("should ignore the annotation", func() {
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

				// Create InferenceGraph
				graphName := "initialscale3"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "non-integer",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations).NotTo(HaveKey(autoscaling.InitialScaleAnnotationKey))
			})
		})
	})
	Context("with knative configured to allow zero initial scale", func() {
		BeforeEach(func() {
			time.Sleep(10 * time.Second)
			// Patch the existing config-autoscaler configmap to set allow-zero-initial-scale to true
			configAutoscaler := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.AutoscalerConfigmapName,
					Namespace: constants.AutoscalerConfigmapNamespace,
				},
			}
			configPatch := []byte(`{"data":{"allow-zero-initial-scale":"true"}}`)
			Eventually(func() error {
				return k8sClient.Patch(context.TODO(), configAutoscaler, client.RawPatch(types.StrategicMergePatchType, configPatch))
			}, timeout).Should(Succeed())
		})
		AfterEach(func() {
			time.Sleep(10 * time.Second)
			// Restore the default autoscaling configuration
			configAutoscaler := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.AutoscalerConfigmapName,
					Namespace: constants.AutoscalerConfigmapNamespace,
				},
			}
			configPatch := []byte(`{"data":{}}`)
			Eventually(func() error {
				return k8sClient.Patch(context.TODO(), configAutoscaler, client.RawPatch(types.StrategicMergePatchType, configPatch))
			}, timeout).Should(Succeed())
		})
		When("a Serverless InferenceGraph is created with an initial scale annotation and value of zero", func() {
			It("should override the default initial scale value with the annotation value", func() {
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

				graphName := "initialscale4"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "0",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[autoscaling.InitialScaleAnnotationKey]).To(Equal("0"))
			})
		})
		When("a Serverless InferenceGraph is created with an initial scale annotation and valid non-zero integer value", func() {
			It("should override the default initial scale value with the annotation value", func() {
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

				graphName := "initialscale5"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "3",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations[autoscaling.InitialScaleAnnotationKey]).To(Equal("3"))
			})
		})
		When("a Serverless InferenceGraph is created with an initial scale annotation and invalid non-integer value", func() {
			It("should ignore the annotation", func() {
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

				graphName := "initialscale6"
				expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
				serviceKey := expectedRequest.NamespacedName
				ctx := context.Background()
				var minScale int32 = 2
				ig := &v1alpha1.InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceKey.Name,
						Namespace: serviceKey.Namespace,
						Annotations: map[string]string{
							"serving.kserve.io/deploymentMode":    string(constants.Serverless),
							autoscaling.InitialScaleAnnotationKey: "non-integer",
						},
					},
					Spec: v1alpha1.InferenceGraphSpec{
						MinReplicas: &minScale,
						Nodes: map[string]v1alpha1.InferenceRouter{
							v1alpha1.GraphRootNodeName: {
								RouterType: v1alpha1.Sequence,
								Steps: []v1alpha1.InferenceStep{
									{
										InferenceTarget: v1alpha1.InferenceTarget{
											ServiceURL: "http://someservice.exmaple.com",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
				defer k8sClient.Delete(ctx, ig)

				actualService := &knservingv1.Service{}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), serviceKey, actualService)
				}, timeout).
					Should(Succeed())

				Expect(actualService.Spec.Template.Annotations).NotTo(HaveKey(autoscaling.InitialScaleAnnotationKey))
			})
		})
	})

	Context("When creating an inferencegraph with headers in global config", func() {
		It("Should create a knative service with headers as env var of podspec", func() {
			By("By creating a new InferenceGraph")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode1"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": graphName,
									constants.KServeWorkloadKind:       "InferenceGraph",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Env: []corev1.EnvVar{
												{
													Name:  "PROPAGATE_HEADERS",
													Value: "Authorization,Intuit_tid",
												},
											},
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{}}",
											},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("500Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("100Mi"),
												},
											},
											ReadinessProbe: expectedReadinessProbe,
											SecurityContext: &corev1.SecurityContext{
												Privileged:               proto.Bool(false),
												RunAsNonRoot:             proto.Bool(true),
												ReadOnlyRootFilesystem:   proto.Bool(true),
												AllowPrivilegeEscalation: proto.Bool(false),
												Capabilities: &corev1.Capabilities{
													Drop: []corev1.Capability{corev1.Capability("ALL")},
												},
											},
										},
									},
									AutomountServiceAccountToken: proto.Bool(false),
								},
							},
						},
					},
				},
			}
			// Set ResourceVersion which is required for update operation.
			expectedKnService.ResourceVersion = actualKnServiceCreated.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedKnService, client.DryRunAll)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kmp.SafeDiff(actualKnServiceCreated.Spec, expectedKnService.Spec)).To(Equal(""))
		})
	})

	Context("When creating an IG with resource requirements in the spec", func() {
		It("Should propagate to underlying pod", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode2"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("123m"),
							corev1.ResourceMemory: resource.MustParse("123Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("123m"),
							corev1.ResourceMemory: resource.MustParse("123Mi"),
						},
					},
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": graphName,
									constants.KServeWorkloadKind:       "InferenceGraph",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Env: []corev1.EnvVar{
												{
													Name:  "PROPAGATE_HEADERS",
													Value: "Authorization,Intuit_tid",
												},
											},
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{\"limits\":{\"cpu\":\"123m\",\"memory\":\"123Mi\"},\"requests\":{\"cpu\":\"123m\",\"memory\":\"123Mi\"}}}",
											},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("123m"),
													corev1.ResourceMemory: resource.MustParse("123Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("123m"),
													corev1.ResourceMemory: resource.MustParse("123Mi"),
												},
											},
											ReadinessProbe: expectedReadinessProbe,
											SecurityContext: &corev1.SecurityContext{
												Privileged:               proto.Bool(false),
												RunAsNonRoot:             proto.Bool(true),
												ReadOnlyRootFilesystem:   proto.Bool(true),
												AllowPrivilegeEscalation: proto.Bool(false),
												Capabilities: &corev1.Capabilities{
													Drop: []corev1.Capability{corev1.Capability("ALL")},
												},
											},
										},
									},
									AutomountServiceAccountToken: proto.Bool(false),
								},
							},
						},
					},
				},
			}
			// Set ResourceVersion which is required for update operation.
			expectedKnService.ResourceVersion = actualKnServiceCreated.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedKnService, client.DryRunAll)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kmp.SafeDiff(actualKnServiceCreated.Spec, expectedKnService.Spec)).To(Equal(""))
		})
	})

	Context("When creating an IG with podaffinity in the spec", func() {
		It("Should propagate to underlying pod", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode3"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},

				Spec: v1alpha1.InferenceGraphSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "serving.kserve.io/inferencegraph",
													Operator: metav1.LabelSelectorOpIn,
													Values: []string{
														graphName,
													},
												},
											},
										},
										TopologyKey: "topology.kubernetes.io/zone",
									},
								},
							},
						},
					},
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": graphName,
									constants.KServeWorkloadKind:       "InferenceGraph",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Env: []corev1.EnvVar{
												{
													Name:  "PROPAGATE_HEADERS",
													Value: "Authorization,Intuit_tid",
												},
											},
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{},\"affinity\":{\"podAffinity\":{\"preferredDuringSchedulingIgnoredDuringExecution\":[{\"weight\":100,\"podAffinityTerm\":{\"labelSelector\":{\"matchExpressions\":[{\"key\":\"serving.kserve.io/inferencegraph\",\"operator\":\"In\",\"values\":[\"singlenode3\"]}]},\"topologyKey\":\"topology.kubernetes.io/zone\"}}]}}}",
											},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("500Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("100Mi"),
												},
											},
											SecurityContext: &corev1.SecurityContext{
												Privileged:               proto.Bool(false),
												RunAsNonRoot:             proto.Bool(true),
												ReadOnlyRootFilesystem:   proto.Bool(true),
												AllowPrivilegeEscalation: proto.Bool(false),
												Capabilities: &corev1.Capabilities{
													Drop: []corev1.Capability{corev1.Capability("ALL")},
												},
											},
											ReadinessProbe: expectedReadinessProbe,
										},
									},
									Affinity: &corev1.Affinity{
										PodAffinity: &corev1.PodAffinity{
											PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
												{
													Weight: 100,
													PodAffinityTerm: corev1.PodAffinityTerm{
														LabelSelector: &metav1.LabelSelector{
															MatchExpressions: []metav1.LabelSelectorRequirement{
																{
																	Key:      "serving.kserve.io/inferencegraph",
																	Operator: metav1.LabelSelectorOpIn,
																	Values: []string{
																		graphName,
																	},
																},
															},
														},
														TopologyKey: "topology.kubernetes.io/zone",
													},
												},
											},
										},
									},
									AutomountServiceAccountToken: proto.Bool(false),
								},
							},
						},
					},
				},
			}
			// Set ResourceVersion which is required for update operation.
			expectedKnService.ResourceVersion = actualKnServiceCreated.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedKnService, client.DryRunAll)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(kmp.SafeDiff(actualKnServiceCreated.Spec, expectedKnService.Spec)).To(Equal(""))
		})
	})

	Context("When creating an inferencegraph in Raw deployment mode with annotations", func() {
		It("Should create a raw k8s resources with podspec", func() {
			By("By creating a new InferenceGraph")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "igraw1"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.RawDeployment),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				if err != nil {
					return false
				}
				By("Inference graph retrieved")
				return true
			}, timeout, interval).Should(BeTrue())

			actualK8sDeploymentCreated := &appsv1.Deployment{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, serviceKey, actualK8sDeploymentCreated); err != nil {
					return false
				}
				By("K8s Deployment retrieved")
				return true
			}, timeout, interval).Should(BeTrue())

			actualK8sServiceCreated := &corev1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, serviceKey, actualK8sServiceCreated); err != nil {
					return false
				}
				By("K8s Service retrieved")
				return true
			}, timeout, interval).Should(BeTrue())

			// No KNative Service should get created in Raw deployment mode
			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated); err != nil {
					By("KNative Service not retrieved")
					return false
				}
				return true
			}, timeout).
				Should(BeFalse())

			// No Knative Route should get created in Raw deployment mode
			actualKnRouteCreated := &knservingv1.Route{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), serviceKey, actualKnRouteCreated); err != nil {
					return false
				}
				return true
			}, timeout).
				Should(BeFalse())

			result := int32(1)
			Expect(actualK8sDeploymentCreated.Name).To(Equal(graphName))
			Expect(actualK8sDeploymentCreated.Spec.Replicas).To(Equal(&result))
			Expect(actualK8sDeploymentCreated.Spec.Template.Spec.Containers).To(Not(BeNil()))
			Expect(actualK8sDeploymentCreated.Spec.Template.Spec.Containers[0].Image).To(Not(BeNil()))
			Expect(actualK8sDeploymentCreated.Spec.Template.Spec.Containers[0].Args).To(Not(BeNil()))
		})
	})

	Context("When creating an InferenceGraph in Serverless mode", func() {
		It("Should fail if Knative Serving is not installed", func() {
			// Simulate Knative Serving is absent by setting to false the relevant item in utils.gvResourcesCache variable
			servingResources, getServingResourcesErr := utils.GetAvailableResourcesForApi(cfg, knservingv1.SchemeGroupVersion.String())
			Expect(getServingResourcesErr).ToNot(HaveOccurred())
			defer utils.SetAvailableResourcesForApi(knservingv1.SchemeGroupVersion.String(), servingResources)
			utils.SetAvailableResourcesForApi(knservingv1.SchemeGroupVersion.String(), nil)

			By("By creating a new InferenceGraph")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			graphName := "singlenode1"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)

			Eventually(func() bool {
				events := &corev1.EventList{}
				err := k8sClient.List(ctx, events, client.InNamespace(serviceKey.Namespace))
				if err != nil {
					return false
				}

				for _, event := range events.Items {
					if event.InvolvedObject.Kind == "InferenceGraph" &&
						event.InvolvedObject.Name == serviceKey.Name &&
						event.Reason == "ServerlessModeRejected" {
						return true
					}
				}

				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When creating an IG with tolerations in the spec", func() {
		It("Should propagate to underlying pod", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode4"
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			serviceKey := expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},

				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.example.com",
									},
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "key1",
							Operator: corev1.TolerationOpEqual,
							Value:    "value1",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": graphName,
									constants.KServeWorkloadKind:       "InferenceGraph",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: nil,
								TimeoutSeconds:       nil,
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Env: []corev1.EnvVar{
												{
													Name:  "PROPAGATE_HEADERS",
													Value: "Authorization,Intuit_tid",
												},
											},
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.example.com\"}]}},\"resources\":{},\"tolerations\":[{\"key\":\"key1\",\"operator\":\"Equal\",\"value\":\"value1\",\"effect\":\"NoSchedule\"}]}",
											},
											Resources: corev1.ResourceRequirements{
												Limits: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("500Mi"),
												},
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("100m"),
													corev1.ResourceMemory: resource.MustParse("100Mi"),
												},
											},
											ReadinessProbe: expectedReadinessProbe,
											SecurityContext: &corev1.SecurityContext{
												Privileged:               proto.Bool(false),
												RunAsNonRoot:             proto.Bool(true),
												ReadOnlyRootFilesystem:   proto.Bool(true),
												AllowPrivilegeEscalation: proto.Bool(false),
												Capabilities: &corev1.Capabilities{
													Drop: []corev1.Capability{corev1.Capability("ALL")},
												},
											},
										},
									},
									Tolerations: []corev1.Toleration{
										{
											Key:      "key1",
											Operator: corev1.TolerationOpEqual,
											Value:    "value1",
											Effect:   corev1.TaintEffectNoSchedule,
										},
									},
									AutomountServiceAccountToken: proto.Bool(false),
								},
							},
						},
					},
				},
			}
			// Set ResourceVersion which is required for update operation.
			expectedKnService.ResourceVersion = actualKnServiceCreated.ResourceVersion

			// Do a dry-run update. This will populate our local knative service object with any default values
			// that are present on the remote version.
			err := k8sClient.Update(context.TODO(), expectedKnService, client.DryRunAll)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(actualKnServiceCreated.Spec).To(BeComparableTo(expectedKnService.Spec))
		})
	})
})
