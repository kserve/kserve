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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestIsMigratedToV1(t *testing.T) {
	tests := []struct {
		name        string
		llmSvc      *v1alpha2.LLMInferenceService
		expected    bool
		description string
	}{
		{
			name:        "nil LLMInferenceService",
			llmSvc:      nil,
			expected:    false,
			description: "should return false for nil input",
		},
		{
			name: "nil annotations",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-llmisvc",
					Annotations: nil,
				},
			},
			expected:    false,
			description: "should return false when annotations are nil",
		},
		{
			name: "empty annotations",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-llmisvc",
					Annotations: map[string]string{},
				},
			},
			expected:    false,
			description: "should return false when migration annotation is missing",
		},
		{
			name: "migration annotation set to v1",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1",
					},
				},
			},
			expected:    true,
			description: "should return true when migration annotation is 'v1'",
		},
		{
			name: "migration annotation set to different value",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1alpha2",
					},
				},
			},
			expected:    false,
			description: "should return false when migration annotation is not 'v1'",
		},
		{
			name: "migration annotation set to empty string",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "",
					},
				},
			},
			expected:    false,
			description: "should return false when migration annotation is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := isMigratedToV1(tt.llmSvc)
			g.Expect(result).To(Equal(tt.expected), tt.description)
		})
	}
}

func TestSetMigratedToV1(t *testing.T) {
	tests := []struct {
		name        string
		llmSvc      *v1alpha2.LLMInferenceService
		description string
	}{
		{
			name: "nil annotations - should initialize",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-llmisvc",
					Annotations: nil,
				},
			},
			description: "should initialize annotations map and set migration annotation",
		},
		{
			name: "empty annotations",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-llmisvc",
					Annotations: map[string]string{},
				},
			},
			description: "should set migration annotation on empty map",
		},
		{
			name: "existing annotations - should preserve",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						"existing-key": "existing-value",
					},
				},
			},
			description: "should preserve existing annotations and add migration annotation",
		},
		{
			name: "overwrite existing migration annotation",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1alpha2",
					},
				},
			},
			description: "should overwrite existing migration annotation with 'v1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			// Store existing annotations count for preservation check
			existingAnnotations := make(map[string]string)
			if tt.llmSvc.Annotations != nil {
				for k, v := range tt.llmSvc.Annotations {
					if k != constants.InferencePoolMigratedAnnotationKey {
						existingAnnotations[k] = v
					}
				}
			}

			setMigratedToV1(tt.llmSvc)

			// Verify migration annotation is set correctly
			g.Expect(tt.llmSvc.Annotations).ToNot(BeNil(), "annotations should not be nil after setMigratedToV1")
			g.Expect(tt.llmSvc.Annotations[constants.InferencePoolMigratedAnnotationKey]).To(Equal("v1"), tt.description)

			// Verify existing annotations are preserved
			for k, v := range existingAnnotations {
				g.Expect(tt.llmSvc.Annotations[k]).To(Equal(v), "existing annotation %s should be preserved", k)
			}

			// Verify isMigratedToV1 returns true after setting
			g.Expect(isMigratedToV1(tt.llmSvc)).To(BeTrue(), "isMigratedToV1 should return true after setMigratedToV1")
		})
	}
}

func TestGetActivePoolAPIGroup(t *testing.T) {
	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		expected string
	}{
		{
			name: "not migrated - should return v1alpha2 group",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
				},
			},
			expected: constants.InferencePoolV1Alpha2APIGroupName,
		},
		{
			name: "migrated to v1 - should return v1 group",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-llmisvc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1",
					},
				},
			},
			expected: constants.InferencePoolV1APIGroupName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := getActivePoolAPIGroup(tt.llmSvc)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestMigrationAnnotationKey(t *testing.T) {
	g := NewGomegaWithT(t)
	// Verify the annotation key follows the expected pattern
	g.Expect(constants.InferencePoolMigratedAnnotationKey).To(Equal("serving.kserve.io/inferencepool-migrated"))
}

func TestAPIGroupConstants(t *testing.T) {
	g := NewGomegaWithT(t)
	// Verify API group constants are set correctly
	g.Expect(constants.InferencePoolV1APIGroupName).To(Equal("inference.networking.k8s.io"))
	g.Expect(constants.InferencePoolV1Alpha2APIGroupName).To(Equal("inference.networking.x-k8s.io"))
}
