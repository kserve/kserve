/*
Copyright 2026 The KServe Authors.

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

package kernelcachecapture

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	timeout  = "30s"
	interval = "500ms"
	testNS   = "test-ns"
)

func newKCC(name string, trigger bool) *v1alpha1.KernelCacheCapture {
	return &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    fmt.Sprintf("registry/cache/%s:v1", name),
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
			Trigger:        trigger,
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(true),
			},
		},
	}
}

func newPodForKCC(name, kccName string, running bool, containerReady bool) *corev1.Pod {
	phase := corev1.PodPending
	if running {
		phase = corev1.PodRunning
	}

	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
			Labels: map[string]string{
				constants.KernelCacheCaptureLabel: kccName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "model-server", Image: "vllm:latest"},
				{Name: "cache-capture", Image: "mcv:latest"},
			},
		},
	}

	if running {
		p.Status = corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "model-server", Ready: true},
				{Name: "cache-capture", Ready: containerReady},
			},
		}
	}

	return p
}

var _ = Describe("KernelCacheCapture Controller Integration", func() {
	BeforeEach(func() {
		mockExecutor.reset()
		// Default: both build and push succeed
		mockExecutor.setResult("/mcv", execResult{
			stdout: "Build complete", exitCode: 0,
		})
		mockExecutor.setResult("buildah", execResult{
			stdout: "Writing manifest to image destination\nsha256:abc123def456", exitCode: 0,
		})
	})

	Context("Full lifecycle: Pending → Capturing → Complete", func() {
		It("Should complete capture when pod is ready and trigger is set", func(ctx SpecContext) {
			kccName := "lifecycle-test"
			kcc := newKCC(kccName, false)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			// Verify phase initializes to Pending
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhasePending))
			}, timeout, interval).Should(Succeed())

			// Verify finalizer was added
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Finalizers).To(ContainElement(KCCFinalizerName))
			}, timeout, interval).Should(Succeed())

			// Create pod with matching label, running, container ready
			pod := newPodForKCC("lifecycle-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			// envtest doesn't run kubelet — we must set pod status ourselves
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Trigger the capture
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				fetched.Spec.Trigger = true
				g.Expect(k8sClient.Update(ctx, fetched)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Verify phase reaches Complete
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhaseComplete))
				g.Expect(fetched.Status.PodName).To(Equal("lifecycle-pod"))
				g.Expect(fetched.Status.ImageDigest).To(Equal("sha256:abc123def456"))
				g.Expect(fetched.Status.CapturedAt).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify mock executor was called with correct commands
			calls := mockExecutor.getCalls()
			Expect(len(calls)).To(BeNumerically(">=", 2))
			Expect(calls[0].cmd[0]).To(Equal("/mcv"))
			Expect(calls[1].cmd[0]).To(Equal("buildah"))
		})
	})

	Context("Auto-creates KernelCache on completion", func() {
		It("Should create KC with ownerReference after successful capture", func(ctx SpecContext) {
			kccName := "autocreate-test"
			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			// Create ready pod
			pod := newPodForKCC("autocreate-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Wait for completion and KC creation
			Eventually(func(g Gomega) {
				kc := &v1alpha1.KernelCache{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, kc)).To(Succeed())
				g.Expect(kc.Spec.Image).To(Equal(fmt.Sprintf("registry/cache/%s:v1", kccName)))
				g.Expect(kc.Spec.MountType).To(Equal(v1alpha1.KernelCacheMountTypeImageVolume))

				// Verify ownerReference
				g.Expect(kc.OwnerReferences).To(HaveLen(1))
				g.Expect(kc.OwnerReferences[0].Name).To(Equal(kccName))
			}, timeout, interval).Should(Succeed())

			// Verify KCC status has KC ref
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.KernelCacheRef).NotTo(BeNil())
				g.Expect(fetched.Status.KernelCacheRef.Name).To(Equal(kccName))
			}, timeout, interval).Should(Succeed())

			// Cleanup KC
			kc := &v1alpha1.KernelCache{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, kc)
			_ = k8sClient.Delete(ctx, kc)
		})
	})

	Context("Capture fails on build error", func() {
		It("Should transition to Failed when MCV build fails", func(ctx SpecContext) {
			kccName := "build-fail-test"
			mockExecutor.setResult("/mcv", execResult{
				stderr: "MCV build error: permission denied", exitCode: 1,
			})

			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			pod := newPodForKCC("build-fail-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhaseFailed))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Capture fails on push error", func() {
		It("Should transition to Failed when buildah push fails", func(ctx SpecContext) {
			kccName := "push-fail-test"
			mockExecutor.setResult("buildah", execResult{
				stderr: "push failed: unauthorized", exitCode: 1,
			})

			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			pod := newPodForKCC("push-fail-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhaseFailed))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("No pod found stays Pending", func() {
		It("Should remain Pending when no matching pod exists", func(ctx SpecContext) {
			kccName := "no-pod-test"
			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			// Wait for phase to initialize to Pending
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhasePending))
			}, timeout, interval).Should(Succeed())

			// No pod created — controller should stay in Pending
			Consistently(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhasePending))
			}, "5s", interval).Should(Succeed())
		})
	})

	Context("Pod not ready stays Pending", func() {
		It("Should remain Pending when pod container is not ready", func(ctx SpecContext) {
			kccName := "not-ready-test"
			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			pod := newPodForKCC("not-ready-pod", kccName, true, false)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: false},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Wait for phase to initialize to Pending
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhasePending))
			}, timeout, interval).Should(Succeed())

			Consistently(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhasePending))
			}, "5s", interval).Should(Succeed())
		})
	})

	Context("Finalizer prevents deletion when KC in use", func() {
		It("Should block KCC deletion while owned KC is referenced by an ISVC", func(ctx SpecContext) {
			kccName := "finalizer-test"

			// Create KCC with trigger, let it complete
			kcc := newKCC(kccName, true)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())

			pod := newPodForKCC("finalizer-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Wait for completion and KC creation
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhaseComplete))
				g.Expect(fetched.Status.KernelCacheRef).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())

			// Create ISVC referencing the auto-created KC
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: testNS,
					Labels: map[string]string{
						constants.KernelCacheLabel: kccName,
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "pytorch"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, isvc) }()

			// Delete KCC — should be blocked by finalizer
			Expect(k8sClient.Delete(ctx, kcc)).Should(Succeed())

			// KCC should still exist (finalizer blocks deletion)
			Consistently(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.DeletionTimestamp).NotTo(BeNil())
				g.Expect(fetched.Finalizers).To(ContainElement(KCCFinalizerName))
			}, "5s", interval).Should(Succeed())

			// Remove the ISVC so finalizer can proceed
			Expect(k8sClient.Delete(ctx, isvc)).Should(Succeed())

			// Now KCC should be fully deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, &v1alpha1.KernelCacheCapture{})
			}, timeout, interval).ShouldNot(Succeed())

			// Cleanup KC (should be deleted by GC, but envtest doesn't run GC)
			kc := &v1alpha1.KernelCache{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, kc); err == nil {
				_ = k8sClient.Delete(ctx, kc)
			}
		})
	})

	Context("CreateKernelCache disabled skips KC creation", func() {
		It("Should complete without creating KC when createKernelCache.enabled=false", func(ctx SpecContext) {
			kccName := "no-kc-test"
			kcc := newKCC(kccName, true)
			kcc.Spec.CreateKernelCache.Enabled = ptr.To(false)
			Expect(k8sClient.Create(ctx, kcc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, kcc) }()

			pod := newPodForKCC("no-kc-pod", kccName, true, true)
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "model-server", Ready: true},
					{Name: "cache-capture", Ready: true},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Wait for completion
			Eventually(func(g Gomega) {
				fetched := &v1alpha1.KernelCacheCapture{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.KernelCacheCapturePhaseComplete))
			}, timeout, interval).Should(Succeed())

			// Verify no KC was created
			kc := &v1alpha1.KernelCache{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: kccName, Namespace: testNS}, kc)
			Expect(client.IgnoreNotFound(err)).To(Succeed())
			if err == nil {
				Fail("KernelCache should not have been created")
			}
		})
	})
})
