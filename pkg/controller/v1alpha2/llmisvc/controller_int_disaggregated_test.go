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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService DisaggregatedSet", func() {
	enableDisaggregatedSetGate := func(ctx context.Context) {
		PatchLLMISVCFeatureGates(ctx, envTest.Client, map[string]bool{"disaggregatedSet": true})
		DeferCleanup(func(ctx context.Context) {
			PatchLLMISVCFeatureGates(ctx, envTest.Client, nil)
		})
	}

	optIn := WithSpecAnnotations(map[string]string{constants.LLMDisaggregatedSetAnnotationKey: "true"})
	optOut := WithSpecAnnotations(map[string]string{constants.LLMDisaggregatedSetAnnotationKey: "false"})

	singleNodePD := func(name, namespace string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
		return LLMInferenceService(name, append([]LLMInferenceServiceOption{
			InNamespace[*v1alpha2.LLMInferenceService](namespace),
			WithModelURI("hf://facebook/opt-125m"),
			WithTemplate(SimpleWorkerPodSpec()),
			WithPrefill(SimpleWorkerPodSpec()),
		}, opts...)...)
	}

	Context("with the feature gate enabled", func() {
		It("runs an opted-in single-node P/D service on one DisaggregatedSet", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-single", testNs.Name, optIn, WithReplicas(2))

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			ds := getDisaggregatedSet(ctx, llmSvc)
			Expect(ds).To(BeOwnedBy(llmSvc))
			Expect(ds.Spec.Roles).To(HaveLen(2))

			decode, prefill := ds.Spec.Roles[0], ds.Spec.Roles[1]
			Expect(decode.Name).To(Equal(constants.LLMDRoleDecode))
			Expect(decode.Spec.Replicas).To(Equal(ptr.To[int32](2)))
			Expect(decode.Spec.LeaderWorkerTemplate.Size).To(Equal(ptr.To[int32](1)))
			Expect(decode.Spec.LeaderWorkerTemplate.LeaderTemplate).To(BeNil())
			Expect(decode.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, constants.LLMDRoleDecode))
			Expect(prefill.Name).To(Equal(constants.LLMDRolePrefill))
			Expect(prefill.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			Expect(prefill.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, constants.LLMDRolePrefill))

			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Workloads).NotTo(BeNil())
				want := corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("disaggregatedset.x-k8s.io"),
					Kind:     "DisaggregatedSet",
					Name:     ds.Name,
				}
				g.Expect(current.Status.Workloads.Primary).NotTo(BeNil())
				g.Expect(current.Status.Workloads.Primary.TypedLocalObjectReference).To(Equal(want))
				g.Expect(current.Status.Workloads.Prefill).NotTo(BeNil())
				g.Expect(current.Status.Workloads.Prefill.TypedLocalObjectReference).To(Equal(want))
			}).WithContext(ctx).Should(Succeed())
		})

		It("maps DisaggregatedSet readiness onto the workload conditions", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-ready", testNs.Name, optIn)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)
			getDisaggregatedSet(ctx, llmSvc)

			// when
			setDisaggregatedSetStatus(ctx, llmSvc, 1, 1)

			// then
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.MainWorkloadReady), "True"))
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PrefillWorkloadReady), "True"))
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.WorkloadReady), "True"))
			}).WithContext(ctx).Should(Succeed())

			// when
			setDisaggregatedSetStatus(ctx, llmSvc, 1, 0, metav1.Condition{
				Type:    string(disaggregatedsetv1.DisaggregatedSetAvailable),
				Status:  metav1.ConditionFalse,
				Reason:  "RolloutInProgress",
				Message: "prefill is rolling out",
			})

			// then
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.MainWorkloadReady), "True"))
				prefill := current.Status.GetCondition(v1alpha2.PrefillWorkloadReady)
				g.Expect(prefill).NotTo(BeNil())
				g.Expect(prefill.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(prefill.Reason).To(Equal("RolloutInProgress"))
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.WorkloadReady), "False"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("runs a P/D service on a DisaggregatedSet by default", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-default", testNs.Name)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			Expect(getDisaggregatedSet(ctx, llmSvc)).To(BeOwnedBy(llmSvc))
			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			Consistently(func(ctx context.Context) *corev1.Event {
				return findEvent(ctx, envTest.Client, llmSvc, "MigratingToDisaggregatedSet")
			}).WithContext(ctx).WithTimeout(3*time.Second).Should(BeNil(), "a new service has nothing to migrate")
		})

		It("moves a running service onto a DisaggregatedSet and back", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-migrate", testNs.Name, optOut)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)

			// when the service drops its opt-out, it takes the preset default
			removeDisaggregatedSetAnnotation(ctx, llmSvc)

			// then
			getDisaggregatedSet(ctx, llmSvc)
			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectNotFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			expectEvent(ctx, llmSvc, "MigratingToDisaggregatedSet")

			// when
			setDisaggregatedSetAnnotation(ctx, llmSvc, "false")

			// then
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
			expectEvent(ctx, llmSvc, "MigratingFromDisaggregatedSet")
		})

		It("runs a multi-node P/D service on one DisaggregatedSet", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			parallelism := ParallelismSpec(WithDataParallelism(2), WithDataLocalParallelism(1), WithTensorParallelism(2))
			llmSvc := singleNodePD("ds-multi", testNs.Name, optIn,
				WithWorker(SimpleWorkerPodSpec()),
				WithParallelism(parallelism),
				WithPrefillWorker(SimpleWorkerPodSpec()),
				WithPrefillParallelism(parallelism.DeepCopy()),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			ds := getDisaggregatedSet(ctx, llmSvc)
			for _, role := range ds.Spec.Roles {
				Expect(role.Spec.LeaderWorkerTemplate.LeaderTemplate).NotTo(BeNil(), "role %s", role.Name)
				Expect(role.Spec.LeaderWorkerTemplate.Size).To(Equal(parallelism.GetSize()), "role %s", role.Name)
			}
			expectNotFound(ctx, &lwsapi.LeaderWorkerSet{}, llmSvc.Name+"-kserve-mn", llmSvc.Namespace)
			expectNotFound(ctx, &lwsapi.LeaderWorkerSet{}, llmSvc.Name+"-kserve-mn-prefill", llmSvc.Namespace)
		})

		It("keeps the current workloads when autoscaling is configured", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-scaling", testNs.Name, optIn, WithScaling(HPAScaling(1, 2)))

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectNotUsed(ctx, llmSvc, "AutoscalingNotSupported")
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
		})

		It("keeps the current workloads when autoscaling meets the preset default", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-scaling-default", testNs.Name, WithScaling(HPAScaling(1, 2)))

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			expectNotUsed(ctx, llmSvc, "AutoscalingNotSupported")
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
		})

		It("deletes the DisaggregatedSet when the service is stopped", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-stop", testNs.Name, optIn)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)
			getDisaggregatedSet(ctx, llmSvc)

			// when
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, err := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					if llmSvc.Annotations == nil {
						llmSvc.Annotations = map[string]string{}
					}
					llmSvc.Annotations[constants.StopAnnotationKey] = "true"
					return nil
				})
				return err
			})).To(Succeed())

			// then
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				main := current.Status.GetCondition(v1alpha2.MainWorkloadReady)
				g.Expect(main).NotTo(BeNil())
				g.Expect(main.Reason).To(Equal("Stopped"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("keeps the deployed storage-initializer image", func(ctx SpecContext) {
			// given
			enableDisaggregatedSetGate(ctx)
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-storage", testNs.Name, optIn)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)
			getDisaggregatedSet(ctx, llmSvc)

			// when a role runs an older storage-initializer than the one configured
			const deployedImage = "kserve/storage-initializer:deployed"
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				ds := getDisaggregatedSet(ctx, llmSvc)
				setStorageInitializerImage(&ds.Spec.Roles[0].Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, deployedImage)
				return envTest.Update(ctx, ds)
			})).To(Succeed())

			// then the controller keeps it rather than rolling the role
			Consistently(func(g Gomega, ctx context.Context) {
				ds := getDisaggregatedSet(ctx, llmSvc)
				g.Expect(storageInitializerImage(&ds.Spec.Roles[0].Spec.LeaderWorkerTemplate.WorkerTemplate.Spec)).To(Equal(deployedImage))
			}).WithContext(ctx).WithTimeout(3 * time.Second).Should(Succeed())
		})
	})

	Context("with the feature gate disabled", func() {
		It("keeps the current workloads and says why", func(ctx SpecContext) {
			// given
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-gate-off", testNs.Name, optIn)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve", llmSvc.Namespace)
			expectFound(ctx, &appsv1.Deployment{}, llmSvc.Name+"-kserve-prefill", llmSvc.Namespace)
			expectNotUsed(ctx, llmSvc, "FeatureGateDisabled")
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
		})

		It("keeps the current workloads when only the preset asks for it", func(ctx SpecContext) {
			// given
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := singleNodePD("ds-gate-off-default", testNs.Name)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			// then
			for _, name := range []string{llmSvc.Name + "-kserve", llmSvc.Name + "-kserve-prefill"} {
				deployment := &appsv1.Deployment{}
				expectFound(ctx, deployment, name, llmSvc.Namespace)
				// The presets carry the annotation, but it must stay off the pod
				// templates: otherwise adding it to the presets would roll every
				// P/D service on upgrade, even with the gate off.
				Expect(deployment.Spec.Template.Annotations).NotTo(HaveKey(constants.LLMDisaggregatedSetAnnotationKey), name)
			}
			expectNotUsed(ctx, llmSvc, "FeatureGateDisabled")
			expectNotFound(ctx, &disaggregatedsetv1.DisaggregatedSet{}, llmSvc.Name+"-kserve-pd", llmSvc.Namespace)
		})
	})
})

func getDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *disaggregatedsetv1.DisaggregatedSet {
	GinkgoHelper()
	ds := &disaggregatedsetv1.DisaggregatedSet{}
	Eventually(func(g Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{Name: llmSvc.Name + "-kserve-pd", Namespace: llmSvc.Namespace}, ds)
	}).WithContext(ctx).Should(Succeed())
	return ds
}

// setDisaggregatedSetStatus simulates the DisaggregatedSet controller, which envtest
// does not run: the decode role has decodeReady of its replicas ready and updated, the
// prefill role prefillReady.
func setDisaggregatedSetStatus(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, decodeReady, prefillReady int32, conditions ...metav1.Condition) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		ds := getDisaggregatedSet(ctx, llmSvc)
		ds.Status.ObservedGeneration = ds.Generation
		ds.Status.RoleStatuses = []disaggregatedsetv1.RoleStatus{
			{Name: constants.LLMDRoleDecode, Replicas: 1, ReadyReplicas: decodeReady, UpdatedReplicas: decodeReady},
			{Name: constants.LLMDRolePrefill, Replicas: 1, ReadyReplicas: prefillReady, UpdatedReplicas: prefillReady},
		}
		for i := range conditions {
			conditions[i].LastTransitionTime = metav1.Now()
		}
		ds.Status.Conditions = conditions
		g.Expect(envTest.Status().Update(ctx, ds)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())
}

