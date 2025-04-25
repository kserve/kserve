/*
Copyright 2021 The KServe Authors.
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

package ingress

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
)

func TestFilterByPrefix(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		name     string
		input    map[string]string
		prefix   string
		expected map[string]string
	}{
		{
			name: "filter nginx annotations",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "*",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "true",
			},
			prefix: "nginx.ingress.kubernetes.io",
			expected: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "*",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "true",
			},
		},
		{
			name: "filter serving annotations",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "*",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "true",
			},
			prefix:   "serving.kserve.io",
			expected: map[string]string{},
		},
		{
			name: "no matching annotations",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "*",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "true",
			},
			prefix:   "unknown.prefix",
			expected: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := filterByPrefix(tc.input, tc.prefix)
			g.Expect(result).To(gomega.Equal(tc.expected))
		})
	}
}

func TestProcessCorsAnnotations(t *testing.T) {
	testCases := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name: "CORS enabled with custom origin",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":       "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin": "https://example.com",
			},
			expected: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "https://example.com",
				"nginx.ingress.kubernetes.io/cors-allow-headers":     "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "true",
			},
		},
		{
			name: "CORS enabled with all custom values",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "https://example.com",
				"nginx.ingress.kubernetes.io/cors-allow-headers":     "Content-Type,Authorization",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "false",
			},
			expected: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "true",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "https://example.com",
				"nginx.ingress.kubernetes.io/cors-allow-headers":     "Content-Type,Authorization",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "false",
			},
		},
		{
			name: "CORS disabled",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/enable-cors":            "false",
				"nginx.ingress.kubernetes.io/cors-allow-origin":      "https://example.com",
				"nginx.ingress.kubernetes.io/cors-allow-headers":     "Content-Type,Authorization",
				"nginx.ingress.kubernetes.io/cors-allow-credentials": "false",
				"nginx.ingress.kubernetes.io/other-annotation":       "value",
			},
			expected: map[string]string{
				"nginx.ingress.kubernetes.io/other-annotation": "value",
			},
		},
		{
			name: "CORS not specified",
			input: map[string]string{
				"nginx.ingress.kubernetes.io/other-annotation": "value",
			},
			expected: map[string]string{
				"nginx.ingress.kubernetes.io/other-annotation": "value",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := processCorsAnnotations(tc.input)
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("processCorsAnnotations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeAnnotations(t *testing.T) {
	// This test simulates the annotation merging process in createRawIngress
	g := gomega.NewGomegaWithT(t)

	// Test case: Original annotations with CORS enabled
	isvcAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/enable-cors": "true",
		"other-annotation":                        "value",
		"kubernetes.io/ingress.class":             "nginx",
	}

	// Step 1: Copy all original annotations
	ingressAnnotations := make(map[string]string)
	for k, v := range isvcAnnotations {
		ingressAnnotations[k] = v
	}

	// Step 2: Extract and process CORS related annotations
	corsAnnotations := filterByPrefix(isvcAnnotations, NginxIngressAnnotationPrefix)
	corsAnnotations = processCorsAnnotations(corsAnnotations)

	// Step 3: Merge processed CORS annotations back into all annotations
	for k, v := range corsAnnotations {
		ingressAnnotations[k] = v
	}

	// Verify that all original annotations are preserved
	g.Expect(ingressAnnotations).To(gomega.HaveKey("other-annotation"))
	g.Expect(ingressAnnotations).To(gomega.HaveKey("kubernetes.io/ingress.class"))

	// Verify that CORS annotations are properly processed
	g.Expect(ingressAnnotations).To(gomega.HaveKey("nginx.ingress.kubernetes.io/enable-cors"))
	g.Expect(ingressAnnotations).To(gomega.HaveKey("nginx.ingress.kubernetes.io/cors-allow-origin"))
	g.Expect(ingressAnnotations).To(gomega.HaveKey("nginx.ingress.kubernetes.io/cors-allow-headers"))
	g.Expect(ingressAnnotations).To(gomega.HaveKey("nginx.ingress.kubernetes.io/cors-allow-credentials"))

	// Verify specific values
	g.Expect(ingressAnnotations["nginx.ingress.kubernetes.io/enable-cors"]).To(gomega.Equal("true"))
	g.Expect(ingressAnnotations["nginx.ingress.kubernetes.io/cors-allow-origin"]).To(gomega.Equal("*"))
	g.Expect(ingressAnnotations["other-annotation"]).To(gomega.Equal("value"))
}
