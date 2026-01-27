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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDeploymentMode(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected DeploymentModeType
	}{
		{
			name:     "Empty string returns default",
			input:    "",
			expected: DefaultDeployment,
		},
		{
			name:     "Standard mode",
			input:    string(Standard),
			expected: Standard,
		},
		{
			name:     "Knative mode",
			input:    string(Knative),
			expected: Knative,
		},
		{
			name:     "ModelMesh mode",
			input:    string(ModelMeshDeployment),
			expected: ModelMeshDeployment,
		},
		{
			name:     "Legacy RawDeployment normalizes to Standard",
			input:    string(LegacyRawDeployment),
			expected: Standard,
		},
		{
			name:     "Legacy Serverless normalizes to Knative",
			input:    string(LegacyServerless),
			expected: Knative,
		},
		{
			name:     "Unknown mode returns as-is",
			input:    "UnknownMode",
			expected: DeploymentModeType("UnknownMode"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseDeploymentMode(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