func setDisaggregatedSetAnnotation(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, value string) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
			if llmSvc.Spec.Annotations == nil {
				llmSvc.Spec.Annotations = map[string]string{}
			}
			llmSvc.Spec.Annotations[constants.LLMDisaggregatedSetAnnotationKey] = value
			return nil
		})
		return err
	})).To(Succeed())
}

func removeDisaggregatedSetAnnotation(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
			delete(llmSvc.Spec.Annotations, constants.LLMDisaggregatedSetAnnotationKey)
			return nil
		})
		return err
	})).To(Succeed())
}

func expectFound(ctx context.Context, obj client.Object, name, namespace string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	}).WithContext(ctx).Should(Succeed(), "%s %s/%s should exist", obj.GetObjectKind().GroupVersionKind().Kind, namespace, name)
}

func expectNotFound(ctx context.Context, obj client.Object, name, namespace string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) bool {
		return apierrors.IsNotFound(envTest.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj))
	}).WithContext(ctx).Should(BeTrue(), "%s/%s should not exist", namespace, name)
}

// expectNotUsed checks that the service says why it keeps its current workloads in
// the DisaggregatedSetUsed condition, without a warning event: only a migration warns.
func expectNotUsed(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, reason string) {
	GinkgoHelper()
	expectNotUsedCondition(ctx, llmSvc, reason)
	Consistently(func(ctx context.Context) *corev1.Event {
		return findEvent(ctx, envTest.Client, llmSvc, "DisaggregatedSetNotUsed")
	}).WithContext(ctx).WithTimeout(3 * time.Second).Should(BeNil())
}

func expectEvent(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, reason string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		event := findEvent(ctx, envTest.Client, llmSvc, reason)
		g.Expect(event).NotTo(BeNil())
		g.Expect(event.Type).To(Equal(corev1.EventTypeWarning))
	}).WithContext(ctx).Should(Succeed())
}

func expectNotUsedCondition(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, reason string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		current := &v1alpha2.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		cond := current.Status.GetCondition(v1alpha2.DisaggregatedSetUsed)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal(reason))
	}).WithContext(ctx).Should(Succeed())
}

func setStorageInitializerImage(spec *corev1.PodSpec, image string) {
	GinkgoHelper()
	for i := range spec.InitContainers {
		if spec.InitContainers[i].Name == constants.StorageInitializerContainerName {
			spec.InitContainers[i].Image = image
			return
		}
	}
	Fail("pod spec has no storage-initializer init container")
}

func storageInitializerImage(spec *corev1.PodSpec) string {
	for _, c := range spec.InitContainers {
		if c.Name == constants.StorageInitializerContainerName {
			return c.Image
		}
	}
	return ""
}
