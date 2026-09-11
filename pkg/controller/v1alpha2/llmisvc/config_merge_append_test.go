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
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

// specWithArgs builds a minimal LLMInferenceServiceSpec with a single
// container in the main template carrying the given args.
func specWithArgs(args []string) v1alpha2.LLMInferenceServiceSpec {
	return v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Args: args},
				},
			},
		},
	}
}

// mustParseFieldPaths parses multiple Kustomize-style paths, panicking on error.
func mustParseFieldPaths(paths ...string) []fieldPath {
	result := make([]fieldPath, 0, len(paths))
	for _, p := range paths {
		segments, err := v1alpha2.ParseFieldPath(p)
		if err != nil {
			panic(fmt.Sprintf("mustParseFieldPaths(%q): %v", p, err))
		}
		result = append(result, segments)
	}
	return result
}

// ---------------------------------------------------------------------------
// End-to-end merge chain tests (most important, exercise the full pipeline)
// ---------------------------------------------------------------------------

func TestMergeAnnotatedSpecs(t *testing.T) {
	const argsPath = "template.containers.[name=main].args"

	tests := []struct {
		name     string
		specs    []annotatedSpec
		wantArgs []string
	}{
		{
			name: "3 configs, all append: cumulative concatenation",
			specs: []annotatedSpec{
				{spec: specWithArgs([]string{"--a"})},
				{
					spec:        specWithArgs([]string{"--b"}),
					annotations: map[string]string{v1alpha2.MergeAppendFieldsAnnotation: argsPath},
				},
				{
					spec:        specWithArgs([]string{"--c"}),
					annotations: map[string]string{v1alpha2.MergeAppendFieldsAnnotation: argsPath},
				},
			},
			wantArgs: []string{"--a", "--b", "--c"},
		},
		{
			name: "append then replace: final config without annotation replaces",
			specs: []annotatedSpec{
				{spec: specWithArgs([]string{"--a"})},
				{
					spec:        specWithArgs([]string{"--b"}),
					annotations: map[string]string{v1alpha2.MergeAppendFieldsAnnotation: argsPath},
				},
				{spec: specWithArgs([]string{"--c"})},
			},
			wantArgs: []string{"--c"},
		},
		{
			name: "replace then append: appends only to the latest base",
			specs: []annotatedSpec{
				{spec: specWithArgs([]string{"--a"})},
				{spec: specWithArgs([]string{"--b"})},
				{
					spec:        specWithArgs([]string{"--c"}),
					annotations: map[string]string{v1alpha2.MergeAppendFieldsAnnotation: argsPath},
				},
			},
			wantArgs: []string{"--b", "--c"},
		},
		{
			name: "first config annotation is a no-op (nothing to append to)",
			specs: []annotatedSpec{
				{
					spec:        specWithArgs([]string{"--a"}),
					annotations: map[string]string{v1alpha2.MergeAppendFieldsAnnotation: argsPath},
				},
				{spec: specWithArgs([]string{"--b"})},
			},
			wantArgs: []string{"--b"},
		},
		{
			name:     "single config returns as-is",
			specs:    []annotatedSpec{{spec: specWithArgs([]string{"--a"})}},
			wantArgs: []string{"--a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

			got, err := mergeAnnotatedSpecs(ctx, tt.specs)
			if err != nil {
				t.Fatalf("mergeAnnotatedSpecs() error: %v", err)
			}

			if got.Template == nil || len(got.Template.Containers) == 0 {
				t.Fatal("expected at least one container in result")
			}
			if diff := cmp.Diff(tt.wantArgs, got.Template.Containers[0].Args); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeAnnotatedSpecs_OtherFieldsPreserved(t *testing.T) {
	ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

	specs := []annotatedSpec{
		{
			spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Replicas: ptr.To[int32](1),
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "base-image:v1", Args: []string{"--base-flag"}},
						},
					},
				},
			},
		},
		{
			spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Replicas: ptr.To[int32](3),
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Args: []string{"--override-flag"}},
						},
					},
				},
			},
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=main].args",
			},
		},
	}

	got, err := mergeAnnotatedSpecs(ctx, specs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if diff := cmp.Diff([]string{"--base-flag", "--override-flag"}, got.Template.Containers[0].Args); diff != "" {
		t.Errorf("args mismatch (-want +got):\n%s", diff)
	}
	if *got.Replicas != 3 {
		t.Errorf("replicas = %d, want 3", *got.Replicas)
	}
	if got.Template.Containers[0].Image != "base-image:v1" {
		t.Errorf("image = %q, want %q", got.Template.Containers[0].Image, "base-image:v1")
	}
}

// ---------------------------------------------------------------------------
// mergeSpecsWithAppend: single-pair merge with append paths
// ---------------------------------------------------------------------------

