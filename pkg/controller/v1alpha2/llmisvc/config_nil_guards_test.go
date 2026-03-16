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

package llmisvc_test

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// TestReplaceVariables_NilParallelism_WorkerDataParallel verifies that templates
// in config-llm-worker-data-parallel.yaml safely handle nil .Spec.Parallelism
func TestReplaceVariables_NilParallelism_WorkerDataParallel(t *testing.T) {
	g := NewGomegaWithT(t)

	// This LLMInferenceService has NO Parallelism field set (nil)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("test-model"),
			},
			// Parallelism is nil - this is the scenario we're testing
		},
	}

	// This config mimics the template expressions from config-llm-worker-data-parallel.yaml
	// with nil guards using {{with}} blocks
	cfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Command: []string{
								"/bin/bash",
								"-c",
								// These template expressions use {{with}} guards to safely handle nil .Spec.Parallelism
								`vllm serve /mnt/models ` +
									`{{- with .Spec.Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }} ` +
									`{{- with .Spec.Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }} ` +
									`--data-parallel-size {{ with .Spec.Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }} ` +
									`--data-parallel-size-local {{ with .Spec.Parallelism }}{{ or .DataLocal 1 }}{{ else }}1{{ end }} ` +
									`--data-parallel-rpc-port {{ with .Spec.Parallelism }}{{ if .DataRPCPort }}{{ .DataRPCPort }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}`,
							},
						},
					},
				},
			},
		},
	}

	// This should not panic - the test verifies nil safety
	result, err := llmisvc.ReplaceVariables(llmSvc, cfg, nil)

	// We expect this to succeed without panicking
	g.Expect(err).NotTo(HaveOccurred(), "ReplaceVariables should handle nil Parallelism without crashing")
	g.Expect(result).NotTo(BeNil())

	// Verify the command was rendered correctly with default values
	g.Expect(result.Spec.Template.Containers).To(HaveLen(1))
	command := result.Spec.Template.Containers[0].Command[2]
	g.Expect(command).To(ContainSubstring("--data-parallel-size 1"))
	g.Expect(command).To(ContainSubstring("--data-parallel-size-local 1"))
	g.Expect(command).To(ContainSubstring("--data-parallel-rpc-port 5555"))
	g.Expect(command).NotTo(ContainSubstring("--enable-expert-parallel"))
	g.Expect(command).NotTo(ContainSubstring("--tensor-parallel-size"))
}

