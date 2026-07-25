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

package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
)

func TestInferenceGraphDefaulter_Default(t *testing.T) {
	defaulter := &InferenceGraphDefaulter{}

	tests := []struct {
		name    string
		ig      *InferenceGraph
		want    InferenceGraphSpec
		wantErr bool
	}{
		{
			name: "defaults minReplicas when unset",
			ig:   &InferenceGraph{Spec: InferenceGraphSpec{}},
			want: InferenceGraphSpec{MinReplicas: ptr.To(int32(1))},
		},
		{
			name: "preserves explicit zero for scale-to-zero",
			ig:   &InferenceGraph{Spec: InferenceGraphSpec{MinReplicas: ptr.To(int32(0))}},
			want: InferenceGraphSpec{MinReplicas: ptr.To(int32(0))},
		},
		{
			name: "preserves explicit minReplicas",
			ig:   &InferenceGraph{Spec: InferenceGraphSpec{MinReplicas: ptr.To(int32(5))}},
			want: InferenceGraphSpec{MinReplicas: ptr.To(int32(5))},
		},
		{
			name: "does not default mode-dependent fields",
			ig:   &InferenceGraph{Spec: InferenceGraphSpec{}},
			want: InferenceGraphSpec{
				MinReplicas:    ptr.To(int32(1)),
				MaxReplicas:    0,
				ScaleMetric:    nil,
				TimeoutSeconds: nil,
				ScaleTarget:    nil,
			},
		},
		{
			name: "is idempotent",
			ig:   &InferenceGraph{Spec: InferenceGraphSpec{}},
			want: InferenceGraphSpec{MinReplicas: ptr.To(int32(1))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := defaulter.Default(t.Context(), tt.ig); err != nil {
				t.Fatalf("Default() unexpected error: %v", err)
			}
			if tt.name == "is idempotent" {
				if err := defaulter.Default(t.Context(), tt.ig); err != nil {
					t.Fatalf("second Default() unexpected error: %v", err)
				}
			}
			if diff := cmp.Diff(tt.want, tt.ig.Spec); diff != "" {
				t.Errorf("Spec mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("wrong type returns error", func(t *testing.T) {
		err := defaulter.Default(t.Context(), &InferenceGraphList{})
		if err == nil {
			t.Fatal("Default() expected error for wrong type, got nil")
		}
	})
}
