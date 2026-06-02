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

package kernelcachenode

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("KernelCacheNode Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	ctx := context.Background()

	Context("When creating a KernelCache", func() {
		var (
			kernelCacheName      string
			kernelCacheNamespace string
			nodeName             string
		)

		BeforeEach(func() {
			kernelCacheName = "test-cache-" + randStringRunes(5)
			kernelCacheNamespace = "default"
			nodeName = "test-node-" + randStringRunes(5)

			// Create a test node
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		})

		AfterEach(func() {
			// Clean up node
			node := &corev1.Node{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err == nil {
				_ = k8sClient.Delete(ctx, node)
			}

			// Clean up KernelCache
			kc := &v1alpha1.KernelCache{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kernelCacheName,
				Namespace: kernelCacheNamespace,
			}, kc); err == nil {
				_ = k8sClient.Delete(ctx, kc)
			}

			// Clean up KernelCacheNode
			kcNode := &v1alpha1.KernelCacheNode{}
			kcNodeName := nodeName
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcNodeName,
				Namespace: kernelCacheNamespace,
			}, kcNode); err == nil {
				_ = k8sClient.Delete(ctx, kcNode)
			}
		})

		It("Should create KernelCacheNode when KernelCache is created", func() {
			Skip("Requires full reconciliation loop - integration test")
			// This test requires the full operator and agent to be running
			// It's better suited for E2E tests rather than unit tests with envtest
		})
	})

	Context("GPU Detection", func() {
		var (
			reconciler *KernelCacheNodeReconciler
			kcNode     *v1alpha1.KernelCacheNode
		)

		BeforeEach(func() {
			reconciler = &KernelCacheNodeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Log:    ctrl.Log.WithName("test"),
			}

			kcNode = &v1alpha1.KernelCacheNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kcnode-" + randStringRunes(5),
					Namespace: constants.KServeNamespace,
				},
				Status: v1alpha1.KernelCacheNodeStatus{
					NodeName: "test-node",
				},
			}
		})

		It("Should populate GPUInfo with stub mode when noGPU=true", func() {
			// Test that GPU detection populates GPUInfo field
			Expect(kcNode.Status.GPUInfo).To(BeEmpty())

			// Call populateGPUInfo with noGPU=true (stub mode)
			err := reconciler.populateGPUInfo(kcNode, true)
			Expect(err).ToNot(HaveOccurred())

			// Verify GPUInfo was populated
			Expect(kcNode.Status.GPUInfo).ToNot(BeEmpty())

			// Verify stub GPU data (2x AMD MI210 as per MCV stub)
			Expect(kcNode.Status.GPUInfo).To(HaveLen(1))
			gpuInfo := kcNode.Status.GPUInfo[0]
			Expect(gpuInfo.GPUType).To(ContainSubstring("MI200"))
			Expect(gpuInfo.IDs).To(HaveLen(2))
			Expect(gpuInfo.DriverVersion).ToNot(BeEmpty())
		})

		It("Should not populate GPUInfo if already populated", func() {
			// Pre-populate GPUInfo
			kcNode.Status.GPUInfo = []v1alpha1.GPUTypeInfo{
				{
					GPUType:       "existing-gpu",
					IDs:           []int{0, 1},
					DriverVersion: "535.0",
					CUDAVersion:   "12.0",
				},
			}

			// Call populateGPUInfo
			err := reconciler.populateGPUInfo(kcNode, true)
			Expect(err).ToNot(HaveOccurred())

			// Verify GPUInfo was NOT changed
			Expect(kcNode.Status.GPUInfo).To(HaveLen(1))
			Expect(kcNode.Status.GPUInfo[0].GPUType).To(Equal("existing-gpu"))
		})

		It("Should handle GPU detection errors gracefully", func() {
			// This tests error handling - MCV should handle invalid state gracefully
			// The populateGPUInfo function logs errors but doesn't fail reconciliation
			err := reconciler.populateGPUInfo(kcNode, false)

			// Even if GPU detection fails (no real GPUs in test environment),
			// the function should not return an error
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("convertMCVToGPUTypeInfo", func() {
		It("Should convert MCV GPUFleetSummary to GPUTypeInfo correctly", func() {
			Skip("Requires MCV types - test MCV integration separately")
			// This is tested indirectly through populateGPUInfo tests above
			// Direct testing would require importing MCV types which complicates the test
		})
	})

	Context("Pod Watching - Helper Functions", func() {
		var reconciler *KernelCacheNodeReconciler

		BeforeEach(func() {
			reconciler = &KernelCacheNodeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Log:    ctrl.Log.WithName("test"),
			}
		})

		Describe("podHasPVCVolume", func() {
			It("Should return true for pod with PVC volume", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "cache-volume",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-cache",
									},
								},
							},
						},
					},
				}
				Expect(podHasPVCVolume(pod)).To(BeTrue())
			})

			It("Should return false for pod without PVC volume", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "my-config",
										},
									},
								},
							},
						},
					},
				}
				Expect(podHasPVCVolume(pod)).To(BeFalse())
			})

			It("Should return false for pod with no volumes", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{},
					},
				}
				Expect(podHasPVCVolume(pod)).To(BeFalse())
			})
		})

		Describe("podMountsCachePVC", func() {
			It("Should return true when pod mounts exact cache name", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "cache-volume",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-cache",
									},
								},
							},
						},
					},
				}
				Expect(reconciler.podMountsCachePVC(pod, "my-cache")).To(BeTrue())
			})

			It("Should return true when pod mounts cache-serving PVC", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "cache-volume",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-cache-serving",
									},
								},
							},
						},
					},
				}
				Expect(reconciler.podMountsCachePVC(pod, "my-cache")).To(BeTrue())
			})

			It("Should return false when pod mounts different cache", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "cache-volume",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "other-cache",
									},
								},
							},
						},
					},
				}
				Expect(reconciler.podMountsCachePVC(pod, "my-cache")).To(BeFalse())
			})

			It("Should return false when pod has no PVC volumes", func() {
				pod := &corev1.Pod{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{},
					},
				}
				Expect(reconciler.podMountsCachePVC(pod, "my-cache")).To(BeFalse())
			})
		})

		Describe("isPodReady", func() {
			It("Should return true for running and ready pod", func() {
				pod := &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}
				Expect(reconciler.isPodReady(pod)).To(BeTrue())
			})

			It("Should return false for running but not ready pod", func() {
				pod := &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}
				Expect(reconciler.isPodReady(pod)).To(BeFalse())
			})

			It("Should return false for pending pod", func() {
				pod := &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				}
				Expect(reconciler.isPodReady(pod)).To(BeFalse())
			})

			It("Should return false for succeeded pod", func() {
				pod := &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				}
				Expect(reconciler.isPodReady(pod)).To(BeFalse())
			})
		})

		Describe("servingNamespacesEqual", func() {
			It("Should return true for equal maps", func() {
				a := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
					"ns2": {PodsUsing: 1, PodsReady: 1, PodsTerminating: 0},
				}
				b := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
					"ns2": {PodsUsing: 1, PodsReady: 1, PodsTerminating: 0},
				}
				Expect(servingNamespacesEqual(a, b)).To(BeTrue())
			})

			It("Should return false for different pod counts", func() {
				a := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
				}
				b := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 2, PodsReady: 2, PodsTerminating: 0},
				}
				Expect(servingNamespacesEqual(a, b)).To(BeFalse())
			})

			It("Should return false for different namespaces", func() {
				a := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
				}
				b := map[string]v1alpha1.NamespaceServingCounts{
					"ns2": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
				}
				Expect(servingNamespacesEqual(a, b)).To(BeFalse())
			})

			It("Should return false for different map sizes", func() {
				a := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
					"ns2": {PodsUsing: 1, PodsReady: 1, PodsTerminating: 0},
				}
				b := map[string]v1alpha1.NamespaceServingCounts{
					"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 0},
				}
				Expect(servingNamespacesEqual(a, b)).To(BeFalse())
			})

			It("Should return true for empty maps", func() {
				a := map[string]v1alpha1.NamespaceServingCounts{}
				b := map[string]v1alpha1.NamespaceServingCounts{}
				Expect(servingNamespacesEqual(a, b)).To(BeTrue())
			})
		})

		Describe("calculateAggregateCounts", func() {
			It("Should aggregate counts across multiple caches", func() {
				reconciler := &KernelCacheNodeReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Log:    ctrl.Log.WithName("test"),
				}

				kcNode := &v1alpha1.KernelCacheNode{
					Status: v1alpha1.KernelCacheNodeStatus{
						CacheStatus: map[string]v1alpha1.CacheNodeExtractionStatus{
							"cache1": {
								State: v1alpha1.NodeCacheStateRunning,
								ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{
									"ns1": {PodsUsing: 2, PodsReady: 2, PodsTerminating: 0},
									"ns2": {PodsUsing: 1, PodsReady: 1, PodsTerminating: 0},
								},
							},
							"cache2": {
								State: v1alpha1.NodeCacheStateExtracted,
								ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{},
							},
							"cache3": {
								State: v1alpha1.NodeCacheStateError,
								ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{},
							},
						},
					},
				}

				counts := reconciler.calculateAggregateCounts(kcNode)
				Expect(counts.CachesInUse).To(Equal(1))
				Expect(counts.CachesNotInUse).To(Equal(1))
				Expect(counts.CachesError).To(Equal(1))
				Expect(counts.PodRunningCnt).To(Equal(3)) // 2 + 1
				Expect(counts.PodDeletingCnt).To(Equal(0))
			})

			It("Should count terminating pods correctly", func() {
				reconciler := &KernelCacheNodeReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Log:    ctrl.Log.WithName("test"),
				}

				kcNode := &v1alpha1.KernelCacheNode{
					Status: v1alpha1.KernelCacheNodeStatus{
						CacheStatus: map[string]v1alpha1.CacheNodeExtractionStatus{
							"cache1": {
								State: v1alpha1.NodeCacheStateRunning,
								ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{
									"ns1": {PodsUsing: 3, PodsReady: 2, PodsTerminating: 1},
								},
							},
						},
					},
				}

				counts := reconciler.calculateAggregateCounts(kcNode)
				Expect(counts.PodRunningCnt).To(Equal(3))
				Expect(counts.PodDeletingCnt).To(Equal(1))
			})

			It("Should return zero counts for empty CacheStatus", func() {
				reconciler := &KernelCacheNodeReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Log:    ctrl.Log.WithName("test"),
				}

				kcNode := &v1alpha1.KernelCacheNode{
					Status: v1alpha1.KernelCacheNodeStatus{
						CacheStatus: map[string]v1alpha1.CacheNodeExtractionStatus{},
					},
				}

				counts := reconciler.calculateAggregateCounts(kcNode)
				Expect(counts.CachesInUse).To(Equal(0))
				Expect(counts.CachesNotInUse).To(Equal(0))
				Expect(counts.CachesError).To(Equal(0))
				Expect(counts.PodRunningCnt).To(Equal(0))
				Expect(counts.PodDeletingCnt).To(Equal(0))
			})
		})

		Describe("nodeCountsEqual", func() {
			It("Should return true for equal counts", func() {
				a := &v1alpha1.NodeCacheCounts{
					CachesInUse:    1,
					CachesNotInUse: 2,
					CachesError:    0,
					PodRunningCnt:  3,
					PodDeletingCnt: 0,
				}
				b := &v1alpha1.NodeCacheCounts{
					CachesInUse:    1,
					CachesNotInUse: 2,
					CachesError:    0,
					PodRunningCnt:  3,
					PodDeletingCnt: 0,
				}
				Expect(nodeCountsEqual(a, b)).To(BeTrue())
			})

			It("Should return false for different counts", func() {
				a := &v1alpha1.NodeCacheCounts{
					CachesInUse:    1,
					CachesNotInUse: 2,
					CachesError:    0,
					PodRunningCnt:  3,
					PodDeletingCnt: 0,
				}
				b := &v1alpha1.NodeCacheCounts{
					CachesInUse:    2,
					CachesNotInUse: 2,
					CachesError:    0,
					PodRunningCnt:  3,
					PodDeletingCnt: 0,
				}
				Expect(nodeCountsEqual(a, b)).To(BeFalse())
			})

			It("Should return true for both nil", func() {
				Expect(nodeCountsEqual(nil, nil)).To(BeTrue())
			})

			It("Should return false for one nil", func() {
				a := &v1alpha1.NodeCacheCounts{
					CachesInUse: 1,
				}
				Expect(nodeCountsEqual(a, nil)).To(BeFalse())
				Expect(nodeCountsEqual(nil, a)).To(BeFalse())
			})
		})
	})

	// NOTE: State transition tests require the full controller with reconciliation loops.
	// These are integration tests and belong in test/e2e/kernelcache/test_kernelcache_pod_watching.py
	// Unit tests above cover the helper functions that support state transitions.
})

// Helper function to generate random strings for test resource names
func randStringRunes(n int) string {
	const letterRunes = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterRunes[time.Now().UnixNano()%int64(len(letterRunes))]
		time.Sleep(time.Nanosecond) // Ensure different values
	}
	return string(b)
}
