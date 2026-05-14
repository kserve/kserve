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

package llmisvc

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestIsDefaultBackendRef(t *testing.T) {
	shortName := "my-llm-service"
	longName := "my-very-long-llm-inference-service-name-that-exceeds"

	tests := []struct {
		name     string
		llmName  string
		ref      gwapiv1.BackendRef
		expected bool
	}{
		{
			name:    "InferencePool with ChildName (short name)",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					Name: gwapiv1.ObjectName(kmeta.ChildName(shortName, "-inference-pool")),
				},
			},
			expected: true,
		},
		{
			name:    "InferencePool with ChildName (long name, hashed)",
			llmName: longName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					Name: gwapiv1.ObjectName(kmeta.ChildName(longName, "-inference-pool")),
				},
			},
			expected: true,
		},
		{
			name:    "InferencePool with simple concatenation (long name, NOT hashed)",
			llmName: longName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					Name: gwapiv1.ObjectName(longName + "-inference-pool"),
				},
			},
			expected: true,
		},
		{
			name:    "Service kind should not match",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("Service")),
					Name: gwapiv1.ObjectName(kmeta.ChildName(shortName, "-inference-pool")),
				},
			},
			expected: false,
		},
		{
			name:    "InferencePool with unrelated name",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					Name: gwapiv1.ObjectName("unrelated-pool"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: tt.llmName},
			}
			got := isDefaultBackendRef(llmSvc, tt.ref)
			if got != tt.expected {
				t.Errorf("isDefaultBackendRef() = %v, want %v (name=%q, refName=%q)",
					got, tt.expected, tt.llmName, tt.ref.Name)
			}
		})
	}
}

func TestIsDefaultWorkloadServiceBackendRef(t *testing.T) {
	shortName := "my-llm-service"
	longName := "my-very-long-llm-inference-service-name-that-exceeds"

	tests := []struct {
		name     string
		llmName  string
		ref      gwapiv1.BackendRef
		expected bool
	}{
		{
			name:    "Service with ChildName (short name)",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group("")),
					Kind:  ptr.To(gwapiv1.Kind("Service")),
					Name:  gwapiv1.ObjectName(kmeta.ChildName(shortName, "-kserve-workload-svc")),
				},
			},
			expected: true,
		},
		{
			name:    "Service with simple concatenation (short name, same result)",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group("")),
					Kind:  ptr.To(gwapiv1.Kind("Service")),
					Name:  gwapiv1.ObjectName(shortName + "-kserve-workload-svc"),
				},
			},
			expected: true,
		},
		{
			name:    "Service with ChildName (long name, hashed)",
			llmName: longName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group("")),
					Kind:  ptr.To(gwapiv1.Kind("Service")),
					Name:  gwapiv1.ObjectName(kmeta.ChildName(longName, "-kserve-workload-svc")),
				},
			},
			expected: true,
		},
		{
			name:    "Service with simple concatenation (long name, NOT hashed) - the bug case",
			llmName: longName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group("")),
					Kind:  ptr.To(gwapiv1.Kind("Service")),
					Name:  gwapiv1.ObjectName(longName + "-kserve-workload-svc"),
				},
			},
			expected: true,
		},
		{
			name:    "Service with default Kind (nil kind defaults to Service)",
			llmName: longName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Name: gwapiv1.ObjectName(longName + "-kserve-workload-svc"),
				},
			},
			expected: true,
		},
		{
			name:    "InferencePool kind should not match",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					Name: gwapiv1.ObjectName(kmeta.ChildName(shortName, "-kserve-workload-svc")),
				},
			},
			expected: false,
		},
		{
			name:    "Service with non-core group should not match",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group("inference.networking.k8s.io")),
					Kind:  ptr.To(gwapiv1.Kind("Service")),
					Name:  gwapiv1.ObjectName(kmeta.ChildName(shortName, "-kserve-workload-svc")),
				},
			},
			expected: false,
		},
		{
			name:    "Service with unrelated name should not match",
			llmName: shortName,
			ref: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("Service")),
					Name: gwapiv1.ObjectName("unrelated-service"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: tt.llmName},
			}
			got := isDefaultWorkloadServiceBackendRef(llmSvc, tt.ref)
			if got != tt.expected {
				t.Errorf("isDefaultWorkloadServiceBackendRef() = %v, want %v (name=%q, refName=%q)",
					got, tt.expected, tt.llmName, tt.ref.Name)
			}
		})
	}
}

// TestLongNameChildNameDifference verifies that ChildName produces different output
// than simple concatenation when the combined name exceeds 63 characters.
func TestLongNameChildNameDifference(t *testing.T) {
	longName := "my-very-long-llm-inference-service-name-that-exceeds"
	suffix := "-kserve-workload-svc"

	simpleName := longName + suffix
	childName := kmeta.ChildName(longName, suffix)

	if len(simpleName) <= 63 {
		t.Fatalf("Test setup error: simpleName should exceed 63 chars, got %d", len(simpleName))
	}
	if len(childName) > 63 {
		t.Fatalf("ChildName result should be <= 63 chars, got %d", len(childName))
	}
	if simpleName == childName {
		t.Fatal("Expected simpleName and childName to differ for long names")
	}

	t.Logf("simpleName (%d chars): %s", len(simpleName), simpleName)
	t.Logf("childName  (%d chars): %s", len(childName), childName)
}
