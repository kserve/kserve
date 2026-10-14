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
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// runtimeArgMapper encapsulates the CLI-flag differences between inference
// engines. Implementations are stateless; all input arrives via method args.
type runtimeArgMapper interface {
	// AddLoRAArgs appends engine-native LoRA flags to main.Args.
	// adapters is a list of (name, path) pairs already resolved by the controller.
	// maxRank/maxAdapters/maxCpuAdapters are nil when unset in the spec.
	AddLoRAArgs(main *corev1.Container, adapters []loraAdapterRef, maxRank, maxAdapters, maxCpuAdapters *int32)

	// InjectServerTracing appends engine-native tracing flags and env vars.
	// Returns true if the container was mutated.
	InjectServerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, container *corev1.Container) bool

	// SupportsFeature reports whether this engine supports the given feature.
	// Used by the controller as a safety net when the webhook did not catch a combo.
	SupportsFeature(f RuntimeFeature) bool
}

// loraAdapterRef is a (name, path) pair for LoRA adapter injection, decoupled
// from any engine-specific serialization format.
type loraAdapterRef struct {
	Name string
	Path string
}

// RuntimeFeature enumerates features that may be engine-specific.
type RuntimeFeature int

const (
	FeatureLoRA RuntimeFeature = iota
	FeatureTracing
	FeatureKVCacheOffloading
	FeatureTLS
	FeatureMultiNode     // pipeline-parallel, data-parallel
	FeaturePrefillDecode // P/D disaggregated serving
)

// mapperForRuntime returns the arg mapper for the given spec.runtime.
// Falls back to vLLM for nil/empty runtime (preserving existing behavior).
func mapperForRuntime(runtime *string) runtimeArgMapper {
	if runtime != nil && *runtime == SGLangServingRuntimeName {
		return &sglangArgMapper{}
	}
	return &vllmArgMapper{}
}
