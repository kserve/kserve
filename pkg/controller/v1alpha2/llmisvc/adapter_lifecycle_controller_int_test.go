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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService adapter lifecycle", func() {
	It("reports when an adapter warm target is not met", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		llmSvc := LLMInferenceService("adapter-warm-target",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithLoRAAdapters("adapter-warm-target"),
		)
		llmSvc.Spec.Model.LoRA.Adapters[0].MinReplicas = ptr.To(int32(1))

		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(func(g Gomega, ctx context.Context) error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, types.NamespacedName{Namespace: testNs.Name, Name: llmSvc.Name}, current); err != nil {
				return err
			}
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.LoRAAdaptersWarm), "False"))
			condition := current.Status.GetCondition(v1alpha2.LoRAAdaptersWarm)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Reason).To(Equal("WarmTargetNotMet"))
			g.Expect(condition.Message).To(ContainSubstring("minReplicas is 1"))
			return nil
		}).WithContext(ctx).Should(Succeed())
	})

	It("clears the warm condition when minReplicas is unset", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		llmSvc := LLMInferenceService("adapter-warm-target-cleared",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithLoRAAdapters("adapter-warm-target-cleared"),
		)
		llmSvc.Spec.Model.LoRA.Adapters[0].MinReplicas = ptr.To(int32(1))

		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(func(g Gomega, ctx context.Context) error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
				return err
			}
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.LoRAAdaptersWarm), "False"))
			return nil
		}).WithContext(ctx).Should(Succeed())

		current := &v1alpha2.LLMInferenceService{}
		Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		current.Spec.Model.LoRA.Adapters[0].MinReplicas = nil
		Expect(envTest.Update(ctx, current)).To(Succeed())

		Eventually(func(g Gomega, ctx context.Context) error {
			updated := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updated); err != nil {
				return err
			}
			g.Expect(updated.Status.GetCondition(v1alpha2.LoRAAdaptersWarm)).To(BeNil())
			return nil
		}).WithContext(ctx).Should(Succeed())
	})
})
