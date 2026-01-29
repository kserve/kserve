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
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		expected bool
	}{
		{
			name:     "nil LLMInferenceService returns false",
			llmSvc:   nil,
			expected: false,
		},
		{
			name: "no annotations returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
				},
			},
			expected: false,
		},
		{
			name: "migration annotation not set returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "migration annotation set to v1 returns true",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1",
					},
				},
			},
			expected: true,
		},
		{
			name: "migration annotation set to different value returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v2",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := isMigratedToV1(tt.llmSvc)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestSetMigratedToV1(t *testing.T) {
	t.Run("sets annotation on service with no annotations", func(t *testing.T) {
		g := NewGomegaWithT(t)
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-svc",
			},
		}

		setMigratedToV1(llmSvc)

		g.Expect(llmSvc.Annotations).ToNot(BeNil())
		g.Expect(llmSvc.Annotations[constants.InferencePoolMigratedAnnotationKey]).To(Equal("v1"))
	})

	t.Run("sets annotation on service with existing annotations", func(t *testing.T) {
		g := NewGomegaWithT(t)
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-svc",
				Annotations: map[string]string{
					"existing-key": "existing-value",
				},
			},
		}

		setMigratedToV1(llmSvc)

		g.Expect(llmSvc.Annotations[constants.InferencePoolMigratedAnnotationKey]).To(Equal("v1"))
		g.Expect(llmSvc.Annotations["existing-key"]).To(Equal("existing-value"))
	})
}

func TestIsV1Alpha2Unsupported(t *testing.T) {
	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		expected bool
	}{
		{
			name:     "nil LLMInferenceService returns false",
			llmSvc:   nil,
			expected: false,
		},
		{
			name: "no annotations returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
				},
			},
			expected: false,
		},
		{
			name: "unsupported annotation not set returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "unsupported annotation set to true returns true",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						InferencePoolV1Alpha2UnsupportedAnnotationKey: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "unsupported annotation set to false returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						InferencePoolV1Alpha2UnsupportedAnnotationKey: "false",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := isV1Alpha2Unsupported(tt.llmSvc)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestSetV1Alpha2Unsupported(t *testing.T) {
	t.Run("sets annotation on service with no annotations", func(t *testing.T) {
		g := NewGomegaWithT(t)
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-svc",
			},
		}

		setV1Alpha2Unsupported(llmSvc)

		g.Expect(llmSvc.Annotations).ToNot(BeNil())
		g.Expect(llmSvc.Annotations[InferencePoolV1Alpha2UnsupportedAnnotationKey]).To(Equal("true"))
	})

	t.Run("sets annotation on service with existing annotations", func(t *testing.T) {
		g := NewGomegaWithT(t)
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-svc",
				Annotations: map[string]string{
					"existing-key": "existing-value",
				},
			},
		}

		setV1Alpha2Unsupported(llmSvc)

		g.Expect(llmSvc.Annotations[InferencePoolV1Alpha2UnsupportedAnnotationKey]).To(Equal("true"))
		g.Expect(llmSvc.Annotations["existing-key"]).To(Equal("existing-value"))
	})
}

func TestGetActivePoolAPIGroup(t *testing.T) {
	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		expected string
	}{
		{
			name: "not migrated returns v1alpha2 API group",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
				},
			},
			expected: constants.InferencePoolV1Alpha2APIGroupName,
		},
		{
			name: "migrated to v1 returns v1 API group",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
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
