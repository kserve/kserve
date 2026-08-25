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
	"errors"
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
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
	"github.com/kserve/kserve/pkg/utils"
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

func loRASpec(t *testing.T, nameToURI ...string) v1alpha2.LLMInferenceServiceSpec {
	t.Helper()
	if len(nameToURI)%2 != 0 {
		t.Fatal("loRASpec takes name/uri pairs")
	}
	base, err := apis.ParseURL("hf://org/base")
	if err != nil {
		t.Fatal(err)
	}
	adapters := make([]v1alpha2.LLMModelSpec, 0, len(nameToURI)/2)
	for i := 0; i < len(nameToURI); i += 2 {
		uri, err := apis.ParseURL(nameToURI[i+1])
		if err != nil {
			t.Fatal(err)
		}
		adapters = append(adapters, v1alpha2.LLMModelSpec{Name: ptr.To(nameToURI[i]), URI: *uri})
	}
	return v1alpha2.LLMInferenceServiceSpec{
		Model: v1alpha2.LLMModelSpec{URI: *base, LoRA: &v1alpha2.LoRASpec{Adapters: adapters}},
	}
}

// sanitizeLoRAPathSegment is lossy, so distinct adapter names can reduce to the
// same segment. Admission does not catch this: the duplicate check in ValidateLoRA
// compares raw names, not sanitized path segments.
func TestEnumerateLoRAAdaptersDistinctMountPaths(t *testing.T) {
	t.Parallel()

	adapters, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "pvc://adapters/sql-slash-v2",
		"sql-v2", "pvc://adapters/sql-dash-v2",
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 2 {
		t.Fatalf("got %d adapters, want 2", len(adapters))
	}
	if adapters[0].mountPath == adapters[1].mountPath {
		t.Errorf("adapters %q and %q share mount path %q",
			adapters[0].name, adapters[1].name, adapters[0].mountPath)
	}
}

// Adapters whose sanitized name is already unique must keep the path they have
// today, otherwise an upgrade moves the mount and restarts a healthy pod.
func TestEnumerateLoRAAdaptersMountPathStableWithoutCollision(t *testing.T) {
	t.Parallel()

	adapters, err := enumerateLoRAAdapters(loRASpec(t, "sql/v2", "pvc://adapters/sql-v2"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := adapters[0].mountPath, "/mnt/lora/sql-v2"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// Colliding hf:// adapters share a storage-initializer target, so one silently
// overwrites the other and both names serve whichever landed last.
func TestEnumerateLoRAAdaptersDistinctDownloadTargets(t *testing.T) {
	t.Parallel()

	adapters, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "hf://org/sql-slash-v2",
		"sql-v2", "hf://org/sql-dash-v2",
	))
	if err != nil {
		t.Fatal(err)
	}
	pairs := collectLoRADownloadPairs(adapters)
	if len(pairs) != 2 {
		t.Fatalf("got %d download pairs, want 2", len(pairs))
	}
	if pairs[0].path == pairs[1].path {
		t.Errorf("%q and %q both download to %q", pairs[0].uri, pairs[1].uri, pairs[0].path)
	}
}

// Two volume mounts on the same mountPath in one container are rejected by the API
// server (volumeMounts[1].mountPath must be unique), so the workload never starts.
func TestAttachLoRAAdaptersNoDuplicateMountPath(t *testing.T) {
	t.Parallel()

	spec := loRASpec(t,
		"sql/v2", "pvc://adapters/sql-slash-v2",
		"sql-v2", "pvc://adapters/sql-dash-v2",
	)
	adapters, err := enumerateLoRAAdapters(spec)
	if err != nil {
		t.Fatal(err)
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       spec,
	}
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}

	r := &LLMISVCReconciler{}
	if err := r.attachLoRAAdapters(t.Context(), llmSvc, podSpec, adapters); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]string, len(podSpec.Containers[0].VolumeMounts))
	for _, m := range podSpec.Containers[0].VolumeMounts {
		if prev, dup := seen[m.MountPath]; dup {
			t.Errorf("volumes %q and %q both mount at %q", prev, m.Name, m.MountPath)
		}
		seen[m.MountPath] = m.Name
	}
}

