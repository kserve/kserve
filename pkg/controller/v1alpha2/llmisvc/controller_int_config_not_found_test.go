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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Config resolution", func() {
	Context("when a referenced config does not exist", func() {
		It("should set PresetsCombined=False with reason ConfigNotFound for a missing BaseRef", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cfg-not-found"
			testNs := NewTestNamespace(ctx, envTest)

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
				g.Expect(cond.Message).To(ContainSubstring(`"does-not-exist"`),
					"message should quote the missing config name")
				g.Expect(cond.Message).To(ContainSubstring("not found in namespaces"),
					"message should name the searched namespaces")
				g.Expect(cond.Message).To(ContainSubstring("available configs:"),
					"message should include the available-configs hint (even if empty)")
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
			testNs := NewTestNamespace(ctx, envTest)
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
			// No explicit DeleteAndWait for the config: the service's DeleteAndWait
			// (above) runs first in LIFO order and releases the finalizer reference,
			// then namespace cleanup handles the config deletion.

			// then - watch fires, reconcile runs, condition recovers to True
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should list available configs across service and system namespaces in the message", func(ctx SpecContext) {
			// given - configs seeded in BOTH the service namespace and the
			// system (KServe) namespace; the user-facing payoff of the
			// configNotFoundError Available list is that it surfaces both
			// scopes so operators can see what they could have referenced.
			svcName := "test-llm-cfg-available-list"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			userCfgName := svcName + "-user-cfg"
			systemCfgName := svcName + "-system-cfg"

			userCfg := LLMInferenceServiceConfig(userCfgName,
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			)
			Expect(envTest.Create(ctx, userCfg)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, userCfg)).To(Succeed())
			}()

			systemCfg := LLMInferenceServiceConfig(systemCfgName,
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](constants.KServeNamespace),
			)
			Expect(envTest.Create(ctx, systemCfg)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, systemCfg)).To(Succeed())
			}()

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

			// then - the rendered Available list carries both configs in
			// namespace/name form. Other tests in the suite may have left
			// unrelated configs in KServeNamespace, so we only assert that
			// OUR configs are present, not that they are the only entries.
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "False"))
				cond := current.Status.GetCondition(v1alpha2.PresetsCombined)
				g.Expect(cond.Reason).To(Equal("ConfigNotFound"))
				g.Expect(cond.Message).To(ContainSubstring(
					fmt.Sprintf("%s/%s", testNs.Name, userCfgName)),
					"message should list the service-namespace config in ns/name form")
				g.Expect(cond.Message).To(ContainSubstring(
					fmt.Sprintf("%s/%s", constants.KServeNamespace, systemCfgName)),
					"message should list the system-namespace config in ns/name form")
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should disambiguate same-named configs in different namespaces", func(ctx SpecContext) {
			// given - identical config names in two namespaces; without the
			// namespace qualifier the operator could not tell which one is
			// which from the error message.
			svcName := "test-llm-cfg-same-name-disambig"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))
			sharedName := svcName + "-shared"

			userCfg := LLMInferenceServiceConfig(sharedName,
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			)
			Expect(envTest.Create(ctx, userCfg)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, userCfg)).To(Succeed())
			}()

			systemCfg := LLMInferenceServiceConfig(sharedName,
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](constants.KServeNamespace),
			)
			Expect(envTest.Create(ctx, systemCfg)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, systemCfg)).To(Succeed())
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithBaseRefs(corev1.LocalObjectReference{Name: "does-not-exist"}),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - both occurrences appear in the message, distinguished
			// by their namespace prefix.
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "False"))
				cond := current.Status.GetCondition(v1alpha2.PresetsCombined)
				g.Expect(cond.Reason).To(Equal("ConfigNotFound"))
				g.Expect(cond.Message).To(ContainSubstring(
					fmt.Sprintf("%s/%s", testNs.Name, sharedName)),
					"message should carry the service-namespace copy")
				g.Expect(cond.Message).To(ContainSubstring(
					fmt.Sprintf("%s/%s", constants.KServeNamespace, sharedName)),
					"message should carry the system-namespace copy")
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
