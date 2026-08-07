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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
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

func newLoRARewriteScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	return scheme
}

func TestRewriteLoRAAdaptersFromLocalModelCache(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "cached-adapter", uri: "hf://org/adapter", scheme: constants.HfURIPrefix},
		{name: "remote-adapter", uri: "hf://org/remote", scheme: constants.HfURIPrefix},
	}

	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "adapter-cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "hf://org/adapter",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(newLoRARewriteScheme(t)).WithObjects(cache).Build()

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"cached-adapter":{"cache":"adapter-cache"}}`,
			},
		},
	}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(t.Context(), c, llmSvc, adapters)
	require.NoError(t, err)
	require.Len(t, rewritten, 2)

	assert.Equal(t, constants.PvcURIPrefix, rewritten[0].scheme)
	assert.True(t, strings.HasPrefix(rewritten[0].uri, "pvc://adapter-cache-gpu1/models/"))
	assert.Equal(t, constants.HfURIPrefix, rewritten[1].scheme)
	assert.Equal(t, "hf://org/remote", rewritten[1].uri)

	pairs := collectLoRADownloadPairs(rewritten)
	require.Len(t, pairs, 1)
	assert.Equal(t, "hf://org/remote", pairs[0].uri)
}

func TestRewriteLoRAAdaptersFromLocalModelCache_NoAnnotation(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "a", uri: "hf://org/a", scheme: constants.HfURIPrefix},
	}
	llmSvc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	c := fake.NewClientBuilder().WithScheme(newLoRARewriteScheme(t)).Build()

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(t.Context(), c, llmSvc, adapters)
	require.NoError(t, err)
	assert.Equal(t, adapters, rewritten)
}

func TestRewriteLoRAAdaptersFromLocalModelCache_Subpath(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "my-adapter", uri: "hf://org/model/subdir", scheme: constants.HfURIPrefix},
	}
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "c"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "hf://org/model",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(newLoRARewriteScheme(t)).WithObjects(cache).Build()

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"my-adapter":{"cache":"c"}}`,
			},
		},
	}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(t.Context(), c, llmSvc, adapters)
	require.NoError(t, err)
	assert.Contains(t, rewritten[0].uri, "/subdir")
	assert.Equal(t, constants.PvcURIPrefix, rewritten[0].scheme)
}

func TestRewriteLoRAAdaptersFromLocalModelCache_NamespaceScoped(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "my-adapter", uri: "hf://org/adapter", scheme: constants.HfURIPrefix},
	}
	nsCache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-adapter-cache", Namespace: "default"},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://org/adapter",
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu2"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(newLoRARewriteScheme(t)).WithObjects(nsCache).Build()

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"my-adapter":{"cache":"ns-adapter-cache","namespace":"default"}}`,
			},
		},
	}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(t.Context(), c, llmSvc, adapters)
	require.NoError(t, err)
	assert.Equal(t, constants.PvcURIPrefix, rewritten[0].scheme)
	assert.True(t, strings.HasPrefix(rewritten[0].uri, "pvc://ns-adapter-cache-gpu2/models/"))
}

func TestRewriteLoRAAdaptersFromLocalModelCache_MissingCache(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "my-adapter", uri: "hf://org/adapter", scheme: constants.HfURIPrefix},
	}
	c := fake.NewClientBuilder().WithScheme(newLoRARewriteScheme(t)).Build()
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"my-adapter":{"cache":"missing"}}`,
			},
		},
	}

	_, err := rewriteLoRAAdaptersFromLocalModelCache(t.Context(), c, llmSvc, adapters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get LocalModelCache")
}

func TestBuildCachedPVCURI_MatchesLocalModelCacheHelper(t *testing.T) {
	t.Parallel()
	got := localmodelcache.BuildCachedPVCURI("hf://org/model", "cache-gpu1", "hf://org/model/extra")
	assert.True(t, strings.HasPrefix(got, "pvc://cache-gpu1/models/"))
	assert.True(t, strings.HasSuffix(got, "/extra"))
}
