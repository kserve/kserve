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

package llmisvc

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestSanitizeLoRAPathSegment(t *testing.T) {
	t.Parallel()
	if got, want := sanitizeLoRAPathSegment("k8s-lora"), "k8s-lora"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := sanitizeLoRAPathSegment("a/b"), "a-b"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := sanitizeLoRAPathSegment("@@@"), "---"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := sanitizeLoRAPathSegment(""), "adapter"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestAttachLoRAAdaptersVolumeNames covers PVC adapter volume naming. Volume
// names are part of the Deployment pod template, so deriving a different name
// for an unchanged adapter rolls every pod on controller upgrade - the
// hardcoded name below is what the controller produced before the sanitizer
// existed and must never change. The HF-style adapter name used to yield a
// volume name the API server rejected; it only has to yield a valid one now,
// as the exact rewrite is pkg/utils's contract, tested there.
func TestAttachLoRAAdaptersVolumeNames(t *testing.T) {
	t.Parallel()

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}
	adapters := []resolvedLoRAAdapter{
		{name: "billing-en-v1", mountPath: "/mnt/lora/billing-en-v1", uri: "pvc://claim/adapters/billing", scheme: constants.PvcURIPrefix},
		{name: "acme/x.r16", mountPath: "/mnt/lora/acme-x.r16", uri: "pvc://claim/adapters/x", scheme: constants.PvcURIPrefix},
	}

	r := &LLMISVCReconciler{}
	if err := r.attachLoRAAdapters(t.Context(), llmSvc, podSpec, adapters); err != nil {
		t.Fatal(err)
	}
	if len(podSpec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %v", podSpec.Volumes)
	}

	// The exact name running Deployments already use; a change rolls pods.
	if got, want := podSpec.Volumes[0].Name, "lora-pvc-billing-en-v1"; got != want {
		t.Fatalf("valid adapter name renamed: got %q want %q", got, want)
	}

	hfVol := podSpec.Volumes[1].Name
	if errs := validation.IsDNS1123Label(hfVol); len(errs) > 0 {
		t.Fatalf("volume name %q for HF-style adapter is not a valid DNS-1123 label: %v", hfVol, errs)
	}
	if hfVol == podSpec.Volumes[0].Name {
		t.Fatalf("adapters must not share volume name %q", hfVol)
	}

	for i, m := range podSpec.Containers[0].VolumeMounts {
		if m.Name != podSpec.Volumes[i].Name {
			t.Fatalf("mounts[%d]=%q does not match volume %q", i, m.Name, podSpec.Volumes[i].Name)
		}
	}
}

func TestAddLoRAVLLMArgs(t *testing.T) {
	t.Parallel()

	t.Run("all params set with model-routing aliases", func(t *testing.T) {
		c := &corev1.Container{Name: "main", Args: []string{"--user-flag"}}
		addLoRAVLLMArgs(c, []string{
			`{"name":"a","path":"/mnt/lora/a"}`,
			`{"name":"publishers/ns/models/a","path":"/mnt/lora/a"}`,
			`{"name":"b","path":"/mnt/lora/b"}`,
			`{"name":"publishers/ns/models/b","path":"/mnt/lora/b"}`,
		}, ptr.To(int32(64)), ptr.To(int32(2)), ptr.To(int32(4)))
		want := []string{
			"--enable-lora",
			"--max-lora-rank=64",
			"--max-loras=2",
			"--max-cpu-loras=4",
			"--lora-modules",
			`'{"name":"a","path":"/mnt/lora/a"}'`,
			`'{"name":"publishers/ns/models/a","path":"/mnt/lora/a"}'`,
			`'{"name":"b","path":"/mnt/lora/b"}'`,
			`'{"name":"publishers/ns/models/b","path":"/mnt/lora/b"}'`,
			"--user-flag",
		}
		if len(c.Args) != len(want) {
			t.Fatalf("len(args)=%d want %d: %v", len(c.Args), len(want), c.Args)
		}
		for i := range want {
			if c.Args[i] != want[i] {
				t.Fatalf("args[%d]=%q want %q (full %v)", i, c.Args[i], want[i], c.Args)
			}
		}
	})

	t.Run("no optional params", func(t *testing.T) {
		c := &corev1.Container{Name: "main", Args: []string{"--user-flag"}}
		addLoRAVLLMArgs(c, []string{
			`{"name":"a","path":"/mnt/lora/a"}`,
			`{"name":"publishers/ns/models/a","path":"/mnt/lora/a"}`,
		}, nil, nil, nil)
		want := []string{
			"--enable-lora",
			"--lora-modules",
			`'{"name":"a","path":"/mnt/lora/a"}'`,
			`'{"name":"publishers/ns/models/a","path":"/mnt/lora/a"}'`,
			"--user-flag",
		}
		if len(c.Args) != len(want) {
			t.Fatalf("len(args)=%d want %d: %v", len(c.Args), len(want), c.Args)
		}
		for i := range want {
			if c.Args[i] != want[i] {
				t.Fatalf("args[%d]=%q want %q (full %v)", i, c.Args[i], want[i], c.Args)
			}
		}
	})
}

func TestUserSuppliedLoRAConfig(t *testing.T) {
	t.Parallel()
	if !userSuppliedLoRAConfig(&corev1.Container{
		Env: []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "x --lora-modules y"}},
	}) {
		t.Fatal("expected true when VLLM_ADDITIONAL_ARGS has --lora-modules")
	}
	if userSuppliedLoRAConfig(&corev1.Container{
		Env: []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "--enable-lora"}},
	}) {
		t.Fatal("expected false without --lora-modules")
	}
	if !userSuppliedLoRAConfig(&corev1.Container{
		Args: []string{"--lora-modules", "x=y"},
	}) {
		t.Fatal("expected true when Args contains --lora-modules")
	}
}
