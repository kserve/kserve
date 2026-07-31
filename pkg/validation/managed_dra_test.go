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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/constants"
)

func TestHasManagedDRA(t *testing.T) {
	assert.False(t, HasManagedDRA(nil))
	assert.False(t, HasManagedDRA(map[string]string{"foo": "bar"}))
	assert.True(t, HasManagedDRA(map[string]string{constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com"}))
}

func TestManagedDRADeviceCount(t *testing.T) {
	t.Run("missing defaults to 1", func(t *testing.T) {
		count, err := ManagedDRADeviceCount(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
	t.Run("valid", func(t *testing.T) {
		count, err := ManagedDRADeviceCount(map[string]string{constants.ManagedDRADeviceCountAnnotationKey: "4"})
		require.NoError(t, err)
		assert.Equal(t, 4, count)
	})
	t.Run("zero rejected", func(t *testing.T) {
		_, err := ManagedDRADeviceCount(map[string]string{constants.ManagedDRADeviceCountAnnotationKey: "0"})
		assert.Error(t, err)
	})
	t.Run("non-numeric rejected", func(t *testing.T) {
		_, err := ManagedDRADeviceCount(map[string]string{constants.ManagedDRADeviceCountAnnotationKey: "abc"})
		assert.Error(t, err)
	})
}

func TestValidateManagedDRAAnnotations(t *testing.T) {
	const (
		deviceClassKey   = "serving.kserve.io/exp-dra-device-class"
		deviceCountKey   = "serving.kserve.io/exp-dra-device-count"
		celSelectorKey   = "serving.kserve.io/exp-dra-cel-selector"
		containerNameKey = "serving.kserve.io/exp-dra-container-name"
	)

	tests := []struct {
		name         string
		annotations  map[string]string
		wantErrCount int
		wantErrField string
	}{
		{
			name:         "no annotations",
			annotations:  nil,
			wantErrCount: 0,
		},
		{
			name:         "valid: device class only",
			annotations:  map[string]string{deviceClassKey: "gpu.nvidia.com"},
			wantErrCount: 0,
		},
		{
			name: "valid: full valid set",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				deviceCountKey:   "4",
				celSelectorKey:   "device.attributes['gpu.nvidia.com']['type'] == 'A100'",
				containerNameKey: "vllm",
			},
			wantErrCount: 0,
		},
		{
			name:         "invalid: empty device class",
			annotations:  map[string]string{deviceClassKey: "   "},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name:         "invalid: device count without device class",
			annotations:  map[string]string{deviceCountKey: "2"},
			wantErrCount: 1,
			wantErrField: deviceClassKey,
		},
		{
			name: "invalid: device count non-numeric",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				deviceCountKey: "abc",
			},
			wantErrCount: 1,
			wantErrField: deviceCountKey,
		},
		{
			name: "invalid: empty cel selector",
			annotations: map[string]string{
				deviceClassKey: "gpu.nvidia.com",
				celSelectorKey: "\n  \n",
			},
			wantErrCount: 1,
			wantErrField: celSelectorKey,
		},
		{
			name: "invalid: empty container name",
			annotations: map[string]string{
				deviceClassKey:   "gpu.nvidia.com",
				containerNameKey: "   ",
			},
			wantErrCount: 1,
			wantErrField: containerNameKey,
		},
		{
			name: "invalid: multiple errors surfaced",
			annotations: map[string]string{
				deviceClassKey: "BAD CLASS",
				deviceCountKey: "abc",
			},
			wantErrCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateManagedDRAAnnotations(tt.annotations)
			require.Len(t, errs, tt.wantErrCount, "errors: %v", errs)
			if tt.wantErrField != "" {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Field, tt.wantErrField) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error on field %q, got: %v", tt.wantErrField, errs)
			}
		})
	}
}
