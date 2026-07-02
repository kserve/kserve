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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("Status fields", func() {
		It("should populate status.appliedConfigs with well-known and explicit configs", func(ctx SpecContext) {
			// given
			svcName := "test-llm-applied-configs"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithBaseRefs(corev1.LocalObjectReference{Name: "my-custom-override"}),
			)

			// Create the custom override config
			customCfg := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-custom-override",
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{},
			}
			Expect(envTest.Create(ctx, customCfg)).To(Succeed())

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status.AppliedConfigRefs).NotTo(BeEmpty())

				// Verify preset configs appear first with namespace populated
				var presetCount int
				for _, ac := range current.Status.AppliedConfigRefs {
					if ac.Source == v1alpha2.AppliedConfigSourcePreset {
						presetCount++
						g.Expect(ac.Namespace).NotTo(BeEmpty(), "preset config %q should have namespace set", ac.Name)
					}
				}
				g.Expect(presetCount).To(BeNumerically(">", 0), "should have at least one preset config")

				// Verify explicit user ref appears last with correct namespace
				last := current.Status.AppliedConfigRefs[len(current.Status.AppliedConfigRefs)-1]
				g.Expect(last.Name).To(Equal(gwapiv1.ObjectName("my-custom-override")))
				g.Expect(last.Source).To(Equal(v1alpha2.AppliedConfigSourceUserRef))
				g.Expect(last.Namespace).To(Equal(gwapiv1.Namespace(testNs.Name)))

				// Verify ordering: all Preset before all UserRef
				seenUserRef := false
				for _, ac := range current.Status.AppliedConfigRefs {
					if ac.Source == v1alpha2.AppliedConfigSourceUserRef {
						seenUserRef = true
					}
					if seenUserRef {
						g.Expect(ac.Source).To(Equal(v1alpha2.AppliedConfigSourceUserRef),
							"preset config %q appeared after a UserRef", ac.Name)
					}
				}
			})).WithContext(ctx).Should(Succeed())
		})

		It("should preserve pinned config annotations across reconciliations", func(ctx SpecContext) {
			// given
			svcName := "test-llm-pinning-stable"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - wait for initial reconciliation to populate Status.Annotations.
			// We check for PresetsCombined=True which indicates config merging completed,
			// without requiring the full router readiness flow.
			var pinnedAnnotations map[string]string
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
				g.Expect(current.Status.Annotations).NotTo(BeNil())
				pinnedAnnotations = make(map[string]string, len(current.Status.Annotations))
				for k, v := range current.Status.Annotations {
					pinnedAnnotations[k] = v
				}
				return nil
			}).WithContext(ctx).Should(Succeed())

			// Trigger a new reconciliation by changing a spec field.
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Model.Name = ptr.To(*llmSvc.Spec.Model.Name + "-v2")
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Verify annotations are stable after re-reconciliation.
			// Use Consistently to confirm they remain unchanged over a short observation window.
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Annotations).NotTo(BeNil())
				for key, wantValue := range pinnedAnnotations {
					g.Expect(current.Status.Annotations).To(
						HaveKeyWithValue(key, wantValue),
						fmt.Sprintf("pinned annotation %q should be stable across reconciliations", key),
					)
				}
			}).WithContext(ctx).Should(Succeed())
		})

		It("should retain status.appliedConfigs when a referenced config deletion is blocked by finalizer", func(ctx SpecContext) {
			// The LLMISVCConfigReconciler adds a finalizer to configs referenced by services,
			// preventing deletion while in use. This test verifies that appliedConfigs remain
			// stable when a config deletion is requested but blocked.
			// given
			svcName := "test-llm-applied-cleared"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithBaseRefs(corev1.LocalObjectReference{Name: "ephemeral-config"}),
			)

			ephemeralCfg := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ephemeral-config",
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{},
			}
			Expect(envTest.Create(ctx, ephemeralCfg)).To(Succeed())

			// when - create the service and wait for successful reconciliation
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status.AppliedConfigRefs).NotTo(BeEmpty(), "appliedConfigs should be populated after successful reconciliation")
			})).WithContext(ctx).Should(Succeed())

			// when - request deletion of the config (finalizer blocks actual removal)
			Expect(envTest.Delete(ctx, ephemeralCfg)).To(Succeed())

			// then - config should still exist with DeletionTimestamp (finalizer blocks)
			Eventually(func(g Gomega, ctx context.Context) {
				cfg := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(ephemeralCfg), cfg)).To(Succeed())
				g.Expect(cfg.DeletionTimestamp).NotTo(BeNil(), "config should have DeletionTimestamp")
			}).WithContext(ctx).Should(Succeed())

			// and - appliedConfigs should remain populated since the config is still available
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.AppliedConfigRefs).NotTo(BeEmpty(),
					"appliedConfigs should remain populated when config deletion is blocked by finalizer")
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
			}).WithContext(ctx).
				WithTimeout(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(Succeed())
		})
	})
})
