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

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"
)

var _ = Describe("Validating config configs", func() {
	Context("validating new configs", func() {
		It("should reject config with invalid template fields", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("invalid-template-fields",
				fixture.InNamespace[*v1alpha1.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("{{ .NonExisting }}"),
			)

			// when
			admissionError := envTest.Client.Create(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring("can't evaluate field NonExisting in type struct"))
		})

		It("should reject updating config with wrong template syntax", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("invalid-template-fields",
				fixture.InNamespace[*v1alpha1.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("{{ ChildName .ObjectMeta.Name `-inference-pool` }}"),
			)
			Expect(envTest.Client.Create(ctx, preset)).To(Succeed())

			// when
			preset.Spec.Model.Name = ptr.To("{{ ChildName .ObjectMeta.Name \"-inference-pool\" }}")
			admissionError := envTest.Client.Update(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring(`unexpected "\\" in operand`))
		})

		It("should reject config with baseRefs", func(ctx SpecContext) {
			// given
			preset := fixture.LLMInferenceServiceConfig("config-with-baserefs",
				fixture.InNamespace[*v1alpha1.LLMInferenceServiceConfig](constants.KServeNamespace),
				fixture.WithConfigModelName("test-model"),
			)

			preset.Spec.BaseRefs = []corev1.LocalObjectReference{
				{Name: "base-config"},
			}

			// when
			admissionError := envTest.Client.Create(ctx, preset)

			// then
			Expect(admissionError).To(HaveOccurred())
			Expect(admissionError.Error()).To(ContainSubstring("spec.baseRefs is not a permitted field in LLMInferenceServiceConfig"))
		})
	})
})
