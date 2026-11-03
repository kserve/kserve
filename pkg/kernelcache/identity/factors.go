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

package identity

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/kernelcache/runtimeoptions"
)

type factorValueType int

const (
	factorValueString factorValueType = iota
	factorValueInteger
	factorValueBoolean
	factorValueCommaSeparated
	factorValueHash
)

// FactorDefinition describes one runtime setting that can affect a cache artifact.
// Keep option and environment definitions here so extraction and selection use the
// same vocabulary.
type FactorDefinition struct {
	Key             string
	OptionAliases   []string
	EnvironmentName string
	ValueType       factorValueType
	Scoring         bool
}

const (
	PipelineParallelSizeFactor = "option.pipelineParallelSize"
	MaxModelLenFactor          = "option.maxModelLen"
	DTypeFactor                = "option.dtype"
	QuantizationFactor         = "option.quantization"
	QuantizationConfigFactor   = "option.quantizationConfigHash"
	KVCacheDTypeFactor         = "option.kvCacheDtype"
	BlockSizeFactor            = "option.blockSize"
	MaxNumBatchedTokensFactor  = "option.maxNumBatchedTokens"
	MaxNumSeqsFactor           = "option.maxNumSeqs"
	ChunkedPrefillFactor       = "option.enableChunkedPrefill"
	DisableSlidingWindowFactor = "option.disableSlidingWindow"
	EnforceEagerFactor         = "option.enforceEager"
	AttentionBackendFactor     = "option.attentionBackend"
	CompilationConfigFactor    = "option.compilationConfigHash"
)

