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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("Validating config configs", func() {
	Context("validating new configs", func() {
		It("should reject config with invalid template fields", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("invalid-template-fields",
				fixture.InNamespace[*v1alpha2.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("{{ .NonExisting }}"),
			)

			// when
			admissionError := envTest.Create(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring("can't evaluate field NonExisting in type struct"))
		})

		It("should reject updating config with wrong template syntax", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("invalid-template-fields",
				fixture.InNamespace[*v1alpha2.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("{{ ChildName .ObjectMeta.Name `-inference-pool` }}"),
			)
			Expect(envTest.Client.Create(ctx, preset)).To(Succeed())

			// Wait for the controller to add the finalizer so the
			// resourceVersion is stable before we attempt the update.
			Eventually(func(g Gomega) {
				g.Expect(envTest.Client.Get(ctx, types.NamespacedName{Name: preset.Name, Namespace: preset.Namespace}, preset)).To(Succeed())
				g.Expect(preset.Finalizers).NotTo(BeEmpty())
			}).Should(Succeed())

			// when
			preset.Spec.Model.Name = ptr.To("{{ ChildName .ObjectMeta.Name \"-inference-pool\" }}")
			admissionError := envTest.Update(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring(`unexpected "\\" in operand`))
		})

		It("should reject config with baseRefs", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("config-with-baserefs",
				fixture.InNamespace[*v1alpha2.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("test-model"),
			)

			preset.Spec.BaseRefs = []corev1.LocalObjectReference{
				{Name: "base-config"},
			}

			// when
			admissionError := envTest.Create(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring("spec.baseRefs"))
		})

		It("should reject a rendered config that violates the service CRD schema", func(ctx SpecContext) {
			testNs := fixture.NewTestNamespace(ctx, envTest)
			const (
				configName = "invalid-rendered-header"
				svcName    = "invalid-rendered-config"
			)
			preset := fixture.LLMInferenceServiceConfig(configName,
				fixture.InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				func(config *v1alpha2.LLMInferenceServiceConfig) {
					config.Spec.Router = &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{
								Spec: &gwapiv1.HTTPRouteSpec{
									Rules: []gwapiv1.HTTPRouteRule{{
										Matches: []gwapiv1.HTTPRouteMatch{
											fixture.HeaderOnlyMatch("{{ .ObjectMeta.Name }} invalid", "test-model"),
										},
									}},
								},
							},
						},
					}
				},
			)
			Expect(envTest.Create(ctx, preset)).To(Succeed(),
				"the Config CRD must accept header-name templates before rendering")

			llmSvc := fixture.LLMInferenceService(svcName,
				fixture.InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithBaseRefs(corev1.LocalObjectReference{Name: configName}),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, llmSvc)

			Eventually(func(g Gomega) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{Name: svcName, Namespace: testNs.Name}, current)).To(Succeed())
				condition := current.Status.GetCondition(v1alpha2.PresetsCombined)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Reason).To(Equal("InvalidRenderedConfig"))
				g.Expect(condition.Message).To(ContainSubstring("headers[0].name"))
				g.Expect(condition.Message).To(ContainSubstring("Invalid value"))

				// PresetsCombined counts towards Ready, so the failure must show there too.
				ready := current.Status.GetCondition(apis.ConditionReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.IsFalse()).To(BeTrue())
				g.Expect(ready.Reason).To(Equal("InvalidRenderedConfig"))
			}).Should(Succeed())

			Consistently(func() bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, &appsv1.Deployment{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue(), "invalid rendered config must not leave partial child resources")
		})
	})
})
