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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Config resolution", func() {
	Context("when a referenced config does not exist", func() {
		It("should set PresetsCombined=False with reason ConfigNotFound for a missing BaseRef", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cfg-not-found"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithBaseRefs(corev1.LocalObjectReference{Name: "does-not-exist"}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - PresetsCombined must be False with ConfigNotFound reason
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "False"))
				cond := current.Status.GetCondition(v1alpha2.PresetsCombined)
				g.Expect(cond.Reason).To(Equal("ConfigNotFound"))
				g.Expect(cond.Message).To(ContainSubstring("does-not-exist"))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// and no Deployment should have been created
			Consistently(func(g Gomega, ctx context.Context) {
				deployment := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).
				Within(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(Succeed())
		})

		It("should recover PresetsCombined to True when the missing BaseRef config is created", func(ctx SpecContext) {
			// given - a service pointing to a config that does not exist yet
			svcName := "test-llm-cfg-not-found-recover"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))
			configName := "recoverable-config"

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithBaseRefs(corev1.LocalObjectReference{Name: configName}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - condition starts as False / ConfigNotFound
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "False"))
				cond := current.Status.GetCondition(v1alpha2.PresetsCombined)
				g.Expect(cond.Reason).To(Equal("ConfigNotFound"))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// when - the missing config is created in the service namespace
			cfg := LLMInferenceServiceConfig(configName,
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			)
			Expect(envTest.Create(ctx, cfg)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, cfg)
			}()

			// then - watch fires, reconcile runs, condition recovers to True
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
