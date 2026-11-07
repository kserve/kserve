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

package llmisvc_test

import (
	"context"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller - PVC evictor cleanup on deletion", func() {
	const (
		pvcName               = "shared-kv-cache-pvc"
		evictorServiceAccount = "kserve-llm-isvc-kv-pvc-evictor"
		skippedReason         = "PVCEvictorCleanupSkipped"
	)
	evictorLabels := client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"}

	createPVC := func(ctx context.Context, namespace string) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		}
		Expect(envTest.Create(ctx, pvc)).To(Succeed())
		DeferCleanup(func(ctx context.Context) { _ = envTest.Delete(ctx, pvc) })
	}

	service := func(name, namespace string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
		return LLMInferenceService(name, append([]LLMInferenceServiceOption{
			InNamespace[*v1alpha2.LLMInferenceService](namespace),
			WithModelURI("pvc://facebook-models/opt-125m"),
			WithModelName("foo"),
		}, opts...)...)
	}

	withSharedPVCRef := func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: pvcName, Path: "kv/"}},
			}}},
		}
	}

	create := func(ctx context.Context, testNs *TestNamespace, llmSvc *v1alpha2.LLMInferenceService) {
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		DeferCleanup(func(ctx context.Context) { testNs.DeleteAndWait(ctx, llmSvc) })
	}

	// The finalizer is what makes deletion wait for the cleanup, so a spec that
	// deletes a service before the controller added it tests nothing.
	waitForFinalizer := func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) {
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Finalizers).To(ContainElement("serving.kserve.io/llmisvc-finalizer"))
		}).WithContext(ctx).Should(Succeed())
	}

	sharedEvictor := func(ctx context.Context, namespace string) *appsv1.Deployment {
		evictor := &appsv1.Deployment{}
		Eventually(func(g Gomega, ctx context.Context) {
			evictors := &appsv1.DeploymentList{}
			g.Expect(envTest.List(ctx, evictors, client.InNamespace(namespace), evictorLabels)).To(Succeed())
			g.Expect(evictors.Items).To(HaveLen(1))
			*evictor = evictors.Items[0]
			g.Expect(envTest.Get(ctx, types.NamespacedName{Name: evictorServiceAccount, Namespace: namespace}, &corev1.ServiceAccount{})).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
		return evictor
	}

	expectEvictorRunning := func(ctx context.Context, evictor *appsv1.Deployment) {
		GinkgoHelper()
		current := &appsv1.Deployment{}
		Expect(envTest.Get(ctx, client.ObjectKeyFromObject(evictor), current)).To(Succeed())
		Expect(current.DeletionTimestamp).To(BeNil(), "shared evictor is being deleted")
	}

	expectNoEvictorLeft := func(ctx context.Context, namespace string) {
		GinkgoHelper()
		Eventually(func(g Gomega, ctx context.Context) {
			evictors := &appsv1.DeploymentList{}
			g.Expect(envTest.List(ctx, evictors, client.InNamespace(namespace), evictorLabels)).To(Succeed())
			for i := range evictors.Items {
				// envtest runs no garbage collector to finish the foreground deletion.
				evictor := &evictors.Items[i]
				if !evictor.DeletionTimestamp.IsZero() && controllerutil.RemoveFinalizer(evictor, metav1.FinalizerDeleteDependents) {
					g.Expect(client.IgnoreNotFound(envTest.Update(ctx, evictor))).To(Succeed())
				}
			}
			g.Expect(evictors.Items).To(BeEmpty(), "shared evictor Deployments left behind")
			err := envTest.Get(ctx, types.NamespacedName{Name: evictorServiceAccount, Namespace: namespace}, &corev1.ServiceAccount{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "PVC evictor ServiceAccount: %v", err)
		}).WithContext(ctx).Should(Succeed())
	}

	expectCleanupSkipped := func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) {
		GinkgoHelper()
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(findEvent(ctx, envTest.Client, llmSvc, skippedReason)).ToNot(BeNil())
		}).WithContext(ctx).Should(Succeed())
	}

	It("should finish deleting services it cannot resolve and leave the shared evictor to the services that remain", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		createPVC(ctx, testNs.Name)

		// The shipped preset carries the pvc-evictor container.
		adopted := service("adopted", testNs.Name, withSharedPVCRef)
		create(ctx, testNs, adopted)
		evictor := sharedEvictor(ctx, testNs.Name)
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(adopted), current)).To(Succeed())
			g.Expect(current.Status.GetCondition(v1alpha2.PVCEvictorReady)).ToNot(BeNil())
		}).WithContext(ctx).Should(Succeed())

		// Admission does not check that baseRefs exist.
		typo := service("typo", testNs.Name, WithBaseRefs(corev1.LocalObjectReference{Name: "missing-config"}))
		create(ctx, testNs, typo)
		waitForFinalizer(ctx, typo)

		// when
		testNs.DeleteAndWait(ctx, typo)

		// then
		expectEvictorRunning(ctx, evictor)

		// given
		unresolved := service("unresolved", testNs.Name, WithBaseRefs(corev1.LocalObjectReference{Name: "config-created-later"}))
		create(ctx, testNs, unresolved)
		waitForFinalizer(ctx, unresolved)

		// when
		testNs.DeleteAndWait(ctx, adopted)

		// then
		expectCleanupSkipped(ctx, adopted)
		// Events are recorded in the order they are emitted, so with the later one in,
		// one for typo would be too. Its cleanup ran and kept the evictor adopted used.
		Expect(findEvent(ctx, envTest.Client, typo, skippedReason)).To(BeNil(), "typo's deletion skipped the cleanup")
		expectEvictorRunning(ctx, evictor)

		// when
		Expect(envTest.Create(ctx, LLMInferenceServiceConfig("config-created-later",
			InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name)))).To(Succeed())

		// then
		expectNoEvictorLeft(ctx, testNs.Name)
	})

	It("should remove the shared evictor once the adopted and the legacy service on its PVC path are deleted", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		createPVC(ctx, testNs.Name)

		// Every service in the namespace resolves this copy of the template, which
		// lacks the pvc-evictor container as a preset released before it does.
		var legacyTemplate *v1alpha2.LLMInferenceServiceConfig
		for _, preset := range SharedConfigPresets(testNs.Name) {
			if preset.Name == "kserve-config-llm-template" {
				legacyTemplate = preset
			}
		}
		Expect(legacyTemplate).ToNot(BeNil())
		evictorIndex := slices.IndexFunc(legacyTemplate.Spec.Template.Containers, func(container corev1.Container) bool {
			return container.Name == "pvc-evictor"
		})
		Expect(evictorIndex).To(BeNumerically(">=", 0))
		evictorContainer := legacyTemplate.Spec.Template.Containers[evictorIndex]
		legacyTemplate.Spec.Template.Containers = slices.Delete(legacyTemplate.Spec.Template.Containers, evictorIndex, evictorIndex+1)
		Expect(envTest.Create(ctx, legacyTemplate)).To(Succeed())

		// The adopted service gets the container from a baseRef rather than its own
		// spec, so the cleanup only sees it in the merged spec.
		evictorConfig := LLMInferenceServiceConfig("pvc-evictor-template",
			InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			WithConfigWorkloadTemplate(&corev1.PodSpec{Containers: []corev1.Container{evictorContainer}}),
		)
		Expect(envTest.Create(ctx, evictorConfig)).To(Succeed())

		adopted := service("adopted", testNs.Name, withSharedPVCRef, WithBaseRefs(corev1.LocalObjectReference{Name: evictorConfig.Name}))
		create(ctx, testNs, adopted)
		evictor := sharedEvictor(ctx, testNs.Name)
		// An available evictor turns PVCEvictorReady True, so seeing it means the
		// adopted service reconciled, cleanup included, with the evictor present.
		Eventually(func(g Gomega, ctx context.Context) {
			current := &appsv1.Deployment{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(evictor), current)).To(Succeed())
			current.Status.Replicas = 1
			current.Status.ReadyReplicas = 1
			current.Status.AvailableReplicas = 1
			g.Expect(envTest.Status().Update(ctx, current)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(adopted), current)).To(Succeed())
			g.Expect(current.Status.GetCondition(v1alpha2.PVCEvictorReady)).To(HaveField("Status", corev1.ConditionTrue))
		}).WithContext(ctx).Should(Succeed())

		legacy := service("legacy", testNs.Name, withSharedPVCRef)
		create(ctx, testNs, legacy)
		// status.workloads is written after the evictor pass.
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(legacy), current)).To(Succeed())
			g.Expect(current.Status.Workloads).ToNot(BeNil())
		}).WithContext(ctx).Should(Succeed())

		hasConsumerLabel := func(llmSvc *v1alpha2.LLMInferenceService) bool {
			GinkgoHelper()
			workload := &appsv1.Deployment{}
			Expect(envTest.Get(ctx, types.NamespacedName{Name: llmSvc.Name + "-kserve", Namespace: testNs.Name}, workload)).To(Succeed())
			for key := range workload.Spec.Template.Labels {
				if strings.HasPrefix(key, "serving.kserve.io/pvc-evictor-consumer-") {
					return true
				}
			}
			return false
		}
		Expect(hasConsumerLabel(adopted)).To(BeTrue(), "adopted workload is not an evictor consumer")
		Expect(hasConsumerLabel(legacy)).To(BeFalse(), "legacy workload is an evictor consumer")
		expectEvictorRunning(ctx, evictor)

		// when
		testNs.DeleteAndWait(ctx, adopted)

		// then
		// Checked once, before legacy is deleted: any pass after that excludes
		// legacy and removes the evictor whether or not legacy counts.
		current := &appsv1.Deployment{}
		err := envTest.Get(ctx, client.ObjectKeyFromObject(evictor), current)
		Expect(apierrors.IsNotFound(err) || (err == nil && !current.DeletionTimestamp.IsZero())).To(BeTrue(),
			"shared evictor outlived the last service that uses it: %v", err)
		err = envTest.Get(ctx, types.NamespacedName{Name: evictorServiceAccount, Namespace: testNs.Name}, &corev1.ServiceAccount{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "PVC evictor ServiceAccount: %v", err)

		// when
		testNs.DeleteAndWait(ctx, legacy)

		// then
		expectNoEvictorLeft(ctx, testNs.Name)
	})
})
