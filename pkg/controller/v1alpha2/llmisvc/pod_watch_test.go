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

package llmisvc_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// predicateWithCacheSelector wraps the pod init-containers predicate with the same
// cache logic as the real llmisvc controller (cmd/llmisvc main.go): only pods
// matching ChildResourcesLabelSelector are in the cache, and the mapper only
// enqueues when the pod has a non-empty name label. Using this in tests
// exercises the combined "cache + predicate" behavior so filtering by labels
// is tested as part of the same pipeline as the real controller.
func predicateWithCacheSelector() predicate.Funcs {
	sel, err := metav1.LabelSelectorAsSelector(&llmisvc.ChildResourcesLabelSelector)
	if err != nil {
		panic(err)
	}
	inner := llmisvc.PodInitContainersPredicate()
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newPod, ok := e.ObjectNew.(*corev1.Pod)
			if !ok || newPod == nil {
				return false
			}
			if !sel.Matches(labels.Set(newPod.Labels)) {
				return false
			}
			if newPod.Labels[constants.KubernetesAppNameLabelKey] == "" {
				return false
			}
			return inner.Update(e)
		},
		CreateFunc:  inner.Create,
		DeleteFunc:  inner.Delete,
		GenericFunc: inner.Generic,
	}
}

var _ = Describe("Pod InitContainers Watch", func() {
	// Test the mapper function that maps pods to LLMInferenceService reconcile requests
	Describe("PodInitContainersFunc", func() {
		var reconciler *llmisvc.LLMISVCReconciler

		BeforeEach(func() {
			// Note: Client is not needed for the PodInitContainersFunc mapper
			// as it only reads labels from the pod object passed directly
			reconciler = &llmisvc.LLMISVCReconciler{}
		})

		Context("when pod has the LLMInferenceService labels", func() {
			It("should return a reconcile request for the owning LLMInferenceService", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
							constants.KubernetesAppNameLabelKey: "my-llmisvc",
						},
					},
				}

				requests := reconciler.PodInitContainersFunc(context.Background(), pod)

				Expect(requests).To(HaveLen(1))
				Expect(requests[0]).To(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "my-llmisvc",
					},
				}))
			})

			It("should use the correct namespace from the pod", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "custom-namespace",
						Labels: map[string]string{
							constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
							constants.KubernetesAppNameLabelKey: "my-llmisvc",
						},
					},
				}

				requests := reconciler.PodInitContainersFunc(context.Background(), pod)

				Expect(requests).To(HaveLen(1))
				Expect(requests[0].NamespacedName.Namespace).To(Equal("custom-namespace"))
				Expect(requests[0].NamespacedName.Name).To(Equal("my-llmisvc"))
			})
		})

		Context("when pod does not have the LLMInferenceService labels", func() {
			It("should return nil for pods with empty name label value", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels: map[string]string{
							constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
							constants.KubernetesAppNameLabelKey: "",
						},
					},
				}

				requests := reconciler.PodInitContainersFunc(context.Background(), pod)

				Expect(requests).To(BeNil())
			})

			It("should return nil for pods with nil labels", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				}

				requests := reconciler.PodInitContainersFunc(context.Background(), pod)

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

				requests := reconciler.PodInitContainersFunc(context.Background(), configMap)

				Expect(requests).To(BeNil())
			})

			It("should return nil for nil object", func() {
				requests := reconciler.PodInitContainersFunc(context.Background(), nil)

				Expect(requests).To(BeNil())
			})
		})
	})

	// Test the predicate that filters pod updates. Uses predicateWithCacheSelector
	// so the test controller has the same cache logic as the real llmisvc controller.
	Describe("PodInitContainersPredicate", func() {
		var pred predicate.Funcs

		BeforeEach(func() {
			pred = predicateWithCacheSelector()
		})

		Describe("UpdateFunc", func() {
			Context("when pod has LLMInferenceService labels and InitContainerStatuses change", func() {
				It("should return true when InitContainerStatuses change from empty to waiting", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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

			Context("when InitContainerStatuses don't change", func() {
				It("should return false when only container statuses change", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "kserve-container",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{},
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "kserve-container",
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

					Expect(result).To(BeFalse())
				})

				It("should return false when InitContainerStatuses are identical", func() {
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
						ObjectOld: pod,
						ObjectNew: pod.DeepCopy(),
					})

					Expect(result).To(BeFalse())
				})
			})

			Context("when pod does not have LLMInferenceService labels", func() {
				It("should return false for pods without part-of label", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
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
								constants.KubernetesAppNameLabelKey: "my-llmisvc",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{},
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

				It("should return false for pods with empty name label", func() {
					oldPod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
							Labels: map[string]string{
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "",
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
								constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
								constants.KubernetesAppNameLabelKey: "",
							},
						},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "storage-initializer",
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{},
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
		})
	})

	// Integration tests for pod isolation
	Describe("Pod Isolation", func() {
		Context("when multiple LLMInferenceServices exist", func() {
			It("should only trigger reconcile for the owning LLMInferenceService", func() {
				reconciler := &llmisvc.LLMISVCReconciler{}

				pod1 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-for-llmisvc1",
						Namespace: "default",
						Labels: map[string]string{
							constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
							constants.KubernetesAppNameLabelKey: "llmisvc1",
						},
					},
				}

				pod2 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-for-llmisvc2",
						Namespace: "default",
						Labels: map[string]string{
							constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
							constants.KubernetesAppNameLabelKey: "llmisvc2",
						},
					},
				}

				// Pod1 change should only reconcile llmisvc1
				requests1 := reconciler.PodInitContainersFunc(context.Background(), pod1)
				Expect(requests1).To(HaveLen(1))
				Expect(requests1[0].Name).To(Equal("llmisvc1"))

				// Pod2 change should only reconcile llmisvc2
				requests2 := reconciler.PodInitContainersFunc(context.Background(), pod2)
				Expect(requests2).To(HaveLen(1))
				Expect(requests2[0].Name).To(Equal("llmisvc2"))
			})
		})

		Context("when pod is not managed by any LLMInferenceService", func() {
			It("should not trigger any reconciliation", func() {
				reconciler := &llmisvc.LLMISVCReconciler{}

				// A regular pod without the LLMInferenceService labels
				regularPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app": "some-other-app",
						},
					},
				}

				requests := reconciler.PodInitContainersFunc(context.Background(), regularPod)
				Expect(requests).To(BeNil())
			})
		})
	})
})
