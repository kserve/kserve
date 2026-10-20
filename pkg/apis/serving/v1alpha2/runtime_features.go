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

package v1alpha2

import (
	"fmt"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Well-known runtime names with engine-specific argument mappings. An empty/nil
// runtime is treated as the default (vLLM); custom runtime resource names remain
// valid and use the vLLM-compatible mapping with an admission warning.
const (
	// SGLangRuntimeName is the well-known ClusterServingRuntime name shipped
	// with KServe for the SGLang engine.
	SGLangRuntimeName = "kserve-llm-sglang"
)

// Feature name constants used as keys in the runtime/feature support matrix.
const (
	FeatureNameLoRA              = "lora"
	FeatureNameTracing           = "tracing"
	FeatureNameKVCacheOffloading = "kvCacheOffloading"
	FeatureNameTLS               = "tls"
	FeatureNameMultiNode         = "multiNode"
	FeatureNamePrefillDecode     = "prefillDecode"
)

// runtimeFeatureMatrix maps each known runtime to the set of features it
// supports. The empty string key represents the default runtime (vLLM).
var runtimeFeatureMatrix = map[string]map[string]bool{
	"": { // vLLM (default)
		FeatureNameLoRA:              true,
		FeatureNameTracing:           true,
		FeatureNameKVCacheOffloading: true,
		FeatureNameTLS:               true,
		FeatureNameMultiNode:         true,
		FeatureNamePrefillDecode:     true,
	},
	SGLangRuntimeName: {
		FeatureNameLoRA:              true,
		FeatureNameTracing:           true,
		FeatureNameKVCacheOffloading: false,
		FeatureNameTLS:               false,
		FeatureNameMultiNode:         false,
		FeatureNamePrefillDecode:     false,
	},
}

// IsKnownRuntime reports whether the given runtime name has a well-known
// argument mapping. nil and "" are treated as the vLLM default.
func IsKnownRuntime(runtime *string) bool {
	if runtime == nil || *runtime == "" {
		return true
	}
	_, ok := runtimeFeatureMatrix[*runtime]
	return ok
}

// KnownRuntimeNames returns a sorted list of recognized non-empty runtime
// names, for use in validation error messages.
func KnownRuntimeNames() []string {
	names := make([]string, 0, len(runtimeFeatureMatrix))
	for k := range runtimeFeatureMatrix {
		if k != "" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names
}

// RuntimeWarnings reports non-fatal runtime selection ambiguity. Runtime is a
// ServingRuntime/ClusterServingRuntime resource name, so custom names remain
// valid for backward compatibility. Only well-known names select non-vLLM
// argument mappings; callers must be told when the vLLM-compatible mapping is
// being used rather than silently falling back.
func RuntimeWarnings(runtime *string) admission.Warnings {
	if runtime == nil || *runtime == "" || IsKnownRuntime(runtime) {
		return nil
	}
	return admission.Warnings{fmt.Sprintf(
		"runtime %q is not a well-known runtime; KServe will resolve the referenced ServingRuntime or ClusterServingRuntime but use vLLM-compatible arguments",
		*runtime,
	)}
}

// RuntimeSupportsFeature reports whether the named runtime supports the given
// feature. This is the webhook-side check; the controller has its own
// runtimeArgMapper.SupportsFeature for the safety-net path.
func RuntimeSupportsFeature(runtime *string, feature string) bool {
	r := ""
	if runtime != nil {
		r = *runtime
	}
	supported, ok := runtimeFeatureMatrix[r]
	if !ok {
		return false // unknown runtime supports nothing
	}
	return supported[feature]
}
