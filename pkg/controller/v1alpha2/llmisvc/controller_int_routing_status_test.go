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
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("Routing Status", func() {
	Context("Managed HTTPRoute with default gateway", func() {
		It("populates status.router.gateways after reconcile", func(ctx SpecContext) {
			// given
			svcName := "test-llm-routing-status"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// then - status.router should be populated
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status.Router).ToNot(BeNil(), "status.router should be populated")
				g.Expect(current.Status.Router.Gateways).To(HaveLen(1))

				gw := current.Status.Router.Gateways[0]
				g.Expect(string(gw.Group)).To(Equal("gateway.networking.k8s.io"))
				g.Expect(string(gw.Kind)).To(Equal("Gateway"))
				g.Expect(string(gw.Name)).To(Equal(constants.GatewayName))
				g.Expect(gw.Namespace).ToNot(BeNil())
				g.Expect(string(*gw.Namespace)).To(Equal(constants.KServeNamespace))

				g.Expect(gw.HTTPRoutes).To(HaveLen(1))
				g.Expect(string(gw.HTTPRoutes[0].Kind)).To(Equal("HTTPRoute"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Force-stop clears routing status", func() {
		It("sets status.router to nil when force-stop annotation is applied", func(ctx SpecContext) {
			// given
			svcName := "test-llm-routing-stop"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Verify routing status is populated before stopping
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Router).ToNot(BeNil(), "status.router should be populated before stop")
			}).WithContext(ctx).Should(Succeed())

			// when - apply force-stop annotation
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					if llmSvc.Annotations == nil {
						llmSvc.Annotations = make(map[string]string)
					}
					llmSvc.Annotations[constants.StopAnnotationKey] = "true"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - status.router should be cleared
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Router).To(BeNil(), "status.router should be nil when service is stopped")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("No router configuration", func() {
		It("leaves status.router nil when spec.router is nil", func(ctx SpecContext) {
			// given
			svcName := "test-llm-routing-nil"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				// No WithManagedRoute, no WithGatewayRefs, no WithHTTPRouteRefs
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - status.router should remain nil
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Router).To(BeNil(), "status.router should stay nil without router config")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("DeepEqual stability", func() {
		It("does not change status.router on consecutive reconciles with no spec change", func(ctx SpecContext) {
			// given
			svcName := "test-llm-routing-stable"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Wait for status.router to be populated and capture the initial content
			var initialGateways []v1alpha2.ObservedGateway
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Router).ToNot(BeNil())
				g.Expect(current.Status.Router.Gateways).To(HaveLen(1))
				initialGateways = append([]v1alpha2.ObservedGateway(nil), current.Status.Router.Gateways...)
			}).WithContext(ctx).Should(Succeed())

			// when - touch a label to trigger re-reconcile
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					if llmSvc.Labels == nil {
						llmSvc.Labels = make(map[string]string)
					}
					llmSvc.Labels["trigger-reconcile"] = "yes"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - status.router content should remain the same
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Router).ToNot(BeNil())
				g.Expect(current.Status.Router.Gateways).To(Equal(initialGateways))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
