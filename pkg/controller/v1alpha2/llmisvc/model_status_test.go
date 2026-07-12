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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestObserveModelStatus(t *testing.T) {
	tests := []struct {
		name     string
		service  *v1alpha2.LLMInferenceService
		model    *v1alpha2.LLMModelSpec
		expected *v1alpha2.ObservedModelStatus
	}{
		{
			name: "uses the resolved model and adapter names",
			service: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "service-name"},
			},
			model: &v1alpha2.LLMModelSpec{
				Name: ptr.To("base-model"),
				LoRA: &v1alpha2.LoRASpec{Adapters: []v1alpha2.LLMModelSpec{
					{Name: ptr.To("adapter-b")},
					{Name: ptr.To("adapter-a")},
				}},
			},
			expected: &v1alpha2.ObservedModelStatus{
				Name:     "base-model",
				Adapters: []string{"adapter-a", "adapter-b"},
			},
		},
		{
			name: "defaults an omitted model name to the service name",
			service: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "service-name"},
			},
			model:    &v1alpha2.LLMModelSpec{},
			expected: &v1alpha2.ObservedModelStatus{Name: "service-name"},
		},
		{
			name: "skips adapters with nil names (validation bypass)",
			service: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "service-name"},
			},
			model: &v1alpha2.LLMModelSpec{
				Name: ptr.To("base-model"),
				LoRA: &v1alpha2.LoRASpec{Adapters: []v1alpha2.LLMModelSpec{
					{Name: ptr.To("valid-adapter")},
					{Name: nil},
				}},
			},
			expected: &v1alpha2.ObservedModelStatus{
				Name:     "base-model",
				Adapters: []string{"valid-adapter"},
			},
		},
		{
			name: "keeps adapters nil when LoRA has no configured adapters",
			service: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "service-name"},
			},
			model: &v1alpha2.LLMModelSpec{
				Name: ptr.To("base-model"),
				LoRA: &v1alpha2.LoRASpec{},
			},
			expected: &v1alpha2.ObservedModelStatus{Name: "base-model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observeModelStatus(tt.service, tt.model)
			assert.Equal(t, tt.expected, tt.service.Status.Model)
		})
	}
}
