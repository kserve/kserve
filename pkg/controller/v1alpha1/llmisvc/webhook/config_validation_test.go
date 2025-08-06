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
	"testing"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"

	"github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/webhook"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestLLMInferenceServiceConfigValidator_ValidateUpdate_Warnings(t *testing.T) {
	// Get a well-known config name from the exported set
	wellKnownConfigName := llmisvc.WellKnownDefaultConfigs.UnsortedList()[0]

	tests := []struct {
		name         string
		oldConfig    *v1alpha1.LLMInferenceServiceConfig
		newConfig    *v1alpha1.LLMInferenceServiceConfig
		wantWarnings int
		wantWarning  string
	}{
		{
			name: "updating well-known config should return warning",
			oldConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellKnownConfigName,
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("original-model"),
					},
				},
			},
			newConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellKnownConfigName,
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("updated-model"),
					},
				},
			},
			wantWarnings: 1,
			wantWarning:  "not recommended",
		},
		{
			name: "updating non-well-known config should not return warning",
			oldConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-config",
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("original-model"),
					},
				},
			},
			newConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-config",
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("updated-model"),
					},
				},
			},
			wantWarnings: 0,
		},
		{
			name: "updating well-known config with same spec should not return warning",
			oldConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellKnownConfigName,
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("same-model"),
					},
				},
			},
			newConfig: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellKnownConfigName,
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("same-model"),
					},
				},
			},
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			validator := setupValidator(t)
			ctx := t.Context()

			warnings, err := validator.ValidateUpdate(ctx, tt.oldConfig, tt.newConfig)

			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			g.Expect(warnings).Should(gomega.HaveLen(tt.wantWarnings))

			if tt.wantWarning != "" {
				g.Expect(warnings).Should(gomega.Not(gomega.BeEmpty()))
				g.Expect(warnings[0]).Should(gomega.ContainSubstring(tt.wantWarning))
			}
		})
	}
}

func TestLLMInferenceServiceConfigValidator_ValidateDelete_Warnings(t *testing.T) {
	// Get a well-known config name from the exported set
	wellKnownConfigName := llmisvc.WellKnownDefaultConfigs.UnsortedList()[1]

	tests := []struct {
		name         string
		config       *v1alpha1.LLMInferenceServiceConfig
		wantWarnings int
		wantWarning  string
	}{
		{
			name: "deleting well-known config should return warning",
			config: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellKnownConfigName,
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("test-model"),
					},
				},
			},
			wantWarnings: 1,
			wantWarning:  "not recommended",
		},
		{
			name: "deleting non-well-known config should not return warning",
			config: &v1alpha1.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-config",
					Namespace: constants.KServeNamespace,
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					Model: v1alpha1.LLMModelSpec{
						Name: ptr.To("test-model"),
					},
				},
			},
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			validator := setupValidator(t)
			ctx := t.Context()

			warnings, err := validator.ValidateDelete(ctx, tt.config)

			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			g.Expect(warnings).Should(gomega.HaveLen(tt.wantWarnings))

			if tt.wantWarning != "" {
				g.Expect(warnings).Should(gomega.Not(gomega.BeEmpty()))
				g.Expect(warnings[0]).Should(gomega.ContainSubstring(tt.wantWarning))
			}
		})
	}
}

func setupValidator(t *testing.T) *webhook.LLMInferenceServiceConfigValidator {
	clientset := fake.NewClientset()

	namespace := constants.KServeNamespace
	configMap := fixture.InferenceServiceCfgMap(namespace)

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(
		t.Context(), configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create inference service configmap: %v", err)
	}

	return &webhook.LLMInferenceServiceConfigValidator{
		ClientSet: clientset,
	}
}