// Both members of a colliding pair are suffixed, so the outcome does not depend
// on which one the sort happens to put first.
func TestEnumerateLoRAAdaptersCollisionSuffix(t *testing.T) {
	t.Parallel()

	adapters, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "pvc://adapters/sql-slash-v2",
		"sql-v2", "pvc://adapters/sql-dash-v2",
	))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"sql-v2": "/mnt/lora/sql-v2-" + utils.ShortHash("sql-v2"),
		"sql/v2": "/mnt/lora/sql-v2-" + utils.ShortHash("sql/v2"),
	}
	if len(adapters) != len(want) {
		t.Fatalf("got %d adapters, want %d", len(adapters), len(want))
	}
	for _, a := range adapters {
		if got := a.mountPath; got != want[a.name] {
			t.Errorf("adapter %q: got %q want %q", a.name, got, want[a.name])
		}
	}
}

// Admission rejects duplicate names, so this only reaches specs that bypassed the
// webhook. The message must name the duplicate rather than report a self-collision.
func TestEnumerateLoRAAdaptersDuplicateName(t *testing.T) {
	t.Parallel()

	_, err := enumerateLoRAAdapters(loRASpec(t,
		"dup", "pvc://adapters/one",
		"dup", "pvc://adapters/two",
	))
	if err == nil {
		t.Fatal("expected an error for duplicate adapter names")
	}
	if want := `duplicate LoRA adapter name "dup"`; !strings.Contains(err.Error(), want) {
		t.Errorf("got %q, want it to contain %q", err.Error(), want)
	}
}

// A literal adapter name can equal the suffixed form another adapter resolves to.
// The occurrence count cannot see that, so the uniqueness check has to catch it.
func TestEnumerateLoRAAdaptersSuffixCollidesWithLiteralName(t *testing.T) {
	t.Parallel()

	shadow := "sql-v2-" + utils.ShortHash("sql/v2")
	_, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "pvc://adapters/sql-slash-v2",
		"sql-v2", "pvc://adapters/sql-dash-v2",
		shadow, "pvc://adapters/shadow",
	))
	if err == nil {
		t.Fatal("expected an error when a suffixed segment collides with a literal name")
	}
	if want := "/mnt/lora/" + shadow; !strings.Contains(err.Error(), want) {
		t.Errorf("got %q, want it to contain %q", err.Error(), want)
	}
}

// Adapters merged in from an LLMInferenceServiceConfig are never LoRA-validated, so
// unnamed adapters reach enumerateLoRAAdapters through a fully admitted spec. Two of
// them collide on the sanitize fallback, and the error must name that, not a duplicate "".
func TestEnumerateLoRAAdaptersUnnamedCollision(t *testing.T) {
	t.Parallel()

	spec := loRASpec(t, "placeholder", "pvc://adapters/one", "other", "pvc://adapters/two")
	spec.Model.LoRA.Adapters[0].Name = nil
	spec.Model.LoRA.Adapters[1].Name = nil

	_, err := enumerateLoRAAdapters(spec)
	if err == nil {
		t.Fatal("expected an error for two unnamed adapters")
	}
	if want := "two or more LoRA adapters have no name"; !strings.Contains(err.Error(), want) {
		t.Errorf("got %q, want it to contain %q", err.Error(), want)
	}
	// Adapters are sorted by name before segments are computed, so any index reported
	// here would point into the sorted slice rather than spec.model.lora.adapters.
	if strings.Contains(err.Error(), "index") {
		t.Errorf("message names an index that does not locate the adapter in the spec: %q", err.Error())
	}
}

