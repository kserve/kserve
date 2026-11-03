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

package tracing

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestHasArg(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		flag   string
		expect bool
	}{
		{"exact match", []string{"--tracing=true"}, "--tracing", true},
		{"flag=value form", []string{"--otlp-traces-endpoint=http://x"}, "--otlp-traces-endpoint", true},
		{"flag alone", []string{"--tracing"}, "--tracing", true},
		{"no match", []string{"--other"}, "--tracing", false},
		{"empty args", []string{}, "--tracing", false},
		{"partial prefix doesn't match", []string{"--tracing-extra"}, "--tracing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(HasArg(tt.args, tt.flag)).To(Equal(tt.expect))
		})
	}
}
