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

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// otherMode stands in for a real (fallible) verification mode. ShouldBlock only
// special-cases ModeDisabled, so any non-disabled mode exercises the same path.
const otherMode Mode = "othermode"

func TestVerifyResultShouldBlock(t *testing.T) {
	tests := []struct {
		name   string
		result VerifyResult
		policy FailurePolicy
		want   bool
	}{
		{"disabled never blocks even under reject", VerifyResult{Mode: ModeDisabled, Verified: false}, FailurePolicyReject, false},
		{"verified never blocks", VerifyResult{Mode: otherMode, Verified: true}, FailurePolicyReject, false},
		{"unverified blocks under reject", VerifyResult{Mode: otherMode, Verified: false}, FailurePolicyReject, true},
		{"unverified warns (no block) under warn", VerifyResult{Mode: otherMode, Verified: false}, FailurePolicyWarn, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.result.ShouldBlock(tt.policy))
		})
	}
}
