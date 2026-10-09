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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

const configFinalizerName = constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"

var _ = Describe("LLMInferenceServiceConfig Controller", func() {
	Context("Finalizer management", func() {
		It("should add a finalizer and mark Ready on a new LLMInferenceServiceConfig", func(ctx SpecContext) {
			// given
			testNs := NewTestNamespace(ctx, envTest)

			config := LLMInferenceServiceConfig("my-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			)

			// when
			Expect(envTest.Create(ctx, config)).To(Succeed())

			// then
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue(),
					"expected finalizer %q to be present on LLMInferenceServiceConfig", configFinalizerName)

				readyCond := current.GetStatus().GetCondition(apis.ConditionReady)
				g.Expect(readyCond).ToNot(BeNil(), "expected Ready condition to be set")
				g.Expect(readyCond.IsTrue()).To(BeTrue(), "expected Ready condition to be True")

				inUseCond := current.GetStatus().GetCondition(v1alpha2.ConfigInUseConditionType)
				g.Expect(inUseCond).ToNot(BeNil(), "expected ConfigInUse condition to be set")
				g.Expect(inUseCond.IsFalse()).To(BeTrue(), "expected ConfigInUse=False when not referenced")

				g.Expect(current.Status.ReferencedBy).To(BeEmpty(), "expected no ReferencedBy when not referenced")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should not take ownership of server-side applied spec fields when adding the finalizer", func(ctx SpecContext) {
			// given - a config applied by another field manager, with a CPU quantity sent as a JSON number
			testNs := NewTestNamespace(ctx, envTest)
			applied := ssaConfigWithCPU("ssa-add-config", testNs.Name)
			Expect(envTest.Apply(ctx, client.ApplyConfigurationFromUnstructured(applied.DeepCopy()), client.FieldOwner(ssaFieldOwner))).To(Succeed())

			// when - the controller adds its finalizer
			key := client.ObjectKeyFromObject(applied)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, key, current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// then - the field manager can re-apply the unchanged object without conflicts
			Expect(envTest.Apply(ctx, client.ApplyConfigurationFromUnstructured(applied.DeepCopy()), client.FieldOwner(ssaFieldOwner))).To(Succeed())

			current := &v1alpha2.LLMInferenceServiceConfig{}
			Expect(envTest.Get(ctx, key, current)).To(Succeed())
			expectOnlyOwnerManagesSpec(current)
		})

		It("should not take ownership of server-side applied spec fields when removing the finalizer", func(ctx SpecContext) {
			// given - a server-side applied config carrying a test-owned finalizer that keeps it
			// around after the controller releases its own
			testNs := NewTestNamespace(ctx, envTest)
			applied := ssaConfigWithCPU("ssa-remove-config", testNs.Name)
			applied.SetFinalizers([]string{testKeepFinalizer})
			Expect(envTest.Apply(ctx, client.ApplyConfigurationFromUnstructured(applied.DeepCopy()), client.FieldOwner(ssaFieldOwner))).To(Succeed())

			key := client.ObjectKeyFromObject(applied)
			defer func() {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				if err := envTest.Get(ctx, key, current); err != nil {
					return
				}
				Expect(envTest.Patch(ctx, current, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`)))).To(Succeed())
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, key, current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// when - the unreferenced config is deleted
			Expect(envTest.Delete(ctx, &v1alpha2.LLMInferenceServiceConfig{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}})).To(Succeed())

			// then - the controller releases its finalizer without touching spec ownership
			current := &v1alpha2.LLMInferenceServiceConfig{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, key, current)).To(Succeed())
				g.Expect(current.DeletionTimestamp).ToNot(BeNil())
				g.Expect(current.Finalizers).To(ConsistOf(testKeepFinalizer))
			}).WithContext(ctx).Should(Succeed())

			expectOnlyOwnerManagesSpec(current)
		})
	})

	Context("Deletion protection", func() {
		It("should block deletion when config is referenced via spec.baseRefs", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cfg-protect-baserefs"
			testNs := NewTestNamespace(ctx, envTest)

			config := LLMInferenceServiceConfig("referenced-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)
			Expect(envTest.Create(ctx, config)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(corev1.LocalObjectReference{Name: "referenced-config"}),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the finalizer to be added
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// when - attempt to delete the config
			Expect(envTest.Delete(ctx, config)).To(Succeed())

			// then - config should still exist (finalizer blocks deletion),
			// Ready should remain True, and ConfigInUse should be True with reason DeletionBlocked
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(current.DeletionTimestamp).ToNot(BeNil(),
					"config should have a deletion timestamp")
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue(),
					"finalizer should still be present while config is referenced")

				readyCond := current.GetStatus().GetCondition(apis.ConditionReady)
				g.Expect(readyCond).ToNot(BeNil(), "expected Ready condition to be set")
				g.Expect(readyCond.IsTrue()).To(BeTrue(), "expected Ready=True even when deletion is blocked")

				inUseCond := current.GetStatus().GetCondition(v1alpha2.ConfigInUseConditionType)
				g.Expect(inUseCond).ToNot(BeNil(), "expected ConfigInUse condition to be set")
				g.Expect(inUseCond.IsTrue()).To(BeTrue(), "expected ConfigInUse=True when deletion is blocked")
				g.Expect(inUseCond.Reason).To(Equal("DeletionBlocked"))

				g.Expect(current.Status.ReferencedBy).ToNot(BeEmpty(), "expected ReferencedBy to list referencing services")
				g.Expect(current.Status.ReferencedBy).To(ContainElement(
					v1alpha2.UntypedObjectReference{
						Name:      gwapiv1.ObjectName(svcName),
						Namespace: gwapiv1.Namespace(current.Namespace),
					},
				))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should block deletion when config is referenced via status annotations", func(ctx SpecContext) {
			// given — create config and service, let the reconciler pin the config
			// in status.annotations via versioned config resolution
			svcName := "test-llm-cfg-protect-annot"
			testNs := NewTestNamespace(ctx, envTest)

			config := LLMInferenceServiceConfig("pinned-cfg",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)
			Expect(envTest.Create(ctx, config)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(corev1.LocalObjectReference{Name: "pinned-cfg"}),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the service to reconcile and pin the config in status annotations
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Annotations).ToNot(BeNil(),
					"status annotations should be populated after reconciliation")
			}).WithContext(ctx).Should(Succeed())

			// Wait for finalizer on config
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Remove the baseRef but keep the pinned status annotation
			// The service still has the config name pinned in status.annotations
			// when - attempt to delete the config
			Expect(envTest.Delete(ctx, config)).To(Succeed())

			// then - config should still exist (referenced via status annotations),
			// Ready should remain True, and ConfigInUse should be True with reason DeletionBlocked
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(current.DeletionTimestamp).ToNot(BeNil())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue(),
					"finalizer should still be present while config is pinned in status annotations")

				readyCond := current.GetStatus().GetCondition(apis.ConditionReady)
				g.Expect(readyCond).ToNot(BeNil(), "expected Ready condition to be set")
				g.Expect(readyCond.IsTrue()).To(BeTrue(), "expected Ready=True even when deletion is blocked")

				inUseCond := current.GetStatus().GetCondition(v1alpha2.ConfigInUseConditionType)
				g.Expect(inUseCond).ToNot(BeNil(), "expected ConfigInUse condition to be set")
				g.Expect(inUseCond.IsTrue()).To(BeTrue(), "expected ConfigInUse=True when deletion is blocked")
				g.Expect(inUseCond.Reason).To(Equal("DeletionBlocked"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should allow deletion when config is not referenced by any service", func(ctx SpecContext) {
			// given
			testNs := NewTestNamespace(ctx, envTest)

			config := LLMInferenceServiceConfig("unreferenced-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			)
			Expect(envTest.Create(ctx, config)).To(Succeed())

			// Wait for finalizer to be added
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// when - delete the config (no service references it)
			Expect(envTest.Delete(ctx, config)).To(Succeed())

			// then - config should be fully deleted (finalizer removed, object gone)
			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, client.ObjectKeyFromObject(config), &v1alpha2.LLMInferenceServiceConfig{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"config should be deleted when not referenced, got: %v", err)
			}).WithContext(ctx).Should(Succeed())
		})

		It("should unblock deletion after referencing service is deleted", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cfg-unblock"
			testNs := NewTestNamespace(ctx, envTest)

			config := LLMInferenceServiceConfig("blocking-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)
			Expect(envTest.Create(ctx, config)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(corev1.LocalObjectReference{Name: "blocking-config"}),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())

			// Wait for finalizer on config
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(current, configFinalizerName)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Attempt to delete the config — should be blocked
			Expect(envTest.Delete(ctx, config)).To(Succeed())

			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceServiceConfig{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(config), current)).To(Succeed())
				g.Expect(current.DeletionTimestamp).ToNot(BeNil())
			}).WithContext(ctx).Should(Succeed())

			// when - delete the referencing service
			testNs.DeleteAndWait(ctx, llmSvc)

			// then - config should now be fully deleted
			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, client.ObjectKeyFromObject(config), &v1alpha2.LLMInferenceServiceConfig{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"config should be deleted after referencing service is removed, got: %v", err)
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

const (
	ssaFieldOwner     = "helm"
	testKeepFinalizer = "test.kserve.io/keep"
)

// ssaConfigWithCPU returns a config as an installer would server-side apply it, with the
// CPU quantity as a JSON number. The controller's typed client re-encodes it as a string,
// so any full-object write from the controller changes the value and takes ownership of it.
func ssaConfigWithCPU(name, namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "main",
						"image": "vllm/vllm-openai:latest",
						"resources": map[string]any{
							"limits":   map[string]any{"cpu": int64(6)},
							"requests": map[string]any{"cpu": int64(6)},
						},
					},
				},
			},
		},
	}}
	u.SetGroupVersionKind(v1alpha2.LLMInferenceServiceConfigGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	return u
}

// expectOnlyOwnerManagesSpec asserts that no field manager other than ssaFieldOwner owns any spec field.
func expectOnlyOwnerManagesSpec(config *v1alpha2.LLMInferenceServiceConfig) {
	GinkgoHelper()
	for _, entry := range config.ManagedFields {
		if entry.Manager == ssaFieldOwner || entry.Subresource != "" || entry.FieldsV1 == nil {
			continue
		}
		fields := map[string]any{}
		Expect(json.Unmarshal(entry.FieldsV1.Raw, &fields)).To(Succeed())
		Expect(fields).ToNot(HaveKey("f:spec"),
			"field manager %q (%s) unexpectedly owns spec fields: %s", entry.Manager, entry.Operation, entry.FieldsV1.Raw)
	}
}
