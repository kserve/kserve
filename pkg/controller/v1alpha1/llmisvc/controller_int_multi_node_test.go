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

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"

	. "github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Multi-Node Controller", func() {
	Context("Multi-Node Workload Reconciliation", func() {
		It("should create a basic multi-node deployment with worker spec", func(ctx SpecContext) {
			// given
			svcName := "test-llm-multinode"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithReplicas(2),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(4),
					WithDataLocalParallelism(1),
					WithTensorParallelism(3),
				)),
				WithTemplate(SimpleWorkerPodSpec()),
				WithWorker(SimpleWorkerPodSpec()),
				WithPrefill(SimpleWorkerPodSpec()),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedLWS)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedLWS.Spec.Replicas).To(Equal(ptr.To[int32](2)))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.Size).To(Equal(ptr.To[int32](4)))
			Expect(expectedLWS).To(BeOwnedBy(llmSvc))

			// Verify leader template is set
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate).ToNot(BeNil())
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers).To(HaveLen(1))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].Name).To(Equal("main"))

			// Verify worker template is set
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers).To(HaveLen(1))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].Name).To(Equal("main"))

			// Verify labels
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue("kserve.io/component", "workload"))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue("llm-d.ai/role", "decode"))
		})

		It("should create multi-node deployment with prefill workload", func(ctx SpecContext) {
			// given
			svcName := "test-llm-multinode-prefill"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithReplicas(1),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(10),
					WithDataLocalParallelism(2),
					WithTensorParallelism(4),
				)),
				WithWorker(SimpleWorkerPodSpec()),
				WithPrefillParallelism(ParallelismSpec(
					WithDataParallelism(3),
					WithDataLocalParallelism(1),
					WithTensorParallelism(4),
				)),
				WithPrefillWorker(SimpleWorkerPodSpec()),
				WithPrefillReplicas(1),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - Check main workload LWS
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedMainLWS.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			Expect(expectedMainLWS.Spec.LeaderWorkerTemplate.Size).To(Equal(ptr.To[int32](5)))

			// then - Check prefill workload LWS
			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedPrefillLWS.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			Expect(expectedPrefillLWS.Spec.LeaderWorkerTemplate.Size).To(Equal(ptr.To[int32](3)))
			Expect(expectedPrefillLWS).To(BeOwnedBy(llmSvc))

			// Verify prefill-specific labels
			Expect(expectedPrefillLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue("llm-d.ai/role", "prefill"))
		})

		It("should create RBAC resources when prefill and decode is used", func(ctx SpecContext) {
			// given
			svcName := "test-llm-multinode-rbac"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithReplicas(1),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
					WithTensorParallelism(4),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedScheduler(),
				WithManagedGateway(),
				WithPrefill(SimpleWorkerPodSpec()),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - Check ServiceAccount is created
			expectedSA := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedSA)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedSA).To(BeOwnedBy(llmSvc))
			Expect(expectedSA.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", svcName))

			// then - Check Role is created
			expectedRole := &rbacv1.Role{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-role",
					Namespace: nsName,
				}, expectedRole)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedRole).To(BeOwnedBy(llmSvc))
			Expect(expectedRole.Rules).ToNot(BeEmpty())

			// then - Check RoleBinding is created
			expectedRB := &rbacv1.RoleBinding{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-rb",
					Namespace: nsName,
				}, expectedRB)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedRB).To(BeOwnedBy(llmSvc))
			Expect(expectedRB.Subjects).To(HaveLen(1))
			Expect(expectedRB.Subjects[0].Name).To(Equal(expectedSA.Name))
			Expect(expectedRB.RoleRef.Name).To(Equal(expectedRole.Name))

			// then - Check LWS uses the ServiceAccount
			expectedLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedLWS)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedLWS).To(BeOwnedBy(llmSvc))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.ServiceAccountName).To(Equal(expectedSA.Name))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.ServiceAccountName).To(Equal(expectedSA.Name))
		})

		It("should delete multi-node resources when worker spec is removed", func(ctx SpecContext) {
			// given
			svcName := "test-llm-multinode-cleanup"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			parallelismSpec := ParallelismSpec(
				WithDataParallelism(2),
				WithDataLocalParallelism(1),
			)
			parallelismSpec.Expert = true

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(parallelismSpec),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			lwsName := svcName + "-kserve-mn"

			// Verify LWS is created
			Eventually(func(g Gomega, ctx context.Context) error {
				lws := &lwsapi.LeaderWorkerSet{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      lwsName,
					Namespace: nsName,
				}, lws)
			}).WithContext(ctx).Should(Succeed())

			// when - Remove worker spec
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Worker = nil
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - LWS should be deleted
			Eventually(func(g Gomega, ctx context.Context) error {
				lws := &lwsapi.LeaderWorkerSet{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      lwsName,
					Namespace: nsName,
				}, lws)
				g.Expect(err).To(HaveOccurred())
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete prefill resources when prefill spec is removed", func(ctx SpecContext) {
			// given
			svcName := "test-llm-prefill-cleanup"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithPrefillParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithPrefillWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			prefillLWSName := kmeta.ChildName(svcName, "-kserve-mn-prefill")

			// Verify prefill LWS is created
			Eventually(func(g Gomega, ctx context.Context) error {
				lws := &lwsapi.LeaderWorkerSet{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      prefillLWSName,
					Namespace: nsName,
				}, lws)
			}).WithContext(ctx).Should(Succeed())

			// when - Remove prefill spec
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Prefill = nil
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - Prefill LWS should be deleted
			Eventually(func(g Gomega, ctx context.Context) error {
				lws := &lwsapi.LeaderWorkerSet{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      prefillLWSName,
					Namespace: nsName,
				}, lws)
				g.Expect(err).To(HaveOccurred())
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Multi-Node Label and Annotation Management", func() {
		It("should set correct labels and annotation", func(ctx SpecContext) {
			// given
			svcName := "test-llm-lws-labels"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			localQueueName := "test-local-q"
			preemptPriority := "0"
			testValue := "test"

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(1),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
				// Add a kueue label and annotation to ensure value propagation to the LWS
				// the kueue functionality itself will not be tested here
				WithAnnotations(map[string]string{
					PreemptionReclaimAnnotationKey: preemptPriority,
					testValue:                      testValue, // dummy value, should not be propagated
				}),
				WithLabels(map[string]string{
					LocalQueueNameLabelKey: localQueueName,
					testValue:              testValue, // dummy value, should not be propagated
				}),
			)

			// safety check
			Expect(llmSvc.Spec.Parallelism.IsDataParallel()).To(BeTrue())
			Expect(llmSvc.Spec.Worker).To(Not(BeNil()))

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedLWS)
			}).WithContext(ctx).Should(Succeed())

			By("checking the LeaderWorkerSet's top-level metadata")
			Expect(expectedLWS).To(BeOwnedBy(llmSvc))
			Expect(expectedLWS.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, localQueueName))
			Expect(expectedLWS.Labels).ToNot(HaveKeyWithValue(testValue, testValue))

			Expect(expectedLWS.Annotations).To(HaveKeyWithValue(PreemptionReclaimAnnotationKey, preemptPriority))

			By("checking the leader pod template metadata")
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.Size).To(Equal(ptr.To(int32(1))))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate).To(Not(BeNil()))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue("kserve.io/component", "workload"))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue("llm-d.ai/role", "both"))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, localQueueName))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).ToNot(HaveKeyWithValue(testValue, testValue))

			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations).To(HaveKeyWithValue(PreemptionReclaimAnnotationKey, preemptPriority))

			By("checking the worker pod template metadata")
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate).To(Not(BeNil()))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, localQueueName))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels).ToNot(HaveKeyWithValue(testValue, testValue))

			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations).To(HaveKeyWithValue(PreemptionReclaimAnnotationKey, preemptPriority))
		})
	})
})
