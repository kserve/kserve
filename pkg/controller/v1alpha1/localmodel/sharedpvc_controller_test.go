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

package localmodel

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

var _ = Describe("LocalModelNamespaceCache shared-PVC controller", func() {
	const (
		timeout        = time.Second * 10
		duration       = time.Second * 3
		interval       = time.Millisecond * 250
		sourceModelUri = "s3://mybucket/mymodel"
	)

	// makeRWXPVC returns a filesystem RWX PVC with a request larger than the model size.
	makeRWXPVC := func(name, namespace string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				VolumeMode:  ptr.To(corev1.PersistentVolumeFilesystem),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				},
			},
		}
	}

	makeSharedCache := func(name, namespace, pvcRef string) *v1alpha1.LocalModelNamespaceCache {
		return &v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: sourceModelUri,
				ModelSize:      resource.MustParse("1Gi"),
				PVCRef:         ptr.To(pvcRef),
			},
		}
	}

	markJobCondition := func(ctx context.Context, key types.NamespacedName, condType batchv1.JobConditionType) {
		job := &batchv1.Job{}
		Eventually(func() error { return k8sClient.Get(ctx, key, job) }, timeout, interval).Should(Succeed())
		now := metav1.Now()
		job.Status.StartTime = &now
		// Recent Kubernetes Job status validation requires the interim
		// SuccessCriteriaMet/FailureTarget condition before the terminal one.
		var interim batchv1.JobConditionType
		if condType == batchv1.JobComplete {
			job.Status.CompletionTime = &now
			interim = batchv1.JobSuccessCriteriaMet
		} else {
			interim = batchv1.JobFailureTarget
		}
		job.Status.Conditions = []batchv1.JobCondition{
			{Type: interim, Status: corev1.ConditionTrue, LastTransitionTime: now},
			{Type: condType, Status: corev1.ConditionTrue, LastTransitionTime: now},
		}
		Expect(k8sClient.Status().Update(ctx, job)).Should(Succeed())
	}

	Context("When a LocalModelNamespaceCache sets pvcRef", func() {
		It("Should create one import Job, be a no-op on repeat, and not fan out to nodes", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-job-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("shared-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			cache := makeSharedCache("shared-iris", ns, "shared-pvc")
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)

			jobKey := types.NamespacedName{Name: "shared-iris-import", Namespace: ns}
			job := &batchv1.Job{}
			Eventually(func() error { return k8sClient.Get(ctx, jobKey, job) }, timeout, interval).Should(Succeed())

			// Import Job invariants.
			Expect(*job.Spec.Parallelism).To(Equal(int32(1)))
			Expect(*job.Spec.Completions).To(Equal(int32(1)))
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(2)))
			Expect(job.Spec.TTLSecondsAfterFinished).To(BeNil())
			Expect(job.Spec.Template.Spec.NodeSelector).To(BeEmpty())
			Expect(job.OwnerReferences).To(HaveLen(1))
			Expect(job.OwnerReferences[0].Name).To(Equal("shared-iris"))

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(Equal([]string{sourceModelUri, "/mnt/models"}))
			Expect(container.VolumeMounts).To(HaveLen(1))
			storageKey := v1alpha1.GetStorageKey(sourceModelUri)
			Expect(container.VolumeMounts[0].SubPath).To(Equal(filepath.Join("models", storageKey)))
			Expect(container.VolumeMounts[0].ReadOnly).To(BeFalse())
			Expect(job.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("shared-pvc"))

			// No node fan-out: no LocalModelNode references this cache.
			Consistently(func() bool {
				nodes := &v1alpha1.LocalModelNodeList{}
				if err := k8sClient.List(ctx, nodes); err != nil {
					return false
				}
				for _, n := range nodes.Items {
					for _, m := range n.Spec.LocalModels {
						if m.ModelName == "shared-iris" {
							return false
						}
					}
				}
				return true
			}, duration, interval).Should(BeTrue(), "shared-PVC cache must not fan out to LocalModelNodes")

			// Only one import Job exists for this cache.
			Consistently(func() int {
				jobs := &batchv1.JobList{}
				_ = k8sClient.List(ctx, jobs, client.InNamespace(ns))
				return len(jobs.Items)
			}, duration, interval).Should(Equal(1))
		})

		It("Should not trust a foreign Job with the deterministic import name", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-job-collision-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("shared-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			jobKey := types.NamespacedName{Name: "shared-collision-import", Namespace: ns}
			foreignJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobKey.Name, Namespace: jobKey.Namespace},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers:    []corev1.Container{{Name: "foreign", Image: "busybox"}},
					RestartPolicy: corev1.RestartPolicyNever,
				}}},
			}
			Expect(k8sClient.Create(ctx, foreignJob)).Should(Succeed())
			defer k8sClient.Delete(ctx, foreignJob)
			markJobCondition(ctx, jobKey, batchv1.JobComplete)

			cache := makeSharedCache("shared-collision", ns, pvc.Name)
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)
			cacheKey := types.NamespacedName{Name: cache.Name, Namespace: ns}
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, current); err != nil {
					return err
				}
				current.Status.MarkReady(current.Generation)
				return k8sClient.Status().Update(ctx, current)
			})).Should(Succeed())

			Eventually(func() bool {
				current := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, current); err != nil {
					return false
				}
				condition := current.Status.GetCondition(v1alpha1.LocalModelCacheReady)
				return condition != nil && !current.IsReady()
			}, timeout, interval).Should(BeTrue())

			currentJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, jobKey, currentJob)).Should(Succeed())
			Expect(currentJob.OwnerReferences).To(BeEmpty())
		})

		It("Should report PVCNotFound, then progress once the referenced PVC is created", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-pvcwatch-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			cache := makeSharedCache("shared-late-pvc", ns, "late-pvc")
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)

			cacheKey := types.NamespacedName{Name: "shared-late-pvc", Namespace: ns}
			Eventually(func() string {
				c := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, c); err != nil {
					return ""
				}
				if cond := c.Status.GetCondition(v1alpha1.LocalModelCacheReady); cond != nil {
					return cond.Reason
				}
				return ""
			}, timeout, interval).Should(Equal(v1alpha1.ReasonPVCNotFound))

			// Creating the referenced PVC must enqueue the cache and let it create the Job.
			pvc := makeRWXPVC("late-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			jobKey := types.NamespacedName{Name: "shared-late-pvc-import", Namespace: ns}
			Eventually(func() error { return k8sClient.Get(ctx, jobKey, &batchv1.Job{}) }, timeout, interval).Should(Succeed())
		})

		It("Should become Ready with observedGeneration when the import Job completes", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-ready-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("ready-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			cache := makeSharedCache("shared-ready", ns, "ready-pvc")
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)

			cacheKey := types.NamespacedName{Name: "shared-ready", Namespace: ns}
			jobKey := types.NamespacedName{Name: "shared-ready-import", Namespace: ns}

			// Before completion: not Ready, total copy still reported.
			Eventually(func() bool {
				c := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, c); err != nil {
					return false
				}
				return c.Status.ModelCopies != nil && c.Status.ModelCopies.Total == 1 && !c.IsReady()
			}, timeout, interval).Should(BeTrue())

			markJobCondition(ctx, jobKey, batchv1.JobComplete)

			Eventually(func() bool {
				c := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, c); err != nil {
					return false
				}
				cond := c.Status.GetCondition(v1alpha1.LocalModelCacheReady)
				if cond == nil || !c.IsReady() {
					return false
				}
				return cond.ObservedGeneration == c.Generation &&
					c.Status.ModelCopies != nil &&
					c.Status.ModelCopies.Available == 1 &&
					len(c.Status.NodeStatus) == 0
			}, timeout, interval).Should(BeTrue(), "cache should be Ready with observedGeneration and one available copy")
		})

		It("Should re-import when the referenced PVC is recreated with the same name", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-pvc-identity-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("replace-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())

			cache := makeSharedCache("shared-replace", ns, pvc.Name)
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)

			cacheKey := types.NamespacedName{Name: cache.Name, Namespace: ns}
			jobKey := types.NamespacedName{Name: "shared-replace-import", Namespace: ns}
			originalJob := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, jobKey, originalJob)
			}, timeout, interval).Should(Succeed())
			markJobCondition(ctx, jobKey, batchv1.JobComplete)
			Eventually(func() bool {
				current := &v1alpha1.LocalModelNamespaceCache{}
				return k8sClient.Get(ctx, cacheKey, current) == nil && current.IsReady()
			}, timeout, interval).Should(BeTrue())

			// envtest has no PVC-protection controller to clear the admission-added finalizer.
			currentPVC := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: ns}, currentPVC)).Should(Succeed())
			currentPVC.Finalizers = nil
			Expect(k8sClient.Update(ctx, currentPVC)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, currentPVC)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: ns}, &corev1.PersistentVolumeClaim{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			replacementPVC := makeRWXPVC(pvc.Name, ns)
			Expect(k8sClient.Create(ctx, replacementPVC)).Should(Succeed())
			defer k8sClient.Delete(ctx, replacementPVC)
			Expect(replacementPVC.UID).NotTo(Equal(pvc.UID))

			Eventually(func() bool {
				replacementJob := &batchv1.Job{}
				if err := k8sClient.Get(ctx, jobKey, replacementJob); err != nil {
					return false
				}
				return replacementJob.UID != originalJob.UID
			}, timeout, interval).Should(BeTrue(), "a new PVC identity must trigger a new import Job")
		})

		It("Should keep the oldest cache as the sole destination owner", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-owner-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			owner := makeSharedCache("a-owner", ns, "shared-pvc")
			Expect(k8sClient.Create(ctx, owner)).Should(Succeed())
			defer k8sClient.Delete(ctx, owner)

			contender := makeSharedCache("b-contender", ns, "shared-pvc")
			Expect(k8sClient.Create(ctx, contender)).Should(Succeed())
			defer k8sClient.Delete(ctx, contender)

			// Ensure both caches are visible before the PVC event reconciles the destination.
			for _, name := range []string{owner.Name, contender.Name} {
				key := types.NamespacedName{Name: name, Namespace: ns}
				Eventually(func() string {
					cache := &v1alpha1.LocalModelNamespaceCache{}
					if err := k8sClient.Get(ctx, key, cache); err != nil {
						return ""
					}
					condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
					if condition == nil {
						return ""
					}
					return condition.Reason
				}, timeout, interval).Should(Equal(v1alpha1.ReasonPVCNotFound))
			}

			pvc := makeRWXPVC("shared-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			ownerJobKey := types.NamespacedName{Name: "a-owner-import", Namespace: ns}
			ownerJob := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, ownerJobKey, ownerJob)
			}, timeout, interval).Should(Succeed())
			ownerJob.Finalizers = []string{"test.kserve.io/hold-deletion"}
			Expect(k8sClient.Update(ctx, ownerJob)).Should(Succeed())

			Eventually(func() string {
				cache := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: contender.Name, Namespace: ns}, cache); err != nil {
					return ""
				}
				condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
				if condition == nil {
					return ""
				}
				return condition.Reason
			}, timeout, interval).Should(Equal(v1alpha1.ReasonDestinationConflict))

			Consistently(func() int {
				jobs := &batchv1.JobList{}
				_ = k8sClient.List(ctx, jobs, client.InNamespace(ns))
				return len(jobs.Items)
			}, duration, interval).Should(Equal(1))

			Expect(k8sClient.Delete(ctx, owner)).Should(Succeed())
			contenderJobKey := types.NamespacedName{Name: "b-contender-import", Namespace: ns}
			Consistently(func() bool {
				oldJob := &batchv1.Job{}
				if err := k8sClient.Get(ctx, ownerJobKey, oldJob); err != nil {
					return false
				}
				err := k8sClient.Get(ctx, contenderJobKey, &batchv1.Job{})
				return errors.IsNotFound(err)
			}, duration, interval).Should(BeTrue(), "the contender must wait while the old import Job is deleting")

			deletingOwnerJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, ownerJobKey, deletingOwnerJob)).Should(Succeed())
			deletingOwnerJob.Finalizers = nil
			Expect(k8sClient.Update(ctx, deletingOwnerJob)).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, contenderJobKey, &batchv1.Job{})
			}, timeout, interval).Should(Succeed(), "the contender should take ownership after the owner is deleted")
			markJobCondition(ctx, contenderJobKey, batchv1.JobComplete)

			Eventually(func() bool {
				cache := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: contender.Name, Namespace: ns}, cache); err != nil {
					return false
				}
				return cache.IsReady()
			}, timeout, interval).Should(BeTrue())
		})

		It("Should retain a failed Job and create one replacement when it is deleted", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-retry-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("retry-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			cache := makeSharedCache("shared-retry", ns, "retry-pvc")
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())
			defer k8sClient.Delete(ctx, cache)

			cacheKey := types.NamespacedName{Name: "shared-retry", Namespace: ns}
			jobKey := types.NamespacedName{Name: "shared-retry-import", Namespace: ns}

			originalJob := &batchv1.Job{}
			Eventually(func() error { return k8sClient.Get(ctx, jobKey, originalJob) }, timeout, interval).Should(Succeed())

			markJobCondition(ctx, jobKey, batchv1.JobFailed)

			Eventually(func() bool {
				c := &v1alpha1.LocalModelNamespaceCache{}
				if err := k8sClient.Get(ctx, cacheKey, c); err != nil {
					return false
				}
				cond := c.Status.GetCondition(v1alpha1.LocalModelCacheReady)
				return cond != nil && cond.Reason == v1alpha1.ReasonImportFailed &&
					c.Status.ModelCopies != nil && c.Status.ModelCopies.Failed == 1
			}, timeout, interval).Should(BeTrue())

			// Failed Job is retained (no auto-recreate while it exists).
			Consistently(func() string {
				j := &batchv1.Job{}
				if err := k8sClient.Get(ctx, jobKey, j); err != nil {
					return ""
				}
				return string(j.UID)
			}, duration, interval).Should(Equal(string(originalJob.UID)))

			// Explicit retry: delete the failed Job; controller creates exactly one replacement.
			// Background propagation removes the Job object immediately; envtest runs no GC
			// controller, so foreground deletion would leave it terminating forever.
			Expect(k8sClient.Delete(ctx, originalJob, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(Succeed())
			Eventually(func() bool {
				j := &batchv1.Job{}
				if err := k8sClient.Get(ctx, jobKey, j); err != nil {
					return false
				}
				return j.UID != originalJob.UID
			}, timeout, interval).Should(BeTrue(), "a single replacement Job should be created")

			Consistently(func() int {
				jobs := &batchv1.JobList{}
				_ = k8sClient.List(ctx, jobs, client.InNamespace(ns))
				return len(jobs.Items)
			}, duration, interval).Should(Equal(1), "never concurrent import Jobs")
		})

		It("Should delete the cache cleanly and preserve the referenced PVC", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			ns := fmt.Sprintf("test-shared-delete-%d", time.Now().UnixNano())
			defer k8sClient.Delete(ctx, createTestNamespace(ctx, ns))

			pvc := makeRWXPVC("keep-pvc", ns)
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, pvc)

			cache := makeSharedCache("shared-delete", ns, "keep-pvc")
			Expect(k8sClient.Create(ctx, cache)).Should(Succeed())

			cacheKey := types.NamespacedName{Name: "shared-delete", Namespace: ns}
			jobKey := types.NamespacedName{Name: "shared-delete-import", Namespace: ns}
			Eventually(func() error { return k8sClient.Get(ctx, jobKey, &batchv1.Job{}) }, timeout, interval).Should(Succeed())

			// Shared-PVC caches carry no finalizer, so deletion completes immediately.
			Expect(k8sClient.Delete(ctx, cache)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, cacheKey, &v1alpha1.LocalModelNamespaceCache{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// The user-provided PVC is never owned or deleted by the controller.
			Consistently(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "keep-pvc", Namespace: ns}, &corev1.PersistentVolumeClaim{})
			}, duration, interval).Should(Succeed())
		})
	})
})
