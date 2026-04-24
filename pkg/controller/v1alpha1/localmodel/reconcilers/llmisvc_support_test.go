/*
Copyright 2024 The KServe Authors.

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

package reconcilers

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestHasLLMInferenceServiceResource(t *testing.T) {
	t.Run("returns false when llmisvc resource is not mapped", func(t *testing.T) {
		restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1alpha2.SchemeGroupVersion})

		ok, err := hasLLMInferenceServiceResource(restMapper)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ok {
			t.Fatalf("expected llmisvc resource to be unavailable")
		}
	})

	t.Run("returns true when llmisvc resource is mapped", func(t *testing.T) {
		restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1alpha2.SchemeGroupVersion})
		restMapper.Add(v1alpha2.SchemeGroupVersion.WithKind("LLMInferenceService"), meta.RESTScopeNamespace)

		ok, err := hasLLMInferenceServiceResource(restMapper)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ok {
			t.Fatalf("expected llmisvc resource to be available")
		}
	})
}