var runtimeConfigFactorDefinitions = []FactorDefinition{
	{Key: TensorParallelSizeFactor, OptionAliases: []string{"--tensor-parallel-size", "--tensor_parallel_size", "-tp", "--tp"}, ValueType: factorValueInteger, Scoring: true},
	{Key: PipelineParallelSizeFactor, OptionAliases: []string{"--pipeline-parallel-size", "--pipeline_parallel_size", "-pp", "--pp"}, ValueType: factorValueInteger, Scoring: true},
	{Key: MaxModelLenFactor, OptionAliases: []string{"--max-model-len", "--max_model_len"}, ValueType: factorValueInteger, Scoring: true},
	{Key: DTypeFactor, OptionAliases: []string{"--dtype"}, ValueType: factorValueString, Scoring: true},
	{Key: QuantizationFactor, OptionAliases: []string{"--quantization", "-q"}, ValueType: factorValueString, Scoring: true},
	{Key: QuantizationConfigFactor, OptionAliases: []string{"--quantization-config", "--quantization_config"}, ValueType: factorValueHash, Scoring: true},
	{Key: KVCacheDTypeFactor, OptionAliases: []string{"--kv-cache-dtype", "--kv_cache_dtype"}, ValueType: factorValueString, Scoring: true},
	{Key: BlockSizeFactor, OptionAliases: []string{"--block-size", "--block_size"}, ValueType: factorValueInteger, Scoring: true},
	{Key: MaxNumBatchedTokensFactor, OptionAliases: []string{"--max-num-batched-tokens", "--max_num_batched_tokens"}, ValueType: factorValueInteger, Scoring: true},
	{Key: MaxNumSeqsFactor, OptionAliases: []string{"--max-num-seqs", "--max_num_seqs"}, ValueType: factorValueInteger, Scoring: true},
	{Key: ChunkedPrefillFactor, OptionAliases: []string{"--enable-chunked-prefill", "--enable_chunked_prefill"}, ValueType: factorValueBoolean, Scoring: true},
	{Key: DisableSlidingWindowFactor, OptionAliases: []string{"--disable-sliding-window", "--disable_sliding_window"}, ValueType: factorValueBoolean, Scoring: true},
	{Key: EnforceEagerFactor, OptionAliases: []string{"--enforce-eager", "--enforce_eager"}, ValueType: factorValueBoolean, Scoring: true},
	{Key: AttentionBackendFactor, OptionAliases: []string{"--attention-backend", "--attention_backend"}, ValueType: factorValueString, Scoring: true},
	{Key: CompilationConfigFactor, OptionAliases: []string{"--compilation-config", "--compilation_config", "-cc"}, ValueType: factorValueHash, Scoring: true},

	{Key: "env.VLLM_USE_AOT_COMPILE", EnvironmentName: "VLLM_USE_AOT_COMPILE", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_USE_MEGA_AOT_ARTIFACT", EnvironmentName: "VLLM_USE_MEGA_AOT_ARTIFACT", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_USE_STANDALONE_COMPILE", EnvironmentName: "VLLM_USE_STANDALONE_COMPILE", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_ENABLE_PREGRAD_PASSES", EnvironmentName: "VLLM_ENABLE_PREGRAD_PASSES", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_ENABLE_INDUCTOR_MAX_AUTOTUNE", EnvironmentName: "VLLM_ENABLE_INDUCTOR_MAX_AUTOTUNE", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_ENABLE_INDUCTOR_COORDINATE_DESCENT_TUNING", EnvironmentName: "VLLM_ENABLE_INDUCTOR_COORDINATE_DESCENT_TUNING", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_FLOAT32_MATMUL_PRECISION", EnvironmentName: "VLLM_FLOAT32_MATMUL_PRECISION", ValueType: factorValueString, Scoring: true},
	{Key: "env.VLLM_BATCH_INVARIANT", EnvironmentName: "VLLM_BATCH_INVARIANT", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_TRITON_USE_TD", EnvironmentName: "VLLM_TRITON_USE_TD", ValueType: factorValueBoolean, Scoring: true},
	{Key: "env.VLLM_DISABLED_KERNELS", EnvironmentName: "VLLM_DISABLED_KERNELS", ValueType: factorValueCommaSeparated, Scoring: true},
	{Key: "env.VLLM_PP_LAYER_PARTITION", EnvironmentName: "VLLM_PP_LAYER_PARTITION", ValueType: factorValueString, Scoring: true},
}

// vLLMRuntimeOptionSpecs is the current vLLM runtime profile. A different
// runtime can register its own profile without changing the generic resolver.
var vLLMRuntimeOptionSpecs = buildRuntimeOptionSpecs()

func buildRuntimeOptionSpecs() []runtimeoptions.Spec {
	specs := make([]runtimeoptions.Spec, 0, len(runtimeConfigFactorDefinitions))
	for _, definition := range runtimeConfigFactorDefinitions {
		spec := runtimeoptions.Spec{
			Key:     definition.Key,
			Flags:   definition.OptionAliases,
			Boolean: definition.ValueType == factorValueBoolean,
		}
		if definition.EnvironmentName != "" {
			spec.Env = []string{definition.EnvironmentName}
		}
		specs = append(specs, spec)
	}
	return specs
}

// ScoringFactorKeys returns the complete set of factors considered by weighted
// compatibility matching. A factor absent from both identities is not scored.
func ScoringFactorKeys() []string {
	keys := []string{RuntimeImageFactor, ModelURIHashFactor}
	for _, definition := range runtimeConfigFactorDefinitions {
		if definition.Scoring {
			keys = append(keys, definition.Key)
		}
	}
	return keys
}

func isDefinedRuntimeConfigFactor(key string) bool {
	_, ok := runtimeConfigDefinition(key)
	return ok
}

// IsStableRuntimeImage reports whether the image reference is immutable.
// Only digest references can safely select a previously built cache.
func IsStableRuntimeImage(image string) bool {
	imageType, err := classifyImageReference(image)
	return err == nil && imageType == imageReferenceDigest
}

// ParseRuntimeConfigFactors resolves the current vLLM profile from command,
// args, and directly supplied environment values.
func ParseRuntimeConfigFactors(command, args []string, environment map[string]string) map[string]string {
	env := make([]corev1.EnvVar, 0, len(environment))
	for name, value := range environment {
		env = append(env, corev1.EnvVar{Name: name, Value: value})
	}
	return extractRuntimeConfigFactors(&corev1.Container{Command: command, Args: args, Env: env})
}

// ExtractRuntimeConfigFactors reads command, args, and literal environment
// values from a runtime container. ValueFrom references are not resolved.
func ExtractRuntimeConfigFactors(container *corev1.Container) map[string]string {
	return extractRuntimeConfigFactors(container)
}

func extractRuntimeConfigFactors(container *corev1.Container) map[string]string {
	raw := runtimeoptions.Resolve(container, vLLMRuntimeOptionSpecs)
	factors := make(map[string]string, len(raw))
	for key, value := range raw {
		definition, ok := runtimeConfigDefinition(key)
		if !ok {
			continue
		}
		if normalized, ok := normalizeFactorValue(definition.ValueType, value); ok {
			factors[key] = normalized
		}
	}
	return factors
}

func runtimeConfigDefinition(key string) (FactorDefinition, bool) {
	for _, definition := range runtimeConfigFactorDefinitions {
		if definition.Key == key {
			return definition, true
		}
	}
	return FactorDefinition{}, false
}

func normalizeFactorValue(valueType factorValueType, value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "\"'")
	if value == "" {
		return "", false
	}
	switch valueType {
	case factorValueInteger:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return "", false
		}
		return strconv.Itoa(parsed), true
	case factorValueBoolean:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			if value == "1" {
				return "true", true
			}
			if value == "0" {
				return "false", true
			}
			return "", false
		}
		return strconv.FormatBool(parsed), true
	case factorValueCommaSeparated:
		parts := strings.Split(value, ",")
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		sort.Strings(parts)
		return strings.Join(parts, ","), true
	case factorValueHash:
		return HashStrings([]string{value}), true
	default:
		return strings.ToLower(value), true
	}
}