// TestReplaceVariables_NilParallelism_DecodeWorkerDataParallel verifies that templates
// in config-llm-decode-worker-data-parallel.yaml safely handle nil .Spec.Parallelism
func TestReplaceVariables_NilParallelism_DecodeWorkerDataParallel(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("test-model"),
			},
			// Parallelism is nil
		},
	}

	// Mimics config-llm-decode-worker-data-parallel.yaml template section with nil guards
	cfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Command: []string{
								"/bin/bash",
								"-c",
								`vllm serve /mnt/models ` +
									`{{- with .Spec.Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }} ` +
									`{{- with .Spec.Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }} ` +
									`--data-parallel-size {{ with .Spec.Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := llmisvc.ReplaceVariables(llmSvc, cfg, nil)
	g.Expect(err).NotTo(HaveOccurred(), "ReplaceVariables should handle nil Parallelism in decode worker config")
	g.Expect(result).NotTo(BeNil())
}

// TestReplaceVariables_NilPrefillParallelism_PrefillWorkerDataParallel verifies that templates
// in config-llm-prefill-worker-data-parallel.yaml safely handle nil .Spec.Prefill.Parallelism
// This is a DOUBLE pointer chain: .Spec.Prefill.Parallelism.*
func TestReplaceVariables_NilPrefillParallelism_PrefillWorkerDataParallel(t *testing.T) {
	tests := []struct {
		name        string
		llmSvc      *v1alpha2.LLMInferenceService
		description string
	}{
		{
			name: "Prefill is nil",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm",
					Namespace: "test-ns",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("test-model"),
					},
					// Prefill is nil - first level of the chain
				},
			},
			description: "when .Spec.Prefill is nil",
		},
		{
			name: "Prefill exists but Parallelism is nil",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm",
					Namespace: "test-ns",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("test-model"),
					},
					Prefill: &v1alpha2.WorkloadSpec{
						// Parallelism is nil - second level of the chain
					},
				},
			},
			description: "when .Spec.Prefill.Parallelism is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			// Mimics config-llm-prefill-worker-data-parallel.yaml template expressions
			// These access .Spec.Prefill.Parallelism.* which is a double pointer chain
			// Using nested {{with}} blocks to safely handle both nil .Spec.Prefill and nil .Parallelism
			cfg := &v1alpha2.LLMInferenceServiceConfig{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Command: []string{
										"/bin/bash",
										"-c",
										// Double pointer chain with nested {{with}} guards
										`vllm serve /mnt/models ` +
											`{{- with .Spec.Prefill }}{{- with .Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }}{{- end }} ` +
											`{{- with .Spec.Prefill }}{{- with .Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }}{{- end }} ` +
											`--data-parallel-size {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }}{{ else }}1{{ end }} ` +
											`--data-parallel-size-local {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ or .DataLocal 1 }}{{ else }}1{{ end }}{{ else }}1{{ end }} ` +
											`--data-parallel-rpc-port {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ if .DataRPCPort }}{{ .DataRPCPort }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}`,
									},
								},
							},
						},
					},
				},
			}

			result, err := llmisvc.ReplaceVariables(tt.llmSvc, cfg, nil)
			g.Expect(err).NotTo(HaveOccurred(), "ReplaceVariables should handle nil in prefill double pointer chain: %s", tt.description)
			g.Expect(result).NotTo(BeNil())

			// Verify defaults were applied correctly
			g.Expect(result.Spec.Prefill.Template.Containers).To(HaveLen(1))
			command := result.Spec.Prefill.Template.Containers[0].Command[2]
			g.Expect(command).To(ContainSubstring("--data-parallel-size 1"))
			g.Expect(command).To(ContainSubstring("--data-parallel-size-local 1"))
			g.Expect(command).To(ContainSubstring("--data-parallel-rpc-port 5555"))
			g.Expect(command).NotTo(ContainSubstring("--enable-expert-parallel"))
			g.Expect(command).NotTo(ContainSubstring("--tensor-parallel-size"))
		})
	}
}

// TestReplaceVariables_WithParallelism_AllConfigs verifies that when Parallelism
// IS set, the templates correctly render the values
func TestReplaceVariables_WithParallelism_AllConfigs(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("test-model"),
			},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{
					Expert:      true,
					Tensor:      ptr.To[int32](4),
					Data:        ptr.To[int32](2),
					DataLocal:   ptr.To[int32](1),
					DataRPCPort: ptr.To[int32](6666),
				},
			},
		},
	}

	cfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Command: []string{
								"/bin/bash",
								"-c",
								`vllm serve /mnt/models ` +
									`{{- with .Spec.Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }} ` +
									`{{- with .Spec.Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }} ` +
									`--data-parallel-size {{ with .Spec.Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }} ` +
									`--data-parallel-size-local {{ with .Spec.Parallelism }}{{ or .DataLocal 1 }}{{ else }}1{{ end }} ` +
									`--data-parallel-rpc-port {{ with .Spec.Parallelism }}{{ if .DataRPCPort }}{{ .DataRPCPort }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}`,
							},
						},
					},
				},
			},
		},
	}

	result, err := llmisvc.ReplaceVariables(llmSvc, cfg, nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).NotTo(BeNil())

	// Verify all parallelism values were rendered
	command := result.Spec.Template.Containers[0].Command[2]
	g.Expect(command).To(ContainSubstring("--enable-expert-parallel"))
	g.Expect(command).To(ContainSubstring("--tensor-parallel-size 4"))
	g.Expect(command).To(ContainSubstring("--data-parallel-size 2"))
	g.Expect(command).To(ContainSubstring("--data-parallel-size-local 1"))
	g.Expect(command).To(ContainSubstring("--data-parallel-rpc-port 6666"))
}

// TestReplaceVariables_PartialParallelism verifies that when only some Parallelism
// fields are set, the templates correctly render the set values and use defaults for the rest
func TestReplaceVariables_PartialParallelism(t *testing.T) {
	tests := []struct {
		name              string
		parallelism       *v1alpha2.ParallelismSpec
		expectExpert      bool
		expectTensor      bool
		expectedData      string
		expectedDataLocal string
		expectedRPCPort   string
	}{
		{
			name:              "Only Tensor set",
			parallelism:       &v1alpha2.ParallelismSpec{Tensor: ptr.To[int32](8)},
			expectExpert:      false,
			expectTensor:      true,
			expectedData:      "--data-parallel-size 1",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 5555",
		},
		{
			name:              "Only Data set",
			parallelism:       &v1alpha2.ParallelismSpec{Data: ptr.To[int32](4)},
			expectExpert:      false,
			expectTensor:      false,
			expectedData:      "--data-parallel-size 4",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 5555",
		},
		{
			name:              "Expert and DataRPCPort set",
			parallelism:       &v1alpha2.ParallelismSpec{Expert: true, DataRPCPort: ptr.To[int32](7777)},
			expectExpert:      true,
			expectTensor:      false,
			expectedData:      "--data-parallel-size 1",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 7777",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model:        v1alpha2.LLMModelSpec{Name: ptr.To("model")},
					WorkloadSpec: v1alpha2.WorkloadSpec{Parallelism: tt.parallelism},
				},
			}

			cfg := &v1alpha2.LLMInferenceServiceConfig{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "main",
								Command: []string{"/bin/bash", "-c",
									`vllm serve ` +
										`{{- with .Spec.Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }} ` +
										`{{- with .Spec.Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }} ` +
										`--data-parallel-size {{ with .Spec.Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }} ` +
										`--data-parallel-size-local {{ with .Spec.Parallelism }}{{ or .DataLocal 1 }}{{ else }}1{{ end }} ` +
										`--data-parallel-rpc-port {{ with .Spec.Parallelism }}{{ if .DataRPCPort }}{{ .DataRPCPort }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}`,
								},
							}},
						},
					},
				},
			}

			result, err := llmisvc.ReplaceVariables(llmSvc, cfg, nil)
			g.Expect(err).NotTo(HaveOccurred())
			command := result.Spec.Template.Containers[0].Command[2]

			if tt.expectExpert {
				g.Expect(command).To(ContainSubstring("--enable-expert-parallel"))
			} else {
				g.Expect(command).NotTo(ContainSubstring("--enable-expert-parallel"))
			}

			if tt.expectTensor {
				g.Expect(command).To(ContainSubstring("--tensor-parallel-size"))
			} else {
				g.Expect(command).NotTo(ContainSubstring("--tensor-parallel-size"))
			}

			g.Expect(command).To(ContainSubstring(tt.expectedData))
			g.Expect(command).To(ContainSubstring(tt.expectedDataLocal))
			g.Expect(command).To(ContainSubstring(tt.expectedRPCPort))
		})
	}
}

// TestReplaceVariables_PrefillParallelism_ContentValidation validates that
// prefill templates with double pointer chains render correctly
func TestReplaceVariables_PrefillParallelism_ContentValidation(t *testing.T) {
	tests := []struct {
		name              string
		prefill           *v1alpha2.WorkloadSpec
		expectedData      string
		expectedDataLocal string
		expectedRPCPort   string
		expectExpert      bool
		expectTensor      bool
	}{
		{
			name: "Prefill with full Parallelism",
			prefill: &v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{
					Expert:      true,
					Tensor:      ptr.To[int32](2),
					Data:        ptr.To[int32](3),
					DataLocal:   ptr.To[int32](1),
					DataRPCPort: ptr.To[int32](8888),
				},
			},
			expectedData:      "--data-parallel-size 3",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 8888",
			expectExpert:      true,
			expectTensor:      true,
		},
		{
			name:              "Prefill nil - all defaults",
			prefill:           nil,
			expectedData:      "--data-parallel-size 1",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 5555",
			expectExpert:      false,
			expectTensor:      false,
		},
		{
			name:              "Prefill exists but Parallelism nil - all defaults",
			prefill:           &v1alpha2.WorkloadSpec{},
			expectedData:      "--data-parallel-size 1",
			expectedDataLocal: "--data-parallel-size-local 1",
			expectedRPCPort:   "--data-parallel-rpc-port 5555",
			expectExpert:      false,
			expectTensor:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model:   v1alpha2.LLMModelSpec{Name: ptr.To("model")},
					Prefill: tt.prefill,
				},
			}

			cfg := &v1alpha2.LLMInferenceServiceConfig{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "main",
								Command: []string{"/bin/bash", "-c",
									`vllm serve ` +
										`{{- with .Spec.Prefill }}{{- with .Parallelism }}{{- if .Expert -}}--enable-expert-parallel{{- end }}{{- end }}{{- end }} ` +
										`{{- with .Spec.Prefill }}{{- with .Parallelism }}{{- if .Tensor -}}--tensor-parallel-size {{ .Tensor }}{{- end }}{{- end }}{{- end }} ` +
										`--data-parallel-size {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ or .Data 1 }}{{ else }}1{{ end }}{{ else }}1{{ end }} ` +
										`--data-parallel-size-local {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ or .DataLocal 1 }}{{ else }}1{{ end }}{{ else }}1{{ end }} ` +
										`--data-parallel-rpc-port {{ with .Spec.Prefill }}{{ with .Parallelism }}{{ if .DataRPCPort }}{{ .DataRPCPort }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}{{ else }}5555{{ end }}`,
								},
							}},
						},
					},
				},
			}

			result, err := llmisvc.ReplaceVariables(llmSvc, cfg, nil)
			g.Expect(err).NotTo(HaveOccurred())
			command := result.Spec.Prefill.Template.Containers[0].Command[2]

			if tt.expectExpert {
				g.Expect(command).To(ContainSubstring("--enable-expert-parallel"))
			} else {
				g.Expect(command).NotTo(ContainSubstring("--enable-expert-parallel"))
			}

			if tt.expectTensor {
				g.Expect(command).To(ContainSubstring("--tensor-parallel-size"))
			} else {
				g.Expect(command).NotTo(ContainSubstring("--tensor-parallel-size"))
			}

			g.Expect(command).To(ContainSubstring(tt.expectedData))
			g.Expect(command).To(ContainSubstring(tt.expectedDataLocal))
			g.Expect(command).To(ContainSubstring(tt.expectedRPCPort))
		})
	}
}
