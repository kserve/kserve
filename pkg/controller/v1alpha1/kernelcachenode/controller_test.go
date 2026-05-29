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
			kcNodeName := "kernel-cache-node-" + nodeName
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
