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
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("InferencePool Migration", func() {
	Context("Dual-pool creation (both v1 and v1alpha2 CRDs available)", func() {
		It("should create both v1 and v1alpha2 InferencePools when both CRDs are available", func(ctx SpecContext) {
			// given
			svcName := "test-llm-dual-pool"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify v1 InferencePool is created
			expectedPoolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) error {
				v1Pool := &igwapi.InferencePool{}
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed(), "v1 InferencePool should be created")

			// then - verify v1alpha2 InferencePool is created (same name, different API group)
			Eventually(func(g Gomega, ctx context.Context) error {
				v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2Pool)
			}).WithContext(ctx).Should(Succeed(), "v1alpha2 InferencePool should be created")

			// Verify both pools have correct owner reference
			v1Pool := &igwapi.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)).To(Succeed())
			Expect(v1Pool).To(BeOwnedBy(llmSvc))

			v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2Pool)).To(Succeed())
			Expect(v1alpha2Pool).To(BeOwnedBy(llmSvc))

			// Note: In envtest, the v1 pool is considered ready immediately (no status conditions = valid pool = ready),
			// so migration may happen before we can check. The key assertion is that both pools are created.
			// The migration annotation and HTTPRoute swap behavior are tested in separate test cases.
		})

		It("should have dual backendRefs with v1alpha2 active before migration", func(ctx SpecContext) {
			// given
			svcName := "test-llm-initial-backend"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify HTTPRoute has dual backendRefs with v1alpha2 active (weight=1) and v1 inactive (weight=0).
			// Both pools are referenced so the Gateway controller programs both.
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).To(HaveLen(2),
					"HTTPRoute should have dual backendRefs (v1 + v1alpha2)")

				// First ref: v1 pool with weight=0 (inactive)
				v1Ref := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(v1Ref.Group).ToNot(BeNil())
				g.Expect(string(*v1Ref.Group)).To(Equal(constants.InferencePoolV1APIGroupName))
				g.Expect(v1Ref.Weight).ToNot(BeNil())
				g.Expect(*v1Ref.Weight).To(Equal(int32(0)), "v1 pool should have weight=0 before migration")

				// Second ref: v1alpha2 pool with weight=1 (active)
				v1alpha2Ref := route.Spec.Rules[0].BackendRefs[1]
				g.Expect(v1alpha2Ref.Group).ToNot(BeNil())
				g.Expect(string(*v1alpha2Ref.Group)).To(Equal(constants.InferencePoolV1Alpha2APIGroupName))
				g.Expect(v1alpha2Ref.Weight).ToNot(BeNil())
				g.Expect(*v1alpha2Ref.Weight).To(Equal(int32(1)), "v1alpha2 pool should have weight=1 before migration")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Auto-migration when v1 pool becomes ready", func() {
		It("should set migration annotation when v1 InferencePool becomes ready", func(ctx SpecContext) {
			// given
			svcName := "test-llm-auto-migrate"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for v1 pool to be created
			expectedPoolName := svcName + "-inference-pool"
			v1Pool := &igwapi.InferencePool{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed())

			// when - make the v1 pool ready
			ensureV1InferencePoolReady(ctx, envTest.Client, v1Pool)

			// Make managed resources ready (Gateway, HTTPRoute)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// then - verify migration annotation is set
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

				g.Expect(updatedLLMSvc.Annotations).To(HaveKeyWithValue(
					constants.InferencePoolMigratedAnnotationKey, "v1"),
					"Migration annotation should be set to 'v1' when v1 pool becomes ready")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should flip weights to v1 active after migration", func(ctx SpecContext) {
			// given
			svcName := "test-llm-backend-swap"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for v1 pool to be created
			expectedPoolName := svcName + "-inference-pool"
			v1Pool := &igwapi.InferencePool{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed())

			// Make the v1 pool ready
			ensureV1InferencePoolReady(ctx, envTest.Client, v1Pool)

			// Make managed resources ready
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Wait for migration to complete
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())
				g.Expect(updatedLLMSvc.Annotations).To(HaveKey(constants.InferencePoolMigratedAnnotationKey))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// then - verify dual backendRefs with v1 active (weight=1) and v1alpha2 inactive (weight=0)
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).To(HaveLen(2),
					"HTTPRoute should have dual backendRefs after migration")

				v1Ref := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(string(*v1Ref.Group)).To(Equal(constants.InferencePoolV1APIGroupName))
				g.Expect(*v1Ref.Weight).To(Equal(int32(1)), "v1 pool should have weight=1 after migration")

				v1alpha2Ref := route.Spec.Rules[0].BackendRefs[1]
				g.Expect(string(*v1alpha2Ref.Group)).To(Equal(constants.InferencePoolV1Alpha2APIGroupName))
				g.Expect(*v1alpha2Ref.Weight).To(Equal(int32(0)), "v1alpha2 pool should have weight=0 after migration")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should keep both pools after migration (v1alpha2 as fallback)", func(ctx SpecContext) {
			// given
			svcName := "test-llm-both-pools"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for pools to be created
			expectedPoolName := svcName + "-inference-pool"
			v1Pool := &igwapi.InferencePool{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed())

			// Make the v1 pool ready to trigger migration
			ensureV1InferencePoolReady(ctx, envTest.Client, v1Pool)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Wait for migration
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())
				g.Expect(updatedLLMSvc.Annotations).To(HaveKey(constants.InferencePoolMigratedAnnotationKey))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// then - verify BOTH pools still exist after migration
			v1PoolAfter := &igwapi.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1PoolAfter)).To(Succeed(),
				"v1 InferencePool should still exist after migration")

			v1alpha2PoolAfter := &igwapiv1alpha2.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2PoolAfter)).To(Succeed(),
				"v1alpha2 InferencePool should still exist after migration (fallback)")
		})
	})

	Context("Pre-migrated deployments", func() {
		It("should have dual backendRefs with v1 active when migration annotation is already set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-pre-migrated"
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

			// Create LLMInferenceService with migration annotation already set
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithAnnotations(map[string]string{
					constants.InferencePoolMigratedAnnotationKey: "v1",
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - dual backendRefs with v1 active
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).To(HaveLen(2),
					"HTTPRoute should have dual backendRefs for pre-migrated deployment")

				v1Ref := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(string(*v1Ref.Group)).To(Equal(constants.InferencePoolV1APIGroupName))
				g.Expect(*v1Ref.Weight).To(Equal(int32(1)), "v1 pool should have weight=1 for pre-migrated deployment")

				v1alpha2Ref := route.Spec.Rules[0].BackendRefs[1]
				g.Expect(string(*v1alpha2Ref.Group)).To(Equal(constants.InferencePoolV1Alpha2APIGroupName))
				g.Expect(*v1alpha2Ref.Weight).To(Equal(int32(0)), "v1alpha2 pool should have weight=0 for pre-migrated deployment")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Readiness evaluation", func() {
		It("should report ready if either v1 or v1alpha2 pool is ready (prioritizing v1)", func(ctx SpecContext) {
			// given
			svcName := "test-llm-readiness"
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
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for pools to be created
			expectedPoolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) error {
				v1Pool := &igwapi.InferencePool{}
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed())

			// Make only v1alpha2 pool ready (simulate scenario where v1 is not ready yet)
			v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2Pool)
			}).WithContext(ctx).Should(Succeed())

			ensureV1Alpha2InferencePoolReady(ctx, envTest.Client, v1alpha2Pool)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// then - InferencePoolReady condition should be True (v1alpha2 is ready)
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

				condition := updatedLLMSvc.Status.GetCondition(v1alpha2.InferencePoolReady)
				g.Expect(condition).ToNot(BeNil(), "InferencePoolReady condition should be set")
				g.Expect(condition.IsTrue()).To(BeTrue(), "InferencePoolReady should be True when either pool is ready")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Migration lock (prevents flapping)", func() {
		It("should not change migration annotation once set to v1", func(ctx SpecContext) {
			// given
			svcName := "test-llm-migration-lock"
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

			// Create with migration annotation already set (simulating existing migrated deployment)
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithAnnotations(map[string]string{
					constants.InferencePoolMigratedAnnotationKey: "v1",
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Make managed resources ready
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Verify migration annotation remains "v1" (not changed)
			Consistently(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())
				g.Expect(updatedLLMSvc.Annotations).To(HaveKeyWithValue(
					constants.InferencePoolMigratedAnnotationKey, "v1"),
					"Migration annotation should remain 'v1' (one-way lock)")
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

// ensureV1InferencePoolReady sets up v1 InferencePool status conditions to simulate a ready pool
func ensureV1InferencePoolReady(ctx context.Context, c client.Client, pool *igwapi.InferencePool) {
	if envTest.UsingExistingCluster() {
		return
	}

	createdPool := &igwapi.InferencePool{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), createdPool)).To(Succeed())
	WithInferencePoolReadyStatus()(createdPool)
	Expect(c.Status().Update(ctx, createdPool)).To(Succeed())

	// Verify the InferencePool is now ready
	Eventually(func(g Gomega, ctx context.Context) bool {
		updatedPool := &igwapi.InferencePool{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), updatedPool)).To(Succeed())
		return llmisvc.IsInferencePoolReady(updatedPool)
	}).WithContext(ctx).Should(BeTrue())
}

// ensureV1Alpha2InferencePoolReady sets up v1alpha2 InferencePool status conditions to simulate a ready pool
func ensureV1Alpha2InferencePoolReady(ctx context.Context, c client.Client, pool *igwapiv1alpha2.InferencePool) {
	if envTest.UsingExistingCluster() {
		return
	}

	createdPool := &igwapiv1alpha2.InferencePool{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), createdPool)).To(Succeed())
	WithInferencePoolV1Alpha2ReadyStatus()(createdPool)
	Expect(c.Status().Update(ctx, createdPool)).To(Succeed())

	// Verify the InferencePool is now ready
	Eventually(func(g Gomega, ctx context.Context) bool {
		updatedPool := &igwapiv1alpha2.InferencePool{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), updatedPool)).To(Succeed())
		return llmisvc.IsInferencePoolV1Alpha2Ready(updatedPool)
	}).WithContext(ctx).Should(BeTrue())
}

// Helper to list managed InferencePools
func managedInferencePools(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*igwapi.InferencePoolList, error) {
	pools := &igwapi.InferencePoolList{}
	listOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
	}
	err := envTest.List(ctx, pools, listOpts)
	return pools, err
}

// Helper to list managed v1alpha2 InferencePools
func managedInferencePoolsV1Alpha2(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*igwapiv1alpha2.InferencePoolList, error) {
	pools := &igwapiv1alpha2.InferencePoolList{}
	listOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
	}
	err := envTest.List(ctx, pools, listOpts)
	return pools, err
}
