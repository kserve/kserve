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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// ---------------------------------------------------------------------------
// mapperForRuntime
// ---------------------------------------------------------------------------

func TestMapperForRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		runtime    *string
		wantVLLM   bool
		wantSGLang bool
	}{
		{name: "nil defaults to vllm", runtime: nil, wantVLLM: true},
		{name: "empty string defaults to vllm", runtime: ptr.To(""), wantVLLM: true},
		{name: "sglang runtime", runtime: ptr.To(v1alpha2.SGLangRuntimeName), wantSGLang: true},
		{name: "custom runtime uses the warned vllm-compatible mapping", runtime: ptr.To("team-vllm-runtime"), wantVLLM: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := mapperForRuntime(tt.runtime)
			_, isVLLM := m.(*vllmArgMapper)
			_, isSGLang := m.(*sglangArgMapper)
			assert.Equal(t, tt.wantVLLM, isVLLM, "vllm mapper")
			assert.Equal(t, tt.wantSGLang, isSGLang, "sglang mapper")
		})
	}
}

// ---------------------------------------------------------------------------
// vllmArgMapper.SupportsFeature
// ---------------------------------------------------------------------------

func TestVLLMSupportsFeature(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	for _, f := range []RuntimeFeature{FeatureLoRA, FeatureTracing, FeatureKVCacheOffloading, FeatureTLS, FeatureMultiNode, FeaturePrefillDecode} {
		assert.True(t, m.SupportsFeature(f), "vLLM should support feature %d", f)
	}
}

// ---------------------------------------------------------------------------
// sglangArgMapper.SupportsFeature
// ---------------------------------------------------------------------------

func TestSGLangSupportsFeature(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	assert.True(t, m.SupportsFeature(FeatureLoRA))
	assert.True(t, m.SupportsFeature(FeatureTracing))
	assert.False(t, m.SupportsFeature(FeatureKVCacheOffloading))
	assert.False(t, m.SupportsFeature(FeatureTLS))
	assert.False(t, m.SupportsFeature(FeatureMultiNode))
	assert.False(t, m.SupportsFeature(FeaturePrefillDecode))
}

// ---------------------------------------------------------------------------
// vllmArgMapper.AddLoRAArgs
// ---------------------------------------------------------------------------

func TestVLLMAddLoRAArgs_AllParams(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main", Args: []string{"--user-flag"}}
	adapters := []loraAdapterRef{
		{Name: "billing", Path: "/mnt/lora/billing"},
		{Name: "publishers/ns/models/billing", Path: "/mnt/lora/billing"},
	}
	m.AddLoRAArgs(c, adapters, ptr.To(int32(64)), ptr.To(int32(4)), ptr.To(int32(8)))

	want := []string{
		"--enable-lora",
		"--max-lora-rank=64",
		"--max-loras=4",
		"--max-cpu-loras=8",
		"--lora-modules",
		`'{"name":"billing","path":"/mnt/lora/billing"}'`,
		`'{"name":"publishers/ns/models/billing","path":"/mnt/lora/billing"}'`,
		"--user-flag",
	}
	require.Equal(t, want, c.Args)
}

func TestVLLMAddLoRAArgs_NoOptionalParams(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main", Args: []string{"--existing"}}
	adapters := []loraAdapterRef{
		{Name: "a", Path: "/mnt/lora/a"},
	}
	m.AddLoRAArgs(c, adapters, nil, nil, nil)

	want := []string{
		"--enable-lora",
		"--lora-modules",
		`'{"name":"a","path":"/mnt/lora/a"}'`,
		"--existing",
	}
	require.Equal(t, want, c.Args)
}

func TestVLLMAddLoRAArgs_PrependedBeforeUserArgs(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main", Args: []string{"--max-lora-rank=128"}}
	adapters := []loraAdapterRef{{Name: "x", Path: "/mnt/lora/x"}}
	m.AddLoRAArgs(c, adapters, ptr.To(int32(32)), nil, nil)

	// Injected args come first, user args follow — user can override.
	assert.Equal(t, "--enable-lora", c.Args[0])
	assert.Equal(t, "--max-lora-rank=128", c.Args[len(c.Args)-1])
}

// ---------------------------------------------------------------------------
// sglangArgMapper.AddLoRAArgs
// ---------------------------------------------------------------------------

func TestSGLangAddLoRAArgs_AllParams(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main", Args: []string{"--user-flag"}}
	adapters := []loraAdapterRef{
		{Name: "billing", Path: "/mnt/lora/billing"},
		{Name: "gaming", Path: "/mnt/lora/gaming"},
	}
	m.AddLoRAArgs(c, adapters, ptr.To(int32(64)), ptr.To(int32(4)), ptr.To(int32(8)))

	want := []string{
		"--enable-lora",
		"--max-lora-rank=64",
		"--max-loras-per-batch=4",
		"--max-loaded-loras=8",
		"--lora-paths",
		"billing=/mnt/lora/billing",
		"gaming=/mnt/lora/gaming",
		"--user-flag",
	}
	require.Equal(t, want, c.Args)
}