func TestMergeSpecsWithAppend(t *testing.T) {
	tests := []struct {
		name        string
		base        v1alpha2.LLMInferenceServiceSpec
		override    v1alpha2.LLMInferenceServiceSpec
		appendPaths []string
		wantArgs    []string
	}{
		{
			name:        "append container args",
			base:        specWithArgs([]string{"--base-flag"}),
			override:    specWithArgs([]string{"--override-flag"}),
			appendPaths: []string{"template.containers.[name=main].args"},
			wantArgs:    []string{"--base-flag", "--override-flag"},
		},
		{
			name:        "no append paths preserves replace behavior",
			base:        specWithArgs([]string{"--base-flag"}),
			override:    specWithArgs([]string{"--override-flag"}),
			appendPaths: nil,
			wantArgs:    []string{"--override-flag"},
		},
		{
			name:        "append with multiple base args",
			base:        specWithArgs([]string{"--model-name=llama", "--max-len=4096"}),
			override:    specWithArgs([]string{"--enable-lora"}),
			appendPaths: []string{"template.containers.[name=main].args"},
			wantArgs:    []string{"--model-name=llama", "--max-len=4096", "--enable-lora"},
		},
		{
			name: "append when only override has args (no base values to prepend)",
			base: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{Name: "main"}},
					},
				},
			},
			override:    specWithArgs([]string{"--flag"}),
			appendPaths: []string{"template.containers.[name=main].args"},
			wantArgs:    []string{"--flag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

			got, err := mergeSpecsWithAppend(ctx, tt.base, tt.override, mustParseFieldPaths(tt.appendPaths...))
			if err != nil {
				t.Fatalf("mergeSpecsWithAppend() error: %v", err)
			}

			if len(got.Template.Containers) == 0 {
				t.Fatal("expected at least one container in result")
			}
			if diff := cmp.Diff(tt.wantArgs, got.Template.Containers[0].Args); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Append across different PodSpec locations (worker, scheduler, prefill)
// ---------------------------------------------------------------------------

func TestMergeSpecsWithAppend_WorkerArgs(t *testing.T) {
	ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

	base := v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Worker: &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Args: []string{"--worker-base"}}},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Worker: &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Args: []string{"--worker-override"}}},
			},
		},
	}

	got, err := mergeSpecsWithAppend(ctx, base, override, mustParseFieldPaths("worker.containers.[name=main].args"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff([]string{"--worker-base", "--worker-override"}, got.Worker.Containers[0].Args); diff != "" {
		t.Errorf("worker args mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeSpecsWithAppend_RouterSchedulerArgs(t *testing.T) {
	ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

	base := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Args: []string{"--scheduler-base"}}},
				},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Args: []string{"--scheduler-override"}}},
				},
			},
		},
	}

	got, err := mergeSpecsWithAppend(ctx, base, override, mustParseFieldPaths("router.scheduler.template.containers.[name=main].args"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff([]string{"--scheduler-base", "--scheduler-override"}, got.Router.Scheduler.Template.Containers[0].Args); diff != "" {
		t.Errorf("scheduler args mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeSpecsWithAppend_PrefillArgs(t *testing.T) {
	ctx := log.IntoContext(t.Context(), pkgtest.NewTestLogger(t))

	base := v1alpha2.LLMInferenceServiceSpec{
		Prefill: &v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Args: []string{"--prefill-base"}}},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Prefill: &v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Args: []string{"--prefill-override"}}},
			},
		},
	}

	got, err := mergeSpecsWithAppend(ctx, base, override, mustParseFieldPaths("prefill.template.containers.[name=main].args"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff([]string{"--prefill-base", "--prefill-override"}, got.Prefill.Template.Containers[0].Args); diff != "" {
		t.Errorf("prefill args mismatch (-want +got):\n%s", diff)
	}
}

// ---------------------------------------------------------------------------
// concatSequenceFields: low-level JSON fixup
// ---------------------------------------------------------------------------

func TestConcatSequenceFields(t *testing.T) {
	tests := []struct {
		name         string
		baseJSON     string
		overrideJSON string
		mergedJSON   string
		paths        []string
		wantArgs     []string
	}{
		{
			name:         "both sides have args: concatenated",
			baseJSON:     `{"template":{"containers":[{"name":"main","args":["--base"]}]}}`,
			overrideJSON: `{"template":{"containers":[{"name":"main","args":["--override"]}]}}`,
			mergedJSON:   `{"template":{"containers":[{"name":"main","args":["--override"]}]}}`,
			paths:        []string{"template.containers.[name=main].args"},
			wantArgs:     []string{"--base", "--override"},
		},
		{
			name:         "only override has values: no-op",
			baseJSON:     `{"template":{"containers":[{"name":"main"}]}}`,
			overrideJSON: `{"template":{"containers":[{"name":"main","args":["--flag"]}]}}`,
			mergedJSON:   `{"template":{"containers":[{"name":"main","args":["--flag"]}]}}`,
			paths:        []string{"template.containers.[name=main].args"},
			wantArgs:     []string{"--flag"},
		},
		{
			name:         "only base has values: no-op",
			baseJSON:     `{"template":{"containers":[{"name":"main","args":["--base"]}]}}`,
			overrideJSON: `{"template":{"containers":[{"name":"main"}]}}`,
			mergedJSON:   `{"template":{"containers":[{"name":"main","args":["--base"]}]}}`,
			paths:        []string{"template.containers.[name=main].args"},
			wantArgs:     []string{"--base"},
		},
		{
			name:         "path not found: no-op",
			baseJSON:     `{"template":{}}`,
			overrideJSON: `{"template":{}}`,
			mergedJSON:   `{"template":{}}`,
			paths:        []string{"template.containers.[name=main].args"},
			wantArgs:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultJSON, err := concatSequenceFields(
				[]byte(tt.baseJSON), []byte(tt.overrideJSON), []byte(tt.mergedJSON),
				mustParseFieldPaths(tt.paths...),
			)
			if err != nil {
				t.Fatalf("concatSequenceFields() error: %v", err)
			}

			if tt.wantArgs == nil {
				return
			}

			elems, err := lookupSequence(resultJSON, mustParseFieldPaths("template.containers.[name=main].args")[0])
			if err != nil {
				t.Fatalf("lookupSequence on result: %v", err)
			}
			var gotArgs []string
			for _, elem := range elems {
				s, _ := elem.String()
				gotArgs = append(gotArgs, strings.TrimSpace(s))
			}
			if diff := cmp.Diff(tt.wantArgs, gotArgs); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Parsing utilities
// ---------------------------------------------------------------------------

func TestParseFieldPath_FromV1Alpha2(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple field",
			input: "template",
			want:  []string{"template"},
		},
		{
			name:  "dotted path",
			input: "template.containers",
			want:  []string{"template", "containers"},
		},
		{
			name:  "Kustomize-style array filter",
			input: "template.containers.[name=kserve-container].args",
			want:  []string{"template", "containers", "[name=kserve-container]", "args"},
		},
		{
			name:  "deep path through router",
			input: "router.scheduler.template.containers.[name=main].command",
			want:  []string{"router", "scheduler", "template", "containers", "[name=main]", "command"},
		},
		{
			name:  "pod-level field",
			input: "prefill.template.tolerations",
			want:  []string{"prefill", "template", "tolerations"},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1alpha2.ParseFieldPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("v1alpha2.ParseFieldPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("v1alpha2.ParseFieldPath(%q) mismatch (-want +got):\n%s", tt.input, diff)
				}
			}
		})
	}
}

