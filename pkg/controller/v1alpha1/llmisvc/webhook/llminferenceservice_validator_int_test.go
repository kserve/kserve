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

package webhook_test

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService webhook validation", func() {
	var (
		ns        *corev1.Namespace
		nsName    string
		gateway   *gatewayapi.Gateway
		httpRoute *gatewayapi.HTTPRoute
	)

	BeforeEach(func(ctx SpecContext) {
		nsName = fmt.Sprintf("test-llmisvc-validation-%d", time.Now().UnixNano())

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		Expect(envTest.Client.Create(ctx, ns)).To(Succeed())

		gateway = fixture.Gateway("test-gateway",
			fixture.InNamespace[*gatewayapi.Gateway](nsName),
			fixture.WithClassName("test-gateway-class"),
			fixture.WithListener(gatewayapi.HTTPProtocolType),
		)
		Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())

		httpRoute = fixture.HTTPRoute("test-route",
			fixture.InNamespace[*gatewayapi.HTTPRoute](nsName),
			fixture.WithParentRef(fixture.GatewayRef("test-gateway")),
			fixture.WithPath("/test"),
		)
		Expect(envTest.Client.Create(ctx, httpRoute)).To(Succeed())

		DeferCleanup(func() {
			httpRoute := httpRoute
			gateway := gateway
			ns := ns
			envTest.DeleteAll(httpRoute, gateway, ns)
		})
	})

	Context("cross-field constraints validation", func() {
		It("should reject LLMInferenceService with both refs and spec in HTTPRoute", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-both-refs-and-spec",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithHTTPRouteRefs(fixture.HTTPRouteRef("test-route")),
				fixture.WithHTTPRouteSpec(&fixture.HTTPRoute("test-route",
					fixture.WithHTTPRule(
						fixture.Matches(fixture.PathPrefixMatch("/test")),
						fixture.BackendRefs(fixture.ServiceRef("test-service", 80, 1)),
					),
				).Spec),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("unsupported configuration"))
		})

		It("should reject LLMInferenceService with user-defined routes and managed gateway", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-refs-with-managed-gateway",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithHTTPRouteRefs(fixture.HTTPRouteRef("test-route")),
				fixture.WithManagedGateway(),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot be used with a managed gateway"))
		})

		It("should reject LLMInferenceService with managed route and user-defined gateway refs", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-spec-with-gateway-refs",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithGatewayRefs(fixture.LLMGatewayRef("test-gateway", nsName)),
				fixture.WithManagedRoute(),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot be used with managed route"))
		})

		It("should reject LLMInferenceService with managed route spec and user-defined gateway refs", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-spec-with-gateway-refs",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithGatewayRefs(fixture.LLMGatewayRef("test-gateway", nsName)),
				fixture.WithHTTPRouteSpec(&fixture.HTTPRoute("test-route",
					fixture.WithHTTPRule(
						fixture.Matches(fixture.PathPrefixMatch("/test")),
						fixture.BackendRefs(fixture.ServiceRef("custom-backend", 8080, 1)),
						fixture.Timeouts("30s", "60s"),
					),
				).Spec),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("unsupported configuration"))
		})
	})
})
