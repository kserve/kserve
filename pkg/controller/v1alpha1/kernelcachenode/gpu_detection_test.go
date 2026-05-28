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

package kernelcachenode

import (
	"testing"

	mcvDevices "github.com/redhat-et/GKM/mcv/pkg/accelerator/devices"
	"github.com/stretchr/testify/assert"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestConvertMCVToGPUTypeInfo(t *testing.T) {
	tests := []struct {
		name     string
		input    *mcvDevices.GPUFleetSummary
		expected []v1alpha1.GPUTypeInfo
	}{
		{
			name: "Single GPU type with multiple devices",
			input: &mcvDevices.GPUFleetSummary{
				GPUs: []mcvDevices.GPUGroup{
					{
						GPUType:       "nvidia-a100-80gb",
						IDs:           []int{0, 1, 2, 3, 4, 5, 6, 7},
						DriverVersion: "535.104.05",
					},
				},
			},
			expected: []v1alpha1.GPUTypeInfo{
				{
					GPUType:       "nvidia-a100-80gb",
					IDs:           []int{0, 1, 2, 3, 4, 5, 6, 7},
					DriverVersion: "535.104.05",
					CUDAVersion:   "",
					ROCmVersion:   "",
				},
			},
		},
		{
			name: "Multiple GPU types",
			input: &mcvDevices.GPUFleetSummary{
				GPUs: []mcvDevices.GPUGroup{
					{
						GPUType:       "nvidia-a100-80gb",
						IDs:           []int{0, 1, 2, 3},
						DriverVersion: "535.104.05",
					},
					{
						GPUType:       "nvidia-h100-80gb",
						IDs:           []int{4, 5, 6, 7},
						DriverVersion: "535.104.05",
					},
				},
			},
			expected: []v1alpha1.GPUTypeInfo{
				{
					GPUType:       "nvidia-a100-80gb",
					IDs:           []int{0, 1, 2, 3},
					DriverVersion: "535.104.05",
					CUDAVersion:   "",
					ROCmVersion:   "",
				},
				{
					GPUType:       "nvidia-h100-80gb",
					IDs:           []int{4, 5, 6, 7},
					DriverVersion: "535.104.05",
					CUDAVersion:   "",
					ROCmVersion:   "",
				},
			},
		},
		{
			name: "AMD GPU",
			input: &mcvDevices.GPUFleetSummary{
				GPUs: []mcvDevices.GPUGroup{
					{
						GPUType:       "Aldebaran/MI200",
						IDs:           []int{0, 1, 2, 3},
						DriverVersion: "6.1.5",
					},
				},
			},
			expected: []v1alpha1.GPUTypeInfo{
				{
					GPUType:       "Aldebaran/MI200",
					IDs:           []int{0, 1, 2, 3},
					DriverVersion: "6.1.5",
					CUDAVersion:   "",
					ROCmVersion:   "",
				},
			},
		},
		{
			name:     "Empty input",
			input:    &mcvDevices.GPUFleetSummary{GPUs: []mcvDevices.GPUGroup{}},
			expected: []v1alpha1.GPUTypeInfo{},
		},
		{
			name:     "Nil input",
			input:    nil,
			expected: []v1alpha1.GPUTypeInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMCVToGPUTypeInfo(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertMCVToGPUTypeInfo_FieldMapping(t *testing.T) {
	// Test that all fields are correctly mapped
	input := &mcvDevices.GPUFleetSummary{
		GPUs: []mcvDevices.GPUGroup{
			{
				GPUType:       "nvidia-test-gpu",
				IDs:           []int{0, 1},
				DriverVersion: "550.0",
			},
		},
	}

	result := convertMCVToGPUTypeInfo(input)

	assert.Len(t, result, 1)
	assert.Equal(t, "nvidia-test-gpu", result[0].GPUType)
	assert.Equal(t, []int{0, 1}, result[0].IDs)
	assert.Equal(t, "550.0", result[0].DriverVersion)
	assert.Equal(t, "", result[0].CUDAVersion)
	assert.Equal(t, "", result[0].ROCmVersion)
}
