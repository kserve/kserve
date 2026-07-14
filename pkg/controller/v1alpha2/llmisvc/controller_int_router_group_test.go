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
	"strings"
	"time"

	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Group Routing", func() {
	Context("Group formation", func() {
		It("should form a group with weighted backendRefs when two members share the same group", func(ctx SpecContext) {
			// given
			groupName := "my-group"
			svcNameA := "test-group-a"
			svcNameB := "test-group-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(80),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(20),
			)

			// when
			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// then - both members should have the routing-group label
			Eventually(func(g Gomega, ctx context.Context) {
				currentA := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), currentA)).To(Succeed())
				g.Expect(currentA.Labels).To(HaveKeyWithValue(constants.LLMRoutingGroupLabelKey, groupName))

				currentB := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcB), currentB)).To(Succeed())
				g.Expect(currentB.Labels).To(HaveKeyWithValue(constants.LLMRoutingGroupLabelKey, groupName))
			}).WithContext(ctx).Should(Succeed())

			// then - HTTPRoute for member A should have weighted backendRefs for both members
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(2), "should have backendRefs for both group members")

				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameA),
						HaveBackendWeight(int32(80)),
					),
				))
				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameB),
						HaveBackendWeight(int32(20)),
					),
				))
			}).WithContext(ctx).Should(Succeed())

			// then - group status should be populated on both members
			Eventually(func(g Gomega, ctx context.Context) {
				currentA := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), currentA)).To(Succeed())

				g.Expect(currentA.Status.Router).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group.Name).To(Equal(groupName))
				g.Expect(currentA.Status.Router.Group.Members).To(HaveLen(2))

				cond := currentA.Status.GetCondition(v1alpha2.GroupReady)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.IsTrue()).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				currentB := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcB), currentB)).To(Succeed())

				g.Expect(currentB.Status.Router).ToNot(BeNil())
				g.Expect(currentB.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentB.Status.Router.Group.Name).To(Equal(groupName))
				g.Expect(currentB.Status.Router.Group.Members).To(HaveLen(2))

				cond := currentB.Status.GetCondition(v1alpha2.GroupReady)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.IsTrue()).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Weight change", func() {
		It("should update HTTPRoute backendRefs when a member's weight changes", func(ctx SpecContext) {
			// given
			groupName := "weight-group"
			svcNameA := "test-weight-a"
			svcNameB := "test-weight-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// Verify initial equal weights
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(2))
				g.Expect(backendRefs).To(ContainElement(HaveBackendWeight(int32(50))))
			}).WithContext(ctx).Should(Succeed())

			// when - change weight on member A
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvcA, func() error {
					llmSvcA.Spec.Router.Route.Weight = ptr.To[int32](90)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - backendRefs should reflect the updated weights
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(2))
				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameA),
						HaveBackendWeight(int32(90)),
					),
				))
				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameB),
						HaveBackendWeight(int32(50)),
					),
				))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Model name divergence", func() {
		It("should form independent sub-groups and mark GroupDegraded when members have different model names", func(ctx SpecContext) {
			// given
			groupName := "diverge-group"
			svcNameA := "test-div-a"
			svcNameB := "test-div-b"
			svcNameC := "test-div-c"
			testNs := NewTestNamespace(ctx, envTest)

			// A and B share model name
			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(30),
			)

			// C has a different model name - forms its own sub-group
			llmSvcC := LLMInferenceService(svcNameC,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/llama-2-7b"),
				WithModelName("meta-llama/llama-2-7b"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(20),
			)

			// when
			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcC)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
				testNs.DeleteAndWait(ctx, llmSvcC)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcC)

			// then - all members should be GroupReady=True (each sub-group works)
			for _, svc := range []*v1alpha2.LLMInferenceService{llmSvcA, llmSvcB, llmSvcC} {
				Eventually(func(g Gomega, ctx context.Context) {
					current := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())

					cond := current.Status.GetCondition(v1alpha2.GroupReady)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.IsTrue()).To(BeTrue())

					degraded := current.Status.GetCondition(v1alpha2.GroupDegraded)
					g.Expect(degraded).ToNot(BeNil())
					g.Expect(degraded.IsTrue()).To(BeTrue())
					g.Expect(degraded.Reason).To(Equal("MemberDivergence"))
				}).WithContext(ctx).Should(Succeed())
			}

			// then - backendRefs on member A should contain only A and B (same model name)
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(2), "A's sub-group should have A and B")

				backendNames := backendRefNames(backendRefs)
				g.Expect(backendNames).To(ContainElements(
					ContainSubstring(svcNameA),
					ContainSubstring(svcNameB),
				))
				g.Expect(backendNames).ToNot(ContainElement(ContainSubstring(svcNameC)))
			}).WithContext(ctx).Should(Succeed())

			// then - backendRefs on member C should contain only C
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcC)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcC)
				g.Expect(backendRefs).To(HaveLen(1), "C's sub-group should have only C")
				g.Expect(backendRefs[0]).To(HaveBackendName(svcNameC))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Even split divergence (no majority needed)", func() {
		It("should form independent sub-groups with GroupReady=True and GroupDegraded without blocking Ready", func(ctx SpecContext) {
			// given - two members with different model names (1-1 split)
			groupName := "even-split-group"
			svcNameA := "test-even-a"
			svcNameB := "test-even-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("model-alpha"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("model-beta"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			// when
			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// then - both members should be GroupReady=True (each forms its own sub-group)
			// and GroupDegraded=True/MemberDivergence
			for _, svc := range []*v1alpha2.LLMInferenceService{llmSvcA, llmSvcB} {
				Eventually(func(g Gomega, ctx context.Context) {
					current := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())

					cond := current.Status.GetCondition(v1alpha2.GroupReady)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.IsTrue()).To(BeTrue(), "each sub-group should be ready")

					degraded := current.Status.GetCondition(v1alpha2.GroupDegraded)
					g.Expect(degraded).ToNot(BeNil())
					g.Expect(degraded.IsTrue()).To(BeTrue())
					g.Expect(degraded.Reason).To(Equal("MemberDivergence"))

					routerCond := current.Status.GetCondition(v1alpha2.RouterReady)
					g.Expect(routerCond).ToNot(BeNil())
					g.Expect(routerCond.IsTrue()).To(BeTrue(), "RouterReady should stay True - group divergence does not block readiness")
				}).WithContext(ctx).Should(Succeed())
			}

			// then - each member's route should have only its own backendRef
			for _, svc := range []*v1alpha2.LLMInferenceService{llmSvcA, llmSvcB} {
				Eventually(func(g Gomega, ctx context.Context) {
					routes, err := managedRoutes(ctx, svc)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))

					backendRefs := groupRoutingBackendRefs(&routes[0], svc)
					g.Expect(backendRefs).To(HaveLen(1), "each sub-group should have only its own backendRef")
					g.Expect(backendRefs[0]).To(HaveBackendName(svc.Name))
				}).WithContext(ctx).Should(Succeed())
			}
		})
	})

	Context("Member deletion", func() {
		It("should update remaining member's HTTPRoute when a group member is deleted", func(ctx SpecContext) {
			// given
			groupName := "del-group"
			svcNameA := "test-del-a"
			svcNameB := "test-del-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(60),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(40),
			)

			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// Wait for group to be formed with both members
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcA)).To(HaveLen(2))
			}).WithContext(ctx).Should(Succeed())

			// when - delete member B
			testNs.DeleteAndWait(ctx, llmSvcB)

			// then - remaining member A should have only its own backendRef
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(1), "should have only one backendRef after member deletion")
				g.Expect(backendRefs[0]).To(HaveBackendName(svcNameA))
			}).WithContext(ctx).Should(Succeed())

			// then - group status should show only one member
			Eventually(func(g Gomega, ctx context.Context) {
				currentA := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), currentA)).To(Succeed())
				g.Expect(currentA.Status.Router).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group.Members).To(HaveLen(1))
				g.Expect(currentA.Status.Router.Group.Members[0].Name).To(Equal(svcNameA))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Leave group", func() {
		It("should remove routing-group label and update peer when a member leaves the group", func(ctx SpecContext) {
			// given
			groupName := "leave-group"
			svcNameA := "test-leave-a"
			svcNameB := "test-leave-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(70),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(30),
			)

			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// Wait for group to be formed
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcA)).To(HaveLen(2))
			}).WithContext(ctx).Should(Succeed())

			// when - remove group/weight from member B (leaving the group)
			// Also remove the routing-group label since the defaulting webhook
			// is not installed in envtest (in a real cluster the webhook does this).
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvcB, func() error {
					llmSvcB.Spec.Router.Route.Group = nil
					llmSvcB.Spec.Router.Route.Weight = nil
					delete(llmSvcB.Labels, constants.LLMRoutingGroupLabelKey)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - remaining member A should have only its own backendRef
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(1))
				g.Expect(backendRefs[0]).To(HaveBackendName(svcNameA))
			}).WithContext(ctx).Should(Succeed())

			// then - remaining member A's group status should show only itself
			Eventually(func(g Gomega, ctx context.Context) {
				currentA := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), currentA)).To(Succeed())
				g.Expect(currentA.Status.Router).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group.Members).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Force-stop grouped member", func() {
		It("should set weight to 0 for a force-stopped member in peer group status", func(ctx SpecContext) {
			// given
			groupName := "stop-group"
			svcNameA := "test-stop-grp-a"
			svcNameB := "test-stop-grp-b"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(60),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(40),
			)

			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)

			// Wait for group to be formed
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcA)).To(HaveLen(2))
			}).WithContext(ctx).Should(Succeed())

			// when - force-stop member A
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvcA, func() error {
					if llmSvcA.Annotations == nil {
						llmSvcA.Annotations = make(map[string]string)
					}
					llmSvcA.Annotations[constants.StopAnnotationKey] = "true"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - on the remaining member B's HTTPRoute, member A should have weight 0
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcB)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcB)
				g.Expect(backendRefs).To(HaveLen(2), "stopped member should still appear in backendRefs")

				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameA),
						HaveBackendWeight(int32(0)),
					),
				))
				g.Expect(backendRefs).To(ContainElement(
					SatisfyAll(
						HaveBackendName(svcNameB),
						HaveBackendWeight(int32(40)),
					),
				))
			}).WithContext(ctx).Should(Succeed())

			// then - group status on member B should show member A with weight 0
			Eventually(func(g Gomega, ctx context.Context) {
				currentB := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcB), currentB)).To(Succeed())
				g.Expect(currentB.Status.Router).ToNot(BeNil())
				g.Expect(currentB.Status.Router.Group).ToNot(BeNil(), "group status should be preserved")
				g.Expect(currentB.Status.Router.Group.Name).To(Equal(groupName))
				g.Expect(currentB.Status.Router.Group.Members).To(HaveLen(2))

				for _, member := range currentB.Status.Router.Group.Members {
					if member.Name == svcNameA {
						g.Expect(member.Weight).To(Equal(int32(60)), "stopped member should show declared weight in group status")
						g.Expect(member.Stopped).To(BeTrue(), "stopped member should be marked as stopped")
					}
					if member.Name == svcNameB {
						g.Expect(member.Weight).To(Equal(int32(40)), "non-stopped member weight should be preserved")
					}
				}
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Group switch", func() {
		It("should move a member from one group to another, updating both groups' routes", func(ctx SpecContext) {
			// given - A and B in group-a, C in group-b
			svcNameA := "test-switch-a"
			svcNameB := "test-switch-b"
			svcNameC := "test-switch-c"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup("group-a"),
				WithWeight(80),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup("group-a"),
				WithWeight(20),
			)

			llmSvcC := LLMInferenceService(svcNameC,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup("group-b"),
				WithWeight(100),
			)

			// when
			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcC)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcA)
				testNs.DeleteAndWait(ctx, llmSvcB)
				testNs.DeleteAndWait(ctx, llmSvcC)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcC)

			// Verify initial state: A's route has backendRefs for A and B
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcA)).To(HaveLen(2))
			}).WithContext(ctx).Should(Succeed())

			// Verify initial state: C's route has backendRef for C only
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcC)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcC)).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())

			// when - move B from group-a to group-b
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvcB, func() error {
					llmSvcB.Spec.Router.Route.Group = ptr.To("group-b")
					llmSvcB.Labels[constants.LLMRoutingGroupLabelKey] = "group-b"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - A's route should drop B (1 backendRef: only A)
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcA)
				g.Expect(backendRefs).To(HaveLen(1), "group-a should have only member A after B switched")
				g.Expect(backendRefs[0]).To(HaveBackendName(svcNameA))
			}).WithContext(ctx).Should(Succeed())

			// then - C's route should gain B (2 backendRefs: B and C)
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcC)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcC)
				g.Expect(backendRefs).To(HaveLen(2), "group-b should have members B and C after switch")
				g.Expect(backendRefs).To(ContainElement(HaveBackendName(svcNameB)))
				g.Expect(backendRefs).To(ContainElement(HaveBackendName(svcNameC)))
			}).WithContext(ctx).Should(Succeed())

			// then - A's group status should show 1 member
			Eventually(func(g Gomega, ctx context.Context) {
				currentA := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), currentA)).To(Succeed())
				g.Expect(currentA.Status.Router).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentA.Status.Router.Group.Members).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())

			// then - C's group status should show 2 members
			Eventually(func(g Gomega, ctx context.Context) {
				currentC := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcC), currentC)).To(Succeed())
				g.Expect(currentC.Status.Router).ToNot(BeNil())
				g.Expect(currentC.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentC.Status.Router.Group.Members).To(HaveLen(2))
			}).WithContext(ctx).Should(Succeed())
		}, SpecTimeout(60*time.Second))
	})

	Context("Concurrent deletion", func() {
		It("should not deadlock when two group members are deleted simultaneously", func(ctx SpecContext) {
			// given - three members in a group; we will delete A and B at the same time
			groupName := "concurrent-group"
			svcNameA := "test-conc-a"
			svcNameB := "test-conc-b"
			svcNameC := "test-conc-c"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvcA := LLMInferenceService(svcNameA,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(50),
			)

			llmSvcB := LLMInferenceService(svcNameB,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(30),
			)

			llmSvcC := LLMInferenceService(svcNameC,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(20),
			)

			Expect(envTest.Create(ctx, llmSvcA)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcB)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvcC)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvcC)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcA)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcB)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvcC)

			// Verify initial state: A's route has 3 backendRefs
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcA)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(groupRoutingBackendRefs(&routes[0], llmSvcA)).To(HaveLen(3))
			}).WithContext(ctx).Should(Succeed())

			// when - delete A and B simultaneously.
			// DeleteAndWait blocks until the object is fully gone (finalizer removed, GC complete).
			// If both complete without timeout, the finalizer deadlock is avoided.
			testNs.DeleteAndWait(ctx, llmSvcA)
			testNs.DeleteAndWait(ctx, llmSvcB)

			// then - C's route should have only its own backendRef
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvcC)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				backendRefs := groupRoutingBackendRefs(&routes[0], llmSvcC)
				g.Expect(backendRefs).To(HaveLen(1), "should have only C's backendRef after A and B are deleted")
				g.Expect(backendRefs[0]).To(HaveBackendName(svcNameC))
			}).WithContext(ctx).Should(Succeed())

			// then - C's group status should show 1 member
			Eventually(func(g Gomega, ctx context.Context) {
				currentC := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcC), currentC)).To(Succeed())
				g.Expect(currentC.Status.Router).ToNot(BeNil())
				g.Expect(currentC.Status.Router.Group).ToNot(BeNil())
				g.Expect(currentC.Status.Router.Group.Members).To(HaveLen(1))
				g.Expect(currentC.Status.Router.Group.Members[0].Name).To(Equal(svcNameC))
			}).WithContext(ctx).Should(Succeed())

			// then - A and B should be fully gone
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcA), &v1alpha2.LLMInferenceService{})).
					To(MatchError(ContainSubstring("not found")))
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvcB), &v1alpha2.LLMInferenceService{})).
					To(MatchError(ContainSubstring("not found")))
			}).WithContext(ctx).Should(Succeed())
		}, SpecTimeout(60*time.Second))
	})
})

