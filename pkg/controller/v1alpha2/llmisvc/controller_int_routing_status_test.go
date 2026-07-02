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

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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

	Context("Status addresses contain model names", func() {
		It("populates status.addresses[].models with the base model name", func(ctx SpecContext) {
			svcName := "test-llm-addr-models"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			gwName := "addr-models-gw"
			gw := Gateway(gwName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
			)
			Expect(envTest.Client.Create(ctx, gw)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, gw)
			setGatewayStatusAddresses(ctx, envTest.Client, gw, "203.0.113.50")

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithGatewayRefs(LLMGatewayRef(gwName, testNs.Name)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Addresses).ToNot(BeEmpty(), "status.addresses should be populated")

				for _, addr := range current.Status.Addresses {
					g.Expect(addr.Models).ToNot(BeEmpty(), "each address should have models")

					modelNames := make([]string, 0, len(addr.Models))
					for _, m := range addr.Models {
						modelNames = append(modelNames, m.Name)
					}

					if addr.Name != nil && strings.HasSuffix(*addr.Name, "-model-routing") {
						g.Expect(modelNames).To(ContainElement("publishers/"+testNs.Name+"/models/facebook/opt-125m"),
							"model-routing address should use publishers format")
					} else {
						g.Expect(modelNames).To(ContainElement("facebook/opt-125m"),
							"path-based address should use plain model name")
					}
				}
			}).WithContext(ctx).Should(Succeed())
		})

		It("includes LoRA adapter model names in status.addresses", func(ctx SpecContext) {
			svcName := "test-llm-addr-lora"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			gwName := "addr-lora-gw"
			gw := Gateway(gwName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
			)
			Expect(envTest.Client.Create(ctx, gw)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, gw)
			setGatewayStatusAddresses(ctx, envTest.Client, gw, "203.0.113.51")

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithLoRAAdapters("adapter-1"),
				WithManagedRoute(),
				WithGatewayRefs(LLMGatewayRef(gwName, testNs.Name)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Addresses).ToNot(BeEmpty(), "status.addresses should be populated")

				for _, addr := range current.Status.Addresses {
					g.Expect(addr.Models).ToNot(BeEmpty(), "each address should have models")

					modelNames := make([]string, 0, len(addr.Models))
					for _, m := range addr.Models {
						modelNames = append(modelNames, m.Name)
					}

					if addr.Name != nil && strings.HasSuffix(*addr.Name, "-model-routing") {
						g.Expect(modelNames).To(ContainElement("publishers/" + testNs.Name + "/models/facebook/opt-125m"))
						g.Expect(modelNames).To(ContainElement("publishers/" + testNs.Name + "/models/adapter-1"))
					} else {
						g.Expect(modelNames).To(ContainElement("facebook/opt-125m"))
						g.Expect(modelNames).To(ContainElement("adapter-1"))
					}
				}
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func setGatewayStatusAddresses(ctx context.Context, c client.Client, gw *gwapiv1.Gateway, addresses ...string) {
	current := &gwapiv1.Gateway{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(gw), current)).To(Succeed())

	current.Status.Addresses = make([]gwapiv1.GatewayStatusAddress, len(addresses))
	for i, addr := range addresses {
		current.Status.Addresses[i] = gwapiv1.GatewayStatusAddress{Value: addr}
	}
	Expect(c.Status().Update(ctx, current)).To(Succeed())
}
