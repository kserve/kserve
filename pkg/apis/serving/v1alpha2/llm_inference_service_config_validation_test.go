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

package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
)

// Adapters declared in an LLMInferenceServiceConfig are merged into the service spec
// and reach the controller unchanged. LLMInferenceServiceValidator only ever sees the
// unmerged service, so the config validator is the only place they can be caught.
func TestValidateLoRAAdaptersFromConfig(t *testing.T) {
	t.Parallel()

	validator := &LLMInferenceServiceConfigValidator{}

	makeConfig := func(modelName *string, adapters ...LLMModelSpec) *LLMInferenceServiceConfig {
		return &LLMInferenceServiceConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "preset", Namespace: "default"},
			Spec: LLMInferenceServiceSpec{
				Model: LLMModelSpec{
					URI:  apis.URL{Scheme: "hf", Host: "base-model"},
					Name: modelName,
					LoRA: &LoRASpec{Adapters: adapters},
				},
			},
		}
	}
	adapter := func(name, host string) LLMModelSpec {
		return LLMModelSpec{URI: apis.URL{Scheme: "hf", Host: host}, Name: ptr.To(name)}
	}

	t.Run("no lora", func(t *testing.T) {
		t.Parallel()
		config := makeConfig(nil)
		config.Spec.Model.LoRA = nil
		require.NoError(t, validator.validate(t.Context(), config))
	})

	t.Run("valid adapters", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validator.validate(t.Context(),
			makeConfig(nil, adapter("adapter-1", "a1"), adapter("adapter-2", "a2"))))
	})

	t.Run("duplicate adapter names", func(t *testing.T) {
		t.Parallel()
		err := validator.validate(t.Context(),
			makeConfig(nil, adapter("dup", "a1"), adapter("dup", "a2")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.lora.adapters[1].name")
	})

	t.Run("adapter name missing", func(t *testing.T) {
		t.Parallel()
		err := validator.validate(t.Context(),
			makeConfig(nil, LLMModelSpec{URI: apis.URL{Scheme: "hf", Host: "a1"}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.lora.adapters[0].name")
	})

	t.Run("adapter name is path traversal", func(t *testing.T) {
		t.Parallel()
		err := validator.validate(t.Context(), makeConfig(nil, adapter("..", "a1")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.lora.adapters[0].name")
	})

	t.Run("maxRank bound", func(t *testing.T) {
		t.Parallel()
		config := makeConfig(nil, adapter("adapter-1", "a1"))
		config.Spec.Model.LoRA.MaxRank = ptr.To(int32(0))
		err := validator.validate(t.Context(), config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.lora.maxRank")
	})

	// The base model name is supplied by the LLMInferenceService at merge time. A config
	// that does not set one must not have its own object name stand in, or an adapter
	// legitimately named after the preset would be rejected.
	t.Run("adapter named after the config object is allowed", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validator.validate(t.Context(), makeConfig(nil, adapter("preset", "a1"))))
	})

	t.Run("adapter colliding with an explicit base model name is rejected", func(t *testing.T) {
		t.Parallel()
		err := validator.validate(t.Context(), makeConfig(ptr.To("base"), adapter("base", "a1")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.lora.adapters[0].name")
	})
}
