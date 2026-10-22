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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/tracing"
)

// sglangArgMapper formats CLI flags for the SGLang inference engine.
type sglangArgMapper struct{}

func (s *sglangArgMapper) SupportsFeature(f RuntimeFeature) bool {
	switch f {
	case FeatureLoRA, FeatureTracing:
		return true
	case FeatureKVCacheOffloading, FeatureTLS, FeatureMultiNode, FeaturePrefillDecode:
		return false
	default:
		return false
	}
}

// AddLoRAArgs appends SGLang-native LoRA flags to main.Args.
// SGLang uses --lora-paths name=path instead of vLLM's JSON --lora-modules.
func (s *sglangArgMapper) AddLoRAArgs(main *corev1.Container, adapters []loraAdapterRef, maxRank, maxAdapters, maxCpuAdapters *int32) {
	argv := make([]string, 0, 5+len(adapters))
	argv = append(argv, "--enable-lora")
	if maxRank != nil {
		argv = append(argv, fmt.Sprintf("--max-lora-rank=%d", *maxRank))
	}
	// SGLang counts every --lora-paths entry against these limits. KServe
	// emits a short and a publisher-qualified alias for each logical adapter,
	// so an explicitly configured logical limit must cover both entries.
	if maxAdapters != nil {
		argv = append(argv, fmt.Sprintf("--max-loras-per-batch=%d", max(int(*maxAdapters), len(adapters))))
	}
	if maxCpuAdapters != nil {
		argv = append(argv, fmt.Sprintf("--max-loaded-loras=%d", max(int(*maxCpuAdapters), len(adapters))))
	}
	argv = append(argv, "--lora-paths")
	for _, a := range adapters {
		argv = append(argv, a.Name+"="+a.Path)
	}

	// Place injected args before so that user-provided arguments could override the injected arguments
	main.Args = append(argv, main.Args...)
}

// InjectServerTracing appends SGLang-native tracing flags (--enable-trace,
// --otlp-traces-endpoint with scheme stripped) and standard OTEL env vars.
// SGLang expects a bare host:port endpoint, unlike vLLM which accepts the full URL.
func (s *sglangArgMapper) InjectServerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	if endpoint == "" {
		return false
	}

	// SGLang expects a bare host:port, strip the http(s):// scheme.
	bareEndpoint := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")

	if !tracing.HasArg(container.Args, "--otlp-traces-endpoint") {
		container.Args = append(container.Args, "--otlp-traces-endpoint", bareEndpoint)
	}

	if !tracing.HasArg(container.Args, "--enable-trace") {
		container.Args = append(container.Args, "--enable-trace")
	}

	appendServerTracingEnvVars(container, t, namespace, llmisvcName, defaultServerServiceName+roleSuffix)
	return true
}