func TestSGLangAddLoRAArgs_NoOptionalParams(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	adapters := []loraAdapterRef{
		{Name: "a", Path: "/mnt/lora/a"},
	}
	m.AddLoRAArgs(c, adapters, nil, nil, nil)

	want := []string{
		"--enable-lora",
		"--lora-paths",
		"a=/mnt/lora/a",
	}
	require.Equal(t, want, c.Args)
}

func TestSGLangAddLoRAArgs_PrependedBeforeUserArgs(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main", Args: []string{"--tp", "4"}}
	adapters := []loraAdapterRef{{Name: "x", Path: "/mnt/lora/x"}}
	m.AddLoRAArgs(c, adapters, nil, nil, nil)

	assert.Equal(t, "--enable-lora", c.Args[0])
	assert.Equal(t, "4", c.Args[len(c.Args)-1])
}

// ---------------------------------------------------------------------------
// vllmArgMapper.InjectServerTracing
// ---------------------------------------------------------------------------

func TestVLLMInjectServerTracing_FullSpec(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
		Exporter:         ptr.To("otlp"),
		Sampler:          ptr.To("always_on"),
		SamplerArg:       ptr.To("1.0"),
	}
	mutated := m.InjectServerTracing(spec, "test-ns", "my-llm", "-decode", c)
	assert.True(t, mutated)

	assert.Contains(t, c.Args, "--otlp-traces-endpoint")
	assert.Contains(t, c.Args, "http://collector:4317")
	assert.Contains(t, c.Args, "--collect-detailed-traces")
	assert.Contains(t, c.Args, "all")

	envMap := envToMap(c.Env)
	assert.Equal(t, "inference-server-decode", envMap["OTEL_SERVICE_NAME"])
	assert.Equal(t, "http://collector:4317", envMap["OTEL_EXPORTER_OTLP_ENDPOINT"])
	assert.Equal(t, "otlp", envMap["OTEL_TRACES_EXPORTER"])
	assert.Equal(t, "always_on", envMap["OTEL_TRACES_SAMPLER"])
	assert.Equal(t, "1.0", envMap["OTEL_TRACES_SAMPLER_ARG"])
	assert.Contains(t, envMap["OTEL_RESOURCE_ATTRIBUTES"], "k8s.namespace.name=test-ns")
	assert.Contains(t, envMap["OTEL_RESOURCE_ATTRIBUTES"], "llmisvc.name=my-llm")
}

func TestVLLMInjectServerTracing_NilSpec(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main"}
	assert.False(t, m.InjectServerTracing(nil, "ns", "svc", "-decode", c))
	assert.Empty(t, c.Args)
	assert.Empty(t, c.Env)
}

func TestVLLMInjectServerTracing_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{Name: "main"}
	assert.False(t, m.InjectServerTracing(&v1alpha2.TracingSpec{}, "ns", "svc", "-decode", c))
	assert.Empty(t, c.Args)
}

func TestVLLMInjectServerTracing_DoesNotDuplicateArgs(t *testing.T) {
	t.Parallel()
	m := &vllmArgMapper{}
	c := &corev1.Container{
		Name: "main",
		Args: []string{"--otlp-traces-endpoint", "http://existing:4317", "--collect-detailed-traces", "model"},
	}
	m.InjectServerTracing(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://new:4317"),
	}, "ns", "svc", "-decode", c)

	count := 0
	for _, a := range c.Args {
		if a == "--otlp-traces-endpoint" {
			count++
		}
	}
	assert.Equal(t, 1, count)
	// Existing value preserved, not overwritten.
	assert.Equal(t, "http://existing:4317", c.Args[1])
}

// ---------------------------------------------------------------------------
// sglangArgMapper.InjectServerTracing
// ---------------------------------------------------------------------------

func TestSGLangInjectServerTracing_FullSpec(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
		Exporter:         ptr.To("otlp"),
		Sampler:          ptr.To("always_on"),
		SamplerArg:       ptr.To("1.0"),
	}
	mutated := m.InjectServerTracing(spec, "test-ns", "my-llm", "-decode", c)
	assert.True(t, mutated)

	// SGLang uses --enable-trace, not --collect-detailed-traces
	assert.Contains(t, c.Args, "--enable-trace")
	assert.NotContains(t, c.Args, "--collect-detailed-traces")

	// SGLang strips the http:// scheme from the endpoint.
	assert.Contains(t, c.Args, "--otlp-traces-endpoint")
	assert.Contains(t, c.Args, "collector:4317")
	assert.NotContains(t, c.Args, "http://collector:4317")

	// OTEL env vars are shared with vLLM.
	envMap := envToMap(c.Env)
	assert.Equal(t, "inference-server-decode", envMap["OTEL_SERVICE_NAME"])
	assert.Equal(t, "http://collector:4317", envMap["OTEL_EXPORTER_OTLP_ENDPOINT"])
}