// The reconciler dispatches the LoRAMountPathCollision event on errors.Is against this
// sentinel, so rewording a message must not quietly cost the event its reason.
func TestLoRAAdapterCollisionErrorsAreMarked(t *testing.T) {
	t.Parallel()

	shadow := "sql-v2-" + utils.ShortHash("sql/v2")
	unnamed := loRASpec(t, "a", "pvc://adapters/a", "b", "pvc://adapters/b")
	unnamed.Model.LoRA.Adapters[0].Name = nil
	unnamed.Model.LoRA.Adapters[1].Name = nil

	for name, spec := range map[string]v1alpha2.LLMInferenceServiceSpec{
		"duplicate name": loRASpec(t, "dup", "pvc://adapters/one", "dup", "pvc://adapters/two"),
		"shadowed suffix": loRASpec(t,
			"sql/v2", "pvc://adapters/one", "sql-v2", "pvc://adapters/two", shadow, "pvc://adapters/three"),
		"both unnamed": unnamed,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := enumerateLoRAAdapters(spec)
			if err == nil {
				t.Fatal("expected an error")
			}
			var collision *loRAMountPathCollisionError
			if !errors.As(err, &collision) {
				t.Errorf("error is not a collision: %v", err)
			}
		})
	}

	// Other enumerate failures must not claim the collision reason.
	_, err := enumerateLoRAAdapters(loRASpec(t, "a", "oci://registry/adapter"))
	if err == nil {
		t.Fatal("expected an error for oci://")
	}
	var collision *loRAMountPathCollisionError
	if errors.As(err, &collision) {
		t.Error("unsupported-scheme error must not be classified as a collision")
	}
}

// The upgrade-safety property that matters is not "a lone adapter keeps its path" but
// "a clean adapter keeps its path even when others in the same spec collide". Only the
// colliding group may move.
func TestEnumerateLoRAAdaptersCollisionDoesNotMoveBystanders(t *testing.T) {
	t.Parallel()

	adapters, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "pvc://adapters/one",
		"sql-v2", "pvc://adapters/two",
		"lonely", "pvc://adapters/three",
	))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"lonely": "/mnt/lora/lonely",
		"sql-v2": "/mnt/lora/sql-v2-" + utils.ShortHash("sql-v2"),
		"sql/v2": "/mnt/lora/sql-v2-" + utils.ShortHash("sql/v2"),
	}
	if len(adapters) != len(want) {
		t.Fatalf("got %d adapters, want %d", len(adapters), len(want))
	}
	for _, a := range adapters {
		if a.mountPath != want[a.name] {
			t.Errorf("adapter %q moved: got %q want %q", a.name, a.mountPath, want[a.name])
		}
	}
}

// Reordering spec.model.lora.adapters is a semantic no-op, so it must not move a mount.
// The doc comment on loraMountSegments claims this; without a test nothing enforces it.
func TestEnumerateLoRAAdaptersMountPathsAreOrderIndependent(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"sql/v2", "pvc://adapters/one"},
		{"sql-v2", "pvc://adapters/two"},
		{"lonely", "pvc://adapters/three"},
	}
	paths := func(order []int) map[string]string {
		t.Helper()
		flat := make([]string, 0, len(order)*2)
		for _, i := range order {
			flat = append(flat, pairs[i][0], pairs[i][1])
		}
		adapters, err := enumerateLoRAAdapters(loRASpec(t, flat...))
		if err != nil {
			t.Fatal(err)
		}
		got := make(map[string]string, len(adapters))
		for _, a := range adapters {
			got[a.name] = a.mountPath
		}
		return got
	}

	base := paths([]int{0, 1, 2})
	for _, order := range [][]int{{2, 1, 0}, {1, 0, 2}, {0, 2, 1}, {2, 0, 1}, {1, 2, 0}} {
		got := paths(order)
		for name, path := range base {
			if got[name] != path {
				t.Errorf("order %v moved adapter %q: got %q want %q", order, name, got[name], path)
			}
		}
	}
}

// An unsupported scheme is the more specific problem, so it must not be masked by a
// collision that happens to share the spec - the event reason is chosen from the error.
func TestEnumerateLoRAAdaptersSchemeErrorBeatsCollision(t *testing.T) {
	t.Parallel()

	_, err := enumerateLoRAAdapters(loRASpec(t,
		"sql/v2", "pvc://adapters/one",
		"sql-v2", "pvc://adapters/two",
		"oci-adapter", "oci://registry/adapter",
	))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "oci://") {
		t.Errorf("got %q, want the unsupported scheme reported", err.Error())
	}
	var collision *loRAMountPathCollisionError
	if errors.As(err, &collision) {
		t.Error("scheme error must not carry the collision reason")
	}
}
