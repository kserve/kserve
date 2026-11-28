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

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

func TestLocalModelCachePolicyRequired(t *testing.T) {
	scenarios := map[string]struct {
		value       string
		present     bool // annotation present on the workload
		expectCache bool
	}{
		"unset":                  {present: false},
		"empty":                  {value: "", present: true},
		"whitespace only":        {value: "   ", present: true},
		"fallback":               {value: "FallbackToSource", present: true, expectCache: false},
		"fallback lowercase":     {value: "fallbacktosource", present: true, expectCache: false},
		"cache required":         {value: "CacheRequired", present: true, expectCache: true},
		"cache required trimmed": {value: "  CacheRequired  ", present: true, expectCache: true},
		"cache required case":    {value: "cacherequired", present: true, expectCache: true},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			annotations := map[string]string{}
			if scenario.present {
				annotations[constants.LocalModelCachePolicyAnnotationKey] = scenario.value
			}
			required, err := LocalModelCachePolicyRequired(annotations)
			assert.NoError(t, err)
			assert.Equal(t, scenario.expectCache, required)
		})
	}

	t.Run("malformed fails closed", func(t *testing.T) {
		required, err := LocalModelCachePolicyRequired(map[string]string{
			constants.LocalModelCachePolicyAnnotationKey: "CacheRequire",
		})
		assert.Error(t, err)
		// A malformed non-empty value is treated as CacheRequired at runtime,
		// never as a silent source fallback.
		assert.True(t, required)
	})

	t.Run("malformed overrides other annotations", func(t *testing.T) {
		required, err := LocalModelCachePolicyRequired(map[string]string{
			"other": "value",
			constants.LocalModelCachePolicyAnnotationKey: "required",
		})
		assert.Error(t, err)
		assert.True(t, required)
	})

	t.Run("nil annotations", func(t *testing.T) {
		required, err := LocalModelCachePolicyRequired(nil)
		assert.NoError(t, err)
		assert.False(t, required)
	})
}

func TestValidateLocalModelCachePolicyAnnotation(t *testing.T) {
	path := field.NewPath("metadata", "annotations")

	t.Run("unset is the default and valid", func(t *testing.T) {
		assert.Empty(t, ValidateLocalModelCachePolicyAnnotation(nil, path))
	})

	t.Run("both policy values are valid, case- and space-insensitively", func(t *testing.T) {
		for _, value := range []string{
			constants.LocalModelCachePolicyFallbackToSource,
			constants.LocalModelCachePolicyCacheRequired,
			"  cacheRequired ",
			"fallbacktosource",
		} {
			assert.Empty(t, ValidateLocalModelCachePolicyAnnotation(map[string]string{
				constants.LocalModelCachePolicyAnnotationKey: value,
			}, path), "value %q", value)
		}
	})

	t.Run("empty value is valid (treated as unset)", func(t *testing.T) {
		assert.Empty(t, ValidateLocalModelCachePolicyAnnotation(map[string]string{
			constants.LocalModelCachePolicyAnnotationKey: "",
		}, path))
		assert.Empty(t, ValidateLocalModelCachePolicyAnnotation(map[string]string{
			constants.LocalModelCachePolicyAnnotationKey: "  ",
		}, path))
	})

	t.Run("malformed value is rejected at admission", func(t *testing.T) {
		errs := ValidateLocalModelCachePolicyAnnotation(map[string]string{
			constants.LocalModelCachePolicyAnnotationKey: "CacheRequire",
		}, path)
		assert.Len(t, errs, 1)
		assert.Equal(t, "metadata.annotations[serving.kserve.io/local-model-cache-policy]", errs[0].Field)
		assert.Equal(t, field.ErrorTypeNotSupported, errs[0].Type)
	})
}
