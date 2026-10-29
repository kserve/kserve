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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/tracing"
)

// vllmArgMapper formats CLI flags for the vLLM inference engine.
type vllmArgMapper struct{}

func (v *vllmArgMapper) SupportsFeature(_ RuntimeFeature) bool {
	return true // vLLM supports all current features
}

// AddLoRAArgs appends vLLM-native LoRA flags to main.Args. Each adapter is
// serialised as a JSON object for --lora-modules, single-quoted so it survives
// bash eval in the config template entrypoint.
func (v *vllmArgMapper) AddLoRAArgs(main *corev1.Container, adapters []loraAdapterRef, maxRank, maxAdapters, maxCpuAdapters *int32) {
	argv := make([]string, 0, 5+len(adapters))
	argv = append(argv, "--enable-lora")
	if maxRank != nil {
		argv = append(argv, fmt.Sprintf("--max-lora-rank=%d", *maxRank))
	}
	if maxAdapters != nil {
		argv = append(argv, fmt.Sprintf("--max-loras=%d", *maxAdapters))
	}
	if maxCpuAdapters != nil {
		argv = append(argv, fmt.Sprintf("--max-cpu-loras=%d", *maxCpuAdapters))
	}
	argv = append(argv, "--lora-modules")
	// Single-quote each JSON object so it survives bash eval in the config template
	// entrypoint (eval "vllm serve ... $@" would otherwise strip quotes and expand braces).
	for _, a := range adapters {
		m, _ := json.Marshal(loraModuleJSON(a))
		argv = append(argv, "'"+string(m)+"'")
	}

	// Place injected args before so that user-provided arguments could override the injected arguments
	main.Args = append(argv, main.Args...)
}

// InjectServerTracing appends vLLM-native tracing flags
// (--otlp-traces-endpoint, --collect-detailed-traces all) and standard OTEL
// env vars.
func (v *vllmArgMapper) InjectServerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, container *corev1.Container) bool {
	if !injectVLLMTracing(t, namespace, llmisvcName, defaultServerServiceName+roleSuffix, container) {
		return false
	}

	if !tracing.HasArg(container.Args, "--collect-detailed-traces") {
		container.Args = append(container.Args, "--collect-detailed-traces", "all")
	}

	return true
}