func TestSGLangInjectServerTracing_StripsHTTPS(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("https://collector.svc:4317"),
	}
	m.InjectServerTracing(spec, "ns", "svc", "-decode", c)

	// https:// prefix stripped for SGLang CLI arg.
	assert.Contains(t, c.Args, "collector.svc:4317")
	assert.NotContains(t, c.Args, "https://collector.svc:4317")
}

func TestSGLangInjectServerTracing_BareHostPort(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("collector:4317"),
	}
	m.InjectServerTracing(spec, "ns", "svc", "-decode", c)

	// Already bare — no double-stripping.
	assert.Contains(t, c.Args, "collector:4317")
}

func TestSGLangInjectServerTracing_NilSpec(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	assert.False(t, m.InjectServerTracing(nil, "ns", "svc", "-decode", c))
	assert.Empty(t, c.Args)
	assert.Empty(t, c.Env)
}

func TestSGLangInjectServerTracing_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{Name: "main"}
	assert.False(t, m.InjectServerTracing(&v1alpha2.TracingSpec{}, "ns", "svc", "-decode", c))
	assert.Empty(t, c.Args)
}

func TestSGLangInjectServerTracing_DoesNotDuplicateArgs(t *testing.T) {
	t.Parallel()
	m := &sglangArgMapper{}
	c := &corev1.Container{
		Name: "main",
		Args: []string{"--otlp-traces-endpoint", "existing:4317", "--enable-trace"},
	}
	m.InjectServerTracing(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://new:4317"),
	}, "ns", "svc", "-decode", c)

	endpointCount := 0
	traceCount := 0
	for _, a := range c.Args {
		if a == "--otlp-traces-endpoint" {
			endpointCount++
		}
		if a == "--enable-trace" {
			traceCount++
		}
	}
	assert.Equal(t, 1, endpointCount)
	assert.Equal(t, 1, traceCount)
	assert.Equal(t, "existing:4317", c.Args[1])
}

// ---------------------------------------------------------------------------
// userSuppliedLoRAConfig — runtime-aware
// ---------------------------------------------------------------------------

func TestUserSuppliedLoRAConfig_VLLMArgs(t *testing.T) {
	t.Parallel()
	assert.True(t, userSuppliedLoRAConfig(&corev1.Container{
		Args: []string{"--lora-modules", "x=y"},
	}, nil))
}

func TestUserSuppliedLoRAConfig_VLLMEnv(t *testing.T) {
	t.Parallel()
	assert.True(t, userSuppliedLoRAConfig(&corev1.Container{
		Env: []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "--lora-modules foo"}},
	}, nil))
}

func TestUserSuppliedLoRAConfig_VLLMNoMatch(t *testing.T) {
	t.Parallel()
	assert.False(t, userSuppliedLoRAConfig(&corev1.Container{
		Env: []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "--enable-lora"}},
	}, nil))
}

func TestUserSuppliedLoRAConfig_SGLangArgs(t *testing.T) {
	t.Parallel()
	sglang := ptr.To(v1alpha2.SGLangRuntimeName)
	assert.True(t, userSuppliedLoRAConfig(&corev1.Container{
		Args: []string{"--lora-paths", "a=/mnt/a"},
	}, sglang))
}

func TestUserSuppliedLoRAConfig_SGLangDoesNotCheckVLLMEnv(t *testing.T) {
	t.Parallel()
	sglang := ptr.To(v1alpha2.SGLangRuntimeName)
	// VLLM_ADDITIONAL_ARGS with --lora-modules should not trigger for SGLang.
	assert.False(t, userSuppliedLoRAConfig(&corev1.Container{
		Env: []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "--lora-modules foo"}},
	}, sglang))
}

func TestUserSuppliedLoRAConfig_SGLangNoMatch(t *testing.T) {
	t.Parallel()
	sglang := ptr.To(v1alpha2.SGLangRuntimeName)
	assert.False(t, userSuppliedLoRAConfig(&corev1.Container{
		Args: []string{"--enable-lora"},
	}, sglang))
}

// ---------------------------------------------------------------------------
// injectServerTracing (integration: dispatches through mapper)
// ---------------------------------------------------------------------------

func TestInjectServerTracing_VLLMDefault(t *testing.T) {
	t.Parallel()
	c := &corev1.Container{Name: "main"}
	mutated := injectServerTracing(nil, &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
	}, "ns", "svc", "-decode", c)
	assert.True(t, mutated)
	assert.Contains(t, c.Args, "--collect-detailed-traces")
}

func TestInjectServerTracing_SGLang(t *testing.T) {
	t.Parallel()
	sglang := ptr.To(v1alpha2.SGLangRuntimeName)
	c := &corev1.Container{Name: "main"}
	mutated := injectServerTracing(sglang, &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
	}, "ns", "svc", "-decode", c)
	assert.True(t, mutated)
	assert.Contains(t, c.Args, "--enable-trace")
	assert.NotContains(t, c.Args, "--collect-detailed-traces")
	// Scheme stripped for SGLang.
	assert.Contains(t, c.Args, "collector:4317")
}
