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
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"knative.dev/pkg/kmp"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
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

	Context("When creating an IG in Raw deployment mode with auth", func() {
		var configMap *corev1.ConfigMap
		var inferenceGraph *v1alpha1.InferenceGraph
		ctx := context.Background()

		BeforeEach(func() {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())

			graphName := "igrawauth1"
			inferenceGraph = &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      graphName,
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.RawDeployment),
						constants.EnableAuthAnnotationKey:  "true",
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
			Expect(k8sClient.Create(ctx, inferenceGraph)).Should(Succeed())
		})
		AfterEach(func() {
			_ = k8sClient.Delete(ctx, inferenceGraph)
			igKey := types.NamespacedName{Namespace: inferenceGraph.GetNamespace(), Name: inferenceGraph.GetName()}
			Eventually(func() error { return k8sClient.Get(ctx, igKey, inferenceGraph) }, timeout, interval).ShouldNot(Succeed())

			_ = k8sClient.Delete(ctx, configMap)
			cmKey := types.NamespacedName{Namespace: configMap.GetNamespace(), Name: configMap.GetName()}
			Eventually(func() error { return k8sClient.Get(ctx, cmKey, configMap) }, timeout, interval).ShouldNot(Succeed())
		})

		It("Should create or update a ClusterRoleBinding giving privileges to validate auth", func() {
			Eventually(func(g Gomega) {
				crbKey := types.NamespacedName{Name: constants.InferenceGraphAuthCRBName}
				clusterRoleBinding := rbacv1.ClusterRoleBinding{}
				g.Expect(k8sClient.Get(ctx, crbKey, &clusterRoleBinding)).To(Succeed())

				crGVK, err := apiutil.GVKForObject(&rbacv1.ClusterRole{}, scheme.Scheme)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(clusterRoleBinding.RoleRef).To(Equal(rbacv1.RoleRef{
					APIGroup: crGVK.Group,
					Kind:     crGVK.Kind,
					Name:     "system:auth-delegator",
				}))
				g.Expect(clusterRoleBinding.Subjects).To(ContainElement(rbacv1.Subject{
					Kind:      "ServiceAccount",
					APIGroup:  "",
					Name:      inferenceGraph.ResolveServiceAccountName(),
					Namespace: inferenceGraph.GetNamespace(),
				}))
			}, timeout, interval).Should(Succeed())
		})

		It("Should create a ServiceAccount for querying the Kubernetes API to check tokens", func() {
			Eventually(func(g Gomega) {
				saKey := types.NamespacedName{Namespace: inferenceGraph.GetNamespace(), Name: inferenceGraph.ResolveServiceAccountName()}
				serviceAccount := corev1.ServiceAccount{}
				g.Expect(k8sClient.Get(ctx, saKey, &serviceAccount)).To(Succeed())
				g.Expect(serviceAccount.OwnerReferences).ToNot(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("Should configure the InferenceGraph deployment with auth enabled", func() {
			Eventually(func(g Gomega) {
				igDeployment := appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: inferenceGraph.GetNamespace(), Name: inferenceGraph.GetName()}, &igDeployment)).To(Succeed())
				g.Expect(igDeployment.Spec.Template.Spec.AutomountServiceAccountToken).To(Equal(proto.Bool(true)))
				g.Expect(igDeployment.Spec.Template.Spec.ServiceAccountName).To(Equal(inferenceGraph.ResolveServiceAccountName()))
				g.Expect(igDeployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(igDeployment.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--enable-auth", "--inferencegraph-name", inferenceGraph.GetName()))
			}, timeout, interval).Should(Succeed())
		})

		It("Should delete the ServiceAccount when the InferenceGraph is deleted", func() {
			serviceAccount := corev1.ServiceAccount{}
			saKey := types.NamespacedName{Namespace: inferenceGraph.GetNamespace(), Name: inferenceGraph.ResolveServiceAccountName()}

			Eventually(func() error {
				return k8sClient.Get(ctx, saKey, &serviceAccount)
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, inferenceGraph)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, saKey, &serviceAccount)
			}, timeout, interval).Should(WithTransform(errors.IsNotFound, BeTrue()))
		})

		It("Should remove the ServiceAccount as subject of the ClusterRoleBinding when the InferenceGraph is deleted", func() {
			crbKey := types.NamespacedName{Name: constants.InferenceGraphAuthCRBName}

			Eventually(func() []rbacv1.Subject {
				clusterRoleBinding := rbacv1.ClusterRoleBinding{}
				_ = k8sClient.Get(ctx, crbKey, &clusterRoleBinding)
				return clusterRoleBinding.Subjects
			}, timeout, interval).Should(ContainElement(HaveField("Name", inferenceGraph.ResolveServiceAccountName())))

			Expect(k8sClient.Delete(ctx, inferenceGraph)).To(Succeed())
			Eventually(func() []rbacv1.Subject {
				clusterRoleBinding := rbacv1.ClusterRoleBinding{}
				_ = k8sClient.Get(ctx, crbKey, &clusterRoleBinding)
				return clusterRoleBinding.Subjects
			}, timeout, interval).ShouldNot(ContainElement(HaveField("Name", inferenceGraph.ResolveServiceAccountName())))
		})
	})
})