// groupRoutingBackendRefs returns backendRefs from the first group-rewritten rule
// in an HTTPRoute. Group routing (rewriteRulesForGroup) only rewrites rules that
// are NOT per-participant - per-participant rules (PathPrefix matching
// /{namespace}/{name}/...) retain their solo backendRef.
func groupRoutingBackendRefs(route *gwapiv1.HTTPRoute, owner *v1alpha2.LLMInferenceService) []gwapiv1.HTTPBackendRef {
	if len(route.Spec.Rules) == 0 {
		return nil
	}
	prefix := "/" + owner.Namespace + "/" + owner.Name
	for _, rule := range route.Spec.Rules {
		scoped := false
		for _, match := range rule.Matches {
			if match.Path != nil && match.Path.Value != nil {
				path := *match.Path.Value
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					scoped = true
					break
				}
			}
		}
		if !scoped {
			return rule.BackendRefs
		}
	}
	return nil
}

// backendRefNames extracts the backend names from a slice of HTTPBackendRef.
func backendRefNames(refs []gwapiv1.HTTPBackendRef) []string {
	names := make([]string, len(refs))
	for i, ref := range refs {
		names[i] = string(ref.Name)
	}
	return names
}

// HaveBackendName matches an HTTPBackendRef whose Name starts with the given prefix.
// Backend names include suffixes like "-inference-pool" or "-kserve-workload-svc".
func HaveBackendName(namePrefix string) OmegaMatcher {
	return WithTransform(func(ref gwapiv1.HTTPBackendRef) string {
		return string(ref.Name)
	}, HavePrefix(namePrefix))
}

// HaveBackendWeight matches an HTTPBackendRef with the given weight.
func HaveBackendWeight(weight int32) OmegaMatcher {
	return WithTransform(func(ref gwapiv1.HTTPBackendRef) *int32 {
		return ref.Weight
	}, Equal(ptr.To(weight)))
}
