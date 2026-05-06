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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// loraTemplateCmd is a minimal vllm command embedding the LoRA template snippet
// (same logic as config/llmisvcconfig/ templates, collapsed to one line for test clarity).
const loraTemplateCmd = "{{ if .Spec.Model.LoRA }}" +
	"{{ if gt (len .Spec.Model.LoRA.Adapters) 0 }}" +
	"--enable-lora " +
	"--max-loras={{ if .Spec.Model.LoRA.MaxAdapters }}{{ .Spec.Model.LoRA.MaxAdapters }}{{ else }}{{ len .Spec.Model.LoRA.Adapters }}{{ end }} " +
	"--max-lora-rank={{ if .Spec.Model.LoRA.MaxRank }}{{ .Spec.Model.LoRA.MaxRank }}{{ else }}16{{ end }} " +
	"{{ range .Spec.Model.LoRA.Adapters }}{{ if not .Disabled }}--lora-modules={{ .Name }}=/mnt/loras/{{ .Name }} {{ end }}{{ end }}" +
	"{{ end }}{{ end }}"

func loraArgsConfig() *v1alpha2.LLMInferenceServiceConfig {
	return &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Command: []string{"/bin/bash", "-c", loraTemplateCmd},
					}},
				},
			},
		},
	}
}

func renderedLoRACmd(t *testing.T, llmSvc *v1alpha2.LLMInferenceService) string {
	t.Helper()
	out, err := llmisvc.ReplaceVariables(llmSvc, loraArgsConfig(), nil)
	if err != nil {
		t.Fatalf("ReplaceVariables: %v", err)
	}
	return out.Spec.WorkloadSpec.Template.Containers[0].Command[2]
}

func TestLoRATemplateRendering(t *testing.T) {
	tests := []struct {
		name        string
		lora        *v1alpha2.LoRASpec
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:       "no adapters",
			lora:       nil,
			wantAbsent: []string{"--enable-lora", "--lora-modules="},
		},
		{
			name: "one Preload",
			lora: &v1alpha2.LoRASpec{
				Adapters: []v1alpha2.LoRAAdapterSpec{
					{Name: "foo"},
				},
			},
			wantPresent: []string{
				"--enable-lora",
				"--max-loras=1",
				"--max-lora-rank=16",
				"--lora-modules=foo=/mnt/loras/foo",
			},
		},
		{
			name: "two Preload",
			lora: &v1alpha2.LoRASpec{
				Adapters: []v1alpha2.LoRAAdapterSpec{
					{Name: "foo"},
					{Name: "bar"},
				},
			},
			wantPresent: []string{
				"--enable-lora",
				"--max-loras=2",
				"--lora-modules=foo=/mnt/loras/foo",
				"--lora-modules=bar=/mnt/loras/bar",
			},
		},
		{
			name: "one Preload one Disabled",
			lora: &v1alpha2.LoRASpec{
				Adapters: []v1alpha2.LoRAAdapterSpec{
					{Name: "foo"},
					{Name: "bar", Disabled: true},
				},
			},
			wantPresent: []string{
				"--enable-lora",
				"--max-loras=2",
				"--lora-modules=foo=/mnt/loras/foo",
			},
			wantAbsent: []string{"--lora-modules=bar=/mnt/loras/bar"},
		},
		{
			name: "custom MaxRank",
			lora: &v1alpha2.LoRASpec{
				MaxRank: ptr.To[int32](64),
				Adapters: []v1alpha2.LoRAAdapterSpec{
					{Name: "foo"},
				},
			},
			wantPresent: []string{"--max-lora-rank=64"},
			wantAbsent:  []string{"--max-lora-rank=16"},
		},
		{
			name: "custom MaxAdapters",
			lora: &v1alpha2.LoRASpec{
				MaxAdapters: ptr.To[int32](5),
				Adapters: []v1alpha2.LoRAAdapterSpec{
					{Name: "foo"},
				},
			},
			wantPresent: []string{"--max-loras=5"},
			wantAbsent:  []string{"--max-loras=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						LoRA: tt.lora,
					},
				},
			}
			got := renderedLoRACmd(t, llmSvc)

			for _, want := range tt.wantPresent {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in rendered command, got:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("unexpected %q in rendered command, got:\n%s", absent, got)
				}
			}
		})
	}
}
