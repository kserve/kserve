/*
Copyright 2025 The KServe Authors.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativeapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("Pod InitContainers Watch", func() {
	// Test the mapper function that maps pods to InferenceService reconcile requests
	Describe("podInitContainersFunc", func() {
		var reconciler *InferenceServiceReconciler

		BeforeEach(func() {
			// Note: Client is not needed for the podInitContainersFunc mapper
			// as it only reads labels from the pod object passed directly
			reconciler = &InferenceServiceReconciler{}
		})

		Context("when pod has the InferenceService label", func() {
			It("should return a reconcile request for the owning InferenceService", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.InferenceServicePodLabelKey: "my-isvc",
						},
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), pod)

				Expect(requests).To(HaveLen(1))
				Expect(requests[0]).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "my-isvc",
					},
				}))
			})

			It("should use the correct namespace from the pod", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "custom-namespace",
						Labels: map[string]string{
							constants.InferenceServicePodLabelKey: "my-isvc",
						},
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), pod)

				Expect(requests).To(HaveLen(1))
				Expect(requests[0].NamespacedName.Namespace).To(Equal("custom-namespace"))
				Expect(requests[0].NamespacedName.Name).To(Equal("my-isvc"))
			})
		})

		Context("when pod does not have the InferenceService label", func() {
			It("should return nil for pods without the label", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels:    map[string]string{},
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), pod)

				Expect(requests).To(BeNil())
			})

			It("should return nil for pods with empty label value", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.InferenceServicePodLabelKey: "",
						},
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), pod)

				Expect(requests).To(BeNil())
			})

			It("should return nil for pods with nil labels", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), pod)

				Expect(requests).To(BeNil())
			})
		})

		Context("when object is not a pod", func() {
			It("should return nil for non-pod objects", func() {
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-configmap",
						Namespace: "default",
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), configMap)

				Expect(requests).To(BeNil())
			})

			It("should return nil for nil object", func() {
				requests := reconciler.podInitContainersFunc(context.Background(), nil)

				Expect(requests).To(BeNil())
			})
		})
	})

	// Test the predicate that filters pod updates
	Describe("podInitContainersPredicate", func() {
		var pred predicate.Funcs

		BeforeEach(func() {
			pred = podInitContainersPredicate()
		})

		Describe("UpdateFunc", func() {
			Context("when pod has InferenceService label and InitContainerStatuses change", func() {
				It("should return true when InitContainerStatuses change from empty to waiting", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{
											Reason:  "PodInitializing",
											Message: "Initializing",
										},
									},
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeTrue())
				})

				It("should return true when InitContainerStatuses change from waiting to terminated with error", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{
											Reason: "PodInitializing",
										},
									},
								},
							},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   "Error",
											Message:  "Failed to download model: certificate verify failed",
										},
									},
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeTrue())
				})

				It("should return true when InitContainerStatuses change from waiting to running", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{
											Reason: "PodInitializing",
										},
									},
								},
							},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Running: &corev1.ContainerStateRunning{},
									},
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeTrue())
				})
			})

			Context("when InitContainerStatuses do not change", func() {
				It("should return false when only other status fields change", func() {
					initStatus := []corev1.ContainerStatus{
						{
							Name: "storage-initializer",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "PodInitializing",
								},
							},
						},
					}

					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							Phase:                 corev1.PodPending,
							InitContainerStatuses: initStatus,
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							Phase:                 corev1.PodRunning, // Phase changed
							InitContainerStatuses: initStatus,        // But InitContainerStatuses unchanged
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeFalse())
				})

				It("should return false when only ContainerStatuses change (not InitContainerStatuses)", func() {
					// This is critical for preventing event storms - main containers constantly
					// update their status but we only care about init container changes
					initStatus := []corev1.ContainerStatus{
						{
							Name: "storage-initializer",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 0,
								},
							},
						},
					}

					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: initStatus,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:         "kserve-container",
									Ready:        false,
									RestartCount: 0,
								},
							},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: initStatus, // Unchanged
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:         "kserve-container",
									Ready:        true, // Changed
									RestartCount: 1,    // Changed
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeFalse())
				})

				It("should return false when InitContainerStatuses are identical", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.InferenceServicePodLabelKey: "my-isvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 0,
										},
									},
								},
							},
						},
					}

					newPod := oldPod.DeepCopy()

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeFalse())
				})
			})

			Context("when pod does not have InferenceService label", func() {
				It("should return false for pods without the label", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unrelated-pod",
							Namespace: "default",
							Labels:    map[string]string{},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unrelated-pod",
							Namespace: "default",
							Labels:    map[string]string{},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "init",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
										},
									},
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeFalse())
				})

				It("should return false for pods with other labels but not InferenceService label", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-pod",
							Namespace: "default",
							Labels: map[string]string{
								"app": "some-other-app",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{},
						},
					}

					newPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-pod",
							Namespace: "default",
							Labels: map[string]string{
								"app": "some-other-app",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "init",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
										},
									},
								},
							},
						},
					}

					result := pred.Update(event.UpdateEvent{
						ObjectOld: oldPod,
						ObjectNew: newPod,
					})

					Expect(result).To(BeFalse())
				})
			})

			Context("when object is not a pod", func() {
				It("should return false for non-pod objects", func() {
					result := pred.Update(event.UpdateEvent{
						ObjectOld: &corev1.ConfigMap{},
						ObjectNew: &corev1.ConfigMap{},
					})

					Expect(result).To(BeFalse())
				})
			})
		})
	})

	// Integration-style tests that verify the mapper doesn't cause "event storms"
	Describe("Event Storm Prevention", func() {
		Context("when multiple pods exist for different InferenceServices", func() {
			It("should only return reconcile request for the specific InferenceService", func() {
				reconciler := &InferenceServiceReconciler{}

				pod1 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "isvc1-predictor-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.InferenceServicePodLabelKey: "isvc1",
						},
					},
				}

				pod2 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "isvc2-predictor-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.InferenceServicePodLabelKey: "isvc2",
						},
					},
				}

				// Pod1 change should only reconcile isvc1
				requests1 := reconciler.podInitContainersFunc(context.Background(), pod1)
				Expect(requests1).To(HaveLen(1))
				Expect(requests1[0].Name).To(Equal("isvc1"))

				// Pod2 change should only reconcile isvc2
				requests2 := reconciler.podInitContainersFunc(context.Background(), pod2)
				Expect(requests2).To(HaveLen(1))
				Expect(requests2[0].Name).To(Equal("isvc2"))
			})
		})

		Context("when pod is not managed by any InferenceService", func() {
			It("should not trigger any reconciliation", func() {
				reconciler := &InferenceServiceReconciler{}

				// A regular pod without the InferenceService label
				regularPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app": "some-other-app",
						},
					},
				}

				requests := reconciler.podInitContainersFunc(context.Background(), regularPod)
				Expect(requests).To(BeNil())
			})
		})
	})
})

var _ = Describe("ServingRuntime Watch", func() {
	var reconciler *InferenceServiceReconciler
	var testNamespace string

	BeforeEach(func() {
		testNamespace = "runtime-watch-test"
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		err := k8sClient.Create(context.Background(), ns)
		if err != nil {
			// Namespace might already exist, ignore error
			_ = k8sClient.Get(context.Background(), types.NamespacedName{Name: testNamespace}, ns)
		}

		reconciler = &InferenceServiceReconciler{
			Client: k8sClient,
		}
	})

	AfterEach(func() {
		// Clean up ISVCs created during tests
		var isvcList v1beta1.InferenceServiceList
		_ = k8sClient.List(context.Background(), &isvcList, client.InNamespace(testNamespace))
		for _, isvc := range isvcList.Items {
			_ = k8sClient.Delete(context.Background(), &isvc)
		}
	})

	// Describe("clusterServingRuntimeFunc", func() {
	//	It("should only reconcile ISVCs that use the specific ClusterServingRuntime", func() {
	//		// Create ISVC using clusterRuntime1
	//		isvc1 := &v1beta1.InferenceService{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name:      "isvc-cluster-runtime-1",
	//				Namespace: testNamespace,
	//			},
	//			Spec: v1beta1.InferenceServiceSpec{
	//				Predictor: v1beta1.PredictorSpec{
	//					SKLearn: &v1beta1.SKLearnSpec{},
	//				},
	//			},
	//		}
	//		Expect(k8sClient.Create(context.Background(), isvc1)).To(Succeed())
	//
	//		// Set the ClusterServingRuntimeName in status
	//		isvc1.Status.ClusterServingRuntimeName = "cluster-runtime-1"
	//		Expect(k8sClient.Status().Update(context.Background(), isvc1)).To(Succeed())
	//
	//		// Create ISVC using clusterRuntime2
	//		isvc2 := &v1beta1.InferenceService{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name:      "isvc-cluster-runtime-2",
	//				Namespace: testNamespace,
	//			},
	//			Spec: v1beta1.InferenceServiceSpec{
	//				Predictor: v1beta1.PredictorSpec{
	//					SKLearn: &v1beta1.SKLearnSpec{},
	//				},
	//			},
	//		}
	//		Expect(k8sClient.Create(context.Background(), isvc2)).To(Succeed())
	//
	//		// Set the ClusterServingRuntimeName in status
	//		isvc2.Status.ClusterServingRuntimeName = "cluster-runtime-2"
	//		Expect(k8sClient.Status().Update(context.Background(), isvc2)).To(Succeed())
	//
	//		// Create a ClusterServingRuntime object (only need metadata for the mapper)
	//		csr := &v1alpha1.ClusterServingRuntime{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: "cluster-runtime-1",
	//			},
	//		}
	//
	//		// Wait for the cache to sync and verify the mapper returns the correct request.
	//		// The cached client may not immediately reflect status updates.
	//		Eventually(func() []reconcile.Request {
	//			return reconciler.clusterServingRuntimeFunc(context.Background(), csr)
	//		}).Should(HaveLen(1))
	//
	//		requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)
	//		Expect(requests[0].Name).To(Equal("isvc-cluster-runtime-1"))
	//		Expect(requests[0].Namespace).To(Equal(testNamespace))
	//	})
	//
	//	It("should not reconcile ISVCs that use a different ClusterServingRuntime", func() {
	//		// Create ISVC using a different runtime
	//		isvc := &v1beta1.InferenceService{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name:      "isvc-different-runtime",
	//				Namespace: testNamespace,
	//			},
	//			Spec: v1beta1.InferenceServiceSpec{
	//				Predictor: v1beta1.PredictorSpec{
	//					SKLearn: &v1beta1.SKLearnSpec{},
	//				},
	//			},
	//		}
	//		Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())
	//
	//		// Set the ClusterServingRuntimeName in status to a different runtime
	//		isvc.Status.ClusterServingRuntimeName = "cluster-runtime-other"
	//		Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())
	//
	//		// Create a ClusterServingRuntime object with a unique name not used by any ISVC
	//		csr := &v1alpha1.ClusterServingRuntime{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: "cluster-runtime-unused",
	//			},
	//		}
	//
	//		// Call the mapper function
	//		requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)
	//
	//		// Should return empty since no ISVC uses cluster-runtime-unused
	//		Expect(requests).To(BeEmpty())
	//	})
	//
	//	It("should not reconcile ISVCs with auto-update disabled when ready", func() {
	//		// Create ISVC with auto-update disabled
	//		isvc := &v1beta1.InferenceService{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name:      "isvc-auto-update-disabled",
	//				Namespace: testNamespace,
	//				Annotations: map[string]string{
	//					constants.DisableAutoUpdateAnnotationKey: "true",
	//				},
	//			},
	//			Spec: v1beta1.InferenceServiceSpec{
	//				Predictor: v1beta1.PredictorSpec{
	//					SKLearn: &v1beta1.SKLearnSpec{},
	//				},
	//			},
	//		}
	//		Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())
	//
	//		// Set the ClusterServingRuntimeName and make it ready
	//		isvc.Status.ClusterServingRuntimeName = "cluster-runtime-auto-update"
	//		isvc.Status.SetCondition(v1beta1.PredictorReady, &knativeapis.Condition{
	//			Type:   v1beta1.PredictorReady,
	//			Status: corev1.ConditionTrue,
	//		})
	//		isvc.Status.SetCondition(v1beta1.IngressReady, &knativeapis.Condition{
	//			Type:   v1beta1.IngressReady,
	//			Status: corev1.ConditionTrue,
	//		})
	//		Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())
	//
	//		// Create a ClusterServingRuntime object
	//		csr := &v1alpha1.ClusterServingRuntime{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: "cluster-runtime-auto-update",
	//			},
	//		}
	//
	//		// Call the mapper function
	//		requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)
	//
	//		// Should not reconcile the ISVC because auto-update is disabled and it's ready
	//		Expect(requests).To(BeEmpty())
	//	})
	// })

	Describe("servingRuntimeFunc", func() {
		It("should only reconcile ISVCs that use the specific ServingRuntime", func() {
			// Create ISVC using runtime1
			isvc1 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-serving-runtime-1",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc1)).To(Succeed())

			// Set the ServingRuntimeName in status
			isvc1.Status.ServingRuntimeName = "serving-runtime-1"
			Expect(k8sClient.Status().Update(context.Background(), isvc1)).To(Succeed())

			// Create ISVC using runtime2
			isvc2 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-serving-runtime-2",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc2)).To(Succeed())

			// Set the ServingRuntimeName in status
			isvc2.Status.ServingRuntimeName = "serving-runtime-2"
			Expect(k8sClient.Status().Update(context.Background(), isvc2)).To(Succeed())

			// Create a ServingRuntime object
			sr := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-runtime-1",
					Namespace: testNamespace,
				},
			}

			// Wait for the cache to sync and verify the mapper returns the correct request.
			// The cached client may not immediately reflect status updates.
			Eventually(func() []reconcile.Request {
				return reconciler.servingRuntimeFunc(context.Background(), sr)
			}).Should(HaveLen(1))

			requests := reconciler.servingRuntimeFunc(context.Background(), sr)
			Expect(requests[0].Name).To(Equal("isvc-serving-runtime-1"))
			Expect(requests[0].Namespace).To(Equal(testNamespace))
		})

		It("should not reconcile ISVCs that use a different ServingRuntime", func() {
			// Create ISVC using a different runtime
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-different-serving-runtime",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())

			// Set the ServingRuntimeName in status to a different runtime
			isvc.Status.ServingRuntimeName = "serving-runtime-other"
			Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())

			// Create a ServingRuntime object with a unique name not used by any ISVC
			sr := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-runtime-unused",
					Namespace: testNamespace,
				},
			}

			// Call the mapper function
			requests := reconciler.servingRuntimeFunc(context.Background(), sr)

			// Should return empty since no ISVC uses serving-runtime-unused
			Expect(requests).To(BeEmpty())
		})

		It("should not reconcile ISVCs with auto-update disabled when ready", func() {
			// Create ISVC with auto-update disabled
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-serving-auto-update-disabled",
					Namespace: testNamespace,
					Annotations: map[string]string{
						constants.DisableAutoUpdateAnnotationKey: "true",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())

			// Set the ServingRuntimeName and make it ready
			isvc.Status.ServingRuntimeName = "serving-runtime-auto-update"
			isvc.Status.SetCondition(v1beta1.PredictorReady, &knativeapis.Condition{
				Type:   v1beta1.PredictorReady,
				Status: corev1.ConditionTrue,
			})
			isvc.Status.SetCondition(v1beta1.IngressReady, &knativeapis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionTrue,
			})
			Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())

			// Create a ServingRuntime object
			sr := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-runtime-auto-update",
					Namespace: testNamespace,
				},
			}

			// Call the mapper function
			requests := reconciler.servingRuntimeFunc(context.Background(), sr)

			// Should not reconcile the ISVC because auto-update is disabled and it's ready
			Expect(requests).To(BeEmpty())
		})
	})
})
