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
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

func disaggAnnotations(value string) map[string]string {
	return map[string]string{constants.LLMDisaggregatedSetAnnotationKey: value}
}

func TestDisaggregatedSetEnabled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "nil annotations", annotations: nil, want: false},
		{name: "unrelated annotation", annotations: map[string]string{"foo": "bar"}, want: false},
		{name: "true", annotations: disaggAnnotations("true"), want: true},
		{name: "false", annotations: disaggAnnotations("false"), want: false},
		{name: "True is accepted case-insensitively", annotations: disaggAnnotations("True"), want: true},
		{name: "TRUE is accepted case-insensitively", annotations: disaggAnnotations("TRUE"), want: true},
		{name: "surrounding whitespace is trimmed", annotations: disaggAnnotations("  true  "), want: true},
		{name: "empty value is not opt-in", annotations: disaggAnnotations(""), want: false},
		// The contract is exactly "true"/"false", matching serving.kserve.io/stop and
		// narrower than strconv.ParseBool. These are rejected at admission, and a
		// controller must never read an unrecognised value as opt-in.
		{name: "1 is not opt-in", annotations: disaggAnnotations("1"), want: false},
		{name: "t is not opt-in", annotations: disaggAnnotations("t"), want: false},
		{name: "unrecognised value is not opt-in", annotations: disaggAnnotations("yes"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisaggregatedSetEnabled(tt.annotations))
		})
	}
}

func TestValidateDisaggregatedSetAnnotation(t *testing.T) {
	t.Run("annotation absent is valid", func(t *testing.T) {
		assert.Empty(t, ValidateDisaggregatedSetAnnotation(nil))
		assert.Empty(t, ValidateDisaggregatedSetAnnotation(map[string]string{"foo": "bar"}))
	})

	t.Run("accepted values", func(t *testing.T) {
		for _, v := range []string{"true", "false", "True", "FALSE", "  true  "} {
			assert.Empty(t, ValidateDisaggregatedSetAnnotation(disaggAnnotations(v)), "value %q", v)
		}
	})

	// The accepted set is exactly "true"/"false" case-insensitively, which is what the
	// error advertises. strconv.ParseBool's extra forms are deliberately excluded so the
	// contract matches the message and the sibling serving.kserve.io/stop annotation.
	t.Run("values outside the documented contract are rejected", func(t *testing.T) {
		for _, v := range []string{"yes", "", "1", "0", "t", "f", "TRUEISH"} {
			errs := ValidateDisaggregatedSetAnnotation(disaggAnnotations(v))
			require.Len(t, errs, 1, "value %q", v)
			assert.Contains(t, errs[0].Field, constants.LLMDisaggregatedSetAnnotationKey)
			assert.Equal(t, field.ErrorTypeNotSupported, errs[0].Type)
		}
	})

	// Admission validates only the shape of the annotation. It cannot see the feature
	// gate, and presets merged after admission can still introduce spec.scaling, so
	// feature-level constraints are the reconciler's job. Opting in alongside scaling
	// must therefore be admitted here.
	t.Run("opting in is admitted regardless of the rest of the spec", func(t *testing.T) {
		assert.Empty(t, ValidateDisaggregatedSetAnnotation(disaggAnnotations("true")))
	})
}
