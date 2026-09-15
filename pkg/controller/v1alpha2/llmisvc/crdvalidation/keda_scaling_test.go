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

package crdvalidation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const kedaValidationNamespace = "keda-crd-validation"

var _ = Describe("KEDA scaling CRD validation", func() {
	BeforeEach(func(ctx SpecContext) {
		Expect(envTest.Client.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: kedaValidationNamespace},
		})).To(Or(Succeed(), MatchError(ContainSubstring("already exists"))))
	})

	DescribeTable("accepts HPA behavior when controller-owned optional fields are omitted",
		func(ctx SpecContext, version, kind, name string) {
			resource := kedaScalingResource(version, kind, name, map[string]any{
				"horizontalPodAutoscalerConfig": map[string]any{
					"behavior": map[string]any{
						"scaleDown": map[string]any{
							"stabilizationWindowSeconds": int64(300),
						},
					},
				},
			})

			Expect(envTest.Client.Create(ctx, resource)).To(Succeed())
		},
		Entry("for v1alpha2 LLMInferenceService", "v1alpha2", "LLMInferenceService", "keda-behavior-service"),
		Entry("for v1alpha2 LLMInferenceServiceConfig", "v1alpha2", "LLMInferenceServiceConfig", "keda-behavior-config"),
		Entry("for v1alpha1 LLMInferenceService", "v1alpha1", "LLMInferenceService", "keda-behavior-service-v1alpha1"),
		Entry("for v1alpha1 LLMInferenceServiceConfig", "v1alpha1", "LLMInferenceServiceConfig", "keda-behavior-config-v1alpha1"),
	)

	DescribeTable("rejects user-configured scaling modifiers",
		func(ctx SpecContext, version, name, field, value string) {
			resource := kedaScalingResource(version, "LLMInferenceService", name, map[string]any{
				"scalingModifiers": map[string]any{field: value},
			})

			err := envTest.Client.Create(ctx, resource)
			Expect(err).To(MatchError(ContainSubstring(
				"scalingModifiers must not be set; WVA controls the scaling metric formula and logic")))
		},
		Entry("formula", "v1alpha2", "keda-scaling-modifier-formula", "formula", "wva_desired_replicas"),
		Entry("target", "v1alpha2", "keda-scaling-modifier-target", "target", "1"),
		Entry("activation target", "v1alpha2", "keda-scaling-modifier-activation-target", "activationTarget", "1"),
		Entry("metric type", "v1alpha2", "keda-scaling-modifier-metric-type", "metricType", "Value"),
		Entry("formula (v1alpha1)", "v1alpha1", "keda-scaling-modifier-formula-v1alpha1", "formula", "wva_desired_replicas"),
		Entry("target (v1alpha1)", "v1alpha1", "keda-scaling-modifier-target-v1alpha1", "target", "1"),
		Entry("activation target (v1alpha1)", "v1alpha1", "keda-scaling-modifier-activation-target-v1alpha1", "activationTarget", "1"),
		Entry("metric type (v1alpha1)", "v1alpha1", "keda-scaling-modifier-metric-type-v1alpha1", "metricType", "Value"),
	)

	DescribeTable("rejects a user-configured HPA name",
		func(ctx SpecContext, version, name string) {
			resource := kedaScalingResource(version, "LLMInferenceService", name, map[string]any{
				"horizontalPodAutoscalerConfig": map[string]any{
					"name": "user-managed-hpa",
				},
			})

			err := envTest.Client.Create(ctx, resource)
			Expect(err).To(MatchError(ContainSubstring(
				"horizontalPodAutoscalerConfig.name must not be set; the controller manages the HPA name")))
		},
		Entry("v1alpha2", "v1alpha2", "keda-hpa-name"),
		Entry("v1alpha1", "v1alpha1", "keda-hpa-name-v1alpha1"),
	)
})

func kedaScalingResource(version, kind, name string, advanced map[string]any) *unstructured.Unstructured {
	resource := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "serving.kserve.io/" + version,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": kedaValidationNamespace,
		},
		"spec": map[string]any{
			"model": map[string]any{
				"uri": "gs://dummy",
			},
			"scaling": map[string]any{
				"maxReplicas": int64(5),
				"wva": map[string]any{
					"keda": map[string]any{
						"advanced": advanced,
					},
				},
			},
		},
	}}
	resource.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "serving.kserve.io", Version: version, Kind: kind,
	})
	return resource
}
