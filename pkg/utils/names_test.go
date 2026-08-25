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

package utils

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeObjectName(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	tests := []struct {
		name string
		in   string
	}{
		{name: "plain name", in: "lora-pvc-billing-summarize-en-v1"},
		{name: "hf org prefix", in: "lora-pvc-acme/billing-summarize-en-v1.r16"},
		{name: "dots", in: "lora-pvc-adapter.v1.2"},
		{name: "uppercase", in: "lora-pvc-Qwen/Qwen2.5-7B-Instruct"},
		{name: "trailing invalid", in: "lora-pvc-adapter."},
		{name: "only invalid chars", in: "///"},
		{name: "longer than a label", in: strings.Repeat("x", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeObjectName(tt.in)
			assert.True(t, valid.MatchString(got), "name %q must be a DNS-1123 label", got)
			assert.LessOrEqual(t, len(got), 63)
		})
	}

	// Valid candidates pass through unchanged; adopting the helper must not
	// rename existing resources.
	assert.Equal(t, "lora-pvc-my-adapter", SafeObjectName("lora-pvc-my-adapter"))

	// "p-a/b" and "p-a.b" both sanitize to "p-a-b"; the hash of the original
	// keeps the two names distinct instead of silently sharing one volume.
	assert.Equal(t, "p-a-b-734c5699", SafeObjectName("p-a/b"))
	assert.Equal(t, "p-a-b-87e580e9", SafeObjectName("p-a.b"))

	// Results name live volumes, so the mapping must not change between
	// releases - a new result would rename volumes and restart pods on upgrade.
	assert.Equal(t, "p-acme-x-y-7c26bdb9", SafeObjectName("p-acme/x.y"))
}

func TestSanitizeDNS1123Label(t *testing.T) {
	assert.Equal(t, "qwen-qwen2-5-7b-instruct", sanitizeDNS1123Label("Qwen/Qwen2.5-7B-Instruct"))
	assert.Equal(t, "my-adapter", sanitizeDNS1123Label("my-adapter"))
	assert.Equal(t, "", sanitizeDNS1123Label("///"))
}