func TestParseMergeAppendFieldPaths(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "absent annotation",
			annotations: map[string]string{},
			wantCount:   0,
		},
		{
			name:        "nil annotations",
			annotations: nil,
			wantCount:   0,
		},
		{
			name: "single path",
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=main].args",
			},
			wantCount: 1,
		},
		{
			name: "comma separated",
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=main].args,worker.tolerations",
			},
			wantCount: 2,
		},
		{
			name: "newline separated",
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=main].args\nworker.tolerations",
			},
			wantCount: 2,
		},
		{
			name: "blank lines and whitespace are trimmed",
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "  template.containers.[name=main].args  \n\n  worker.tolerations  \n",
			},
			wantCount: 2,
		},
		{
			name: "empty annotation value",
			annotations: map[string]string{
				v1alpha2.MergeAppendFieldsAnnotation: "",
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1alpha2.ParseMergeAppendFieldPaths(tt.annotations)
			if (err != nil) != tt.wantErr {
				t.Errorf("v1alpha2.ParseMergeAppendFieldPaths() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.wantCount {
				t.Errorf("v1alpha2.ParseMergeAppendFieldPaths() returned %d paths, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestLookupSequence(t *testing.T) {
	jsonData := []byte(`{
		"template": {
			"containers": [
				{"name": "main", "args": ["--flag1", "--flag2"]},
				{"name": "sidecar", "command": ["/bin/sh"]}
			],
			"tolerations": [
				{"key": "gpu", "operator": "Exists"}
			]
		}
	}`)

	tests := []struct {
		name      string
		path      string
		wantCount int
		wantNil   bool
	}{
		{name: "container args", path: "template.containers.[name=main].args", wantCount: 2},
		{name: "container command", path: "template.containers.[name=sidecar].command", wantCount: 1},
		{name: "pod-level tolerations", path: "template.tolerations", wantCount: 1},
		{name: "missing container", path: "template.containers.[name=nonexistent].args", wantNil: true},
		{name: "missing field", path: "template.nonexistent", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments, _ := v1alpha2.ParseFieldPath(tt.path)
			got, err := lookupSequence(jsonData, segments)
			if err != nil && !tt.wantNil {
				t.Fatalf("lookupSequence() error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %d elements", len(got))
				}
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("got %d elements, want %d", len(got), tt.wantCount)
			}
		})
	}
}
