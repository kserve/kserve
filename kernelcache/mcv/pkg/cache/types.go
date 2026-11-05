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

package cache

type SummaryTargetInfo struct {
	Backend     string `json:"backend"`
	Arch        string `json:"arch"`
	WarpSize    int    `json:"warp_size"`
	PTXVersion  int    `json:"ptx_version,omitempty"`  // CUDA PTX version (for CUDA backend)
	CUDAVersion string `json:"cuda_version,omitempty"` // CUDA toolkit version (e.g., "12.9")
}

type Summary struct {
	Targets []SummaryTargetInfo `json:"targets"`
}

type TritonCacheData struct {
	Hash                      string     `json:"hash"`
	Target                    Target     `json:"target"`
	Name                      string     `json:"name"`
	NumWarps                  int        `json:"num_warps"`
	NumCtas                   int        `json:"num_ctas,omitempty"`
	NumStages                 int        `json:"num_stages"`
	ClusterDims               []int      `json:"cluster_dims"`
	EnableFpFusion            bool       `json:"enable_fp_fusion"`
	SupportedFp8Dtypes        []string   `json:"supported_fp8_dtypes,omitempty"`
	DeprecatedFp8Dtypes       []string   `json:"deprecated_fp8_dtypes,omitempty"`
	DefaultDotInputPrecision  string     `json:"default_dot_input_precision"`
	AllowedDotInputPrecisions []string   `json:"allowed_dot_input_precisions"`
	MaxNumImpreciseAccDefault int        `json:"max_num_imprecise_acc_default"`
	ExternLibs                [][]string `json:"extern_libs,omitempty"`
	Debug                     bool       `json:"debug"`
	BackendName               string     `json:"backend_name"`
	SanitizeOverflow          bool       `json:"sanitize_overflow"`
	Shared                    int        `json:"shared"`
	Arch                      string     `json:"arch"`
	WarpSize                  int        `json:"warp_size"`

	// Optional/Backend-specific fields
	PtxVersion              *int    `json:"ptx_version,omitempty"`  // CUDA-only
	MaxNReg                 *int    `json:"maxnreg,omitempty"`      // CUDA
	WavesPerEU              *int    `json:"waves_per_eu,omitempty"` // ROCm-only
	LaunchCooperativeGrid   *bool   `json:"launch_cooperative_grid,omitempty"`
	MatrixInstrNonKDim      *int    `json:"matrix_instr_nonkdim,omitempty"`
	KPack                   *int    `json:"kpack,omitempty"`
	AllowFlushDenorm        *bool   `json:"allow_flush_denorm,omitempty"`
	InstructionSchedVariant *string `json:"instruction_sched_variant,omitempty"`
}

type TritonCacheMetadata struct {
	Hash       string `json:"hash"`
	DummyKey   string `json:"dummy_key,omitempty"`
	PtxVersion int    `json:"ptx_version,omitempty"`
	NumStages  int    `json:"num_stages,omitempty"`
	NumWarps   int    `json:"num_warps,omitempty"`
	Debug      bool   `json:"debug,omitempty"`
	Target
}

type Target struct {
	Backend  string `json:"backend"`
	Arch     any    `json:"arch"`
	WarpSize int    `json:"warp_size"`
}

type TritonManifest struct {
	Triton []TritonCacheMetadata `json:"triton"`
}

type VLLMManifest struct {
	VLLM []VLLMCacheMetadata `json:"vllm"`
}

// BinaryCacheMetadata represents metadata for binary cache artifacts
type BinaryCacheMetadata struct {
	Rank            string                 `json:"rank"`
	Prefix          string                 `json:"prefix"`
	ArtifactCount   int                    `json:"artifact_count"`
	ArtifactNames   []string               `json:"artifact_names,omitempty"`
	CodeHash        string                 `json:"code_hash,omitempty"`
	ConfigHash      string                 `json:"config_hash,omitempty"`
	CompilerHash    string                 `json:"compiler_hash,omitempty"`
	CacheSaveFormat string                 `json:"cache_save_format,omitempty"`
	TargetDevice    string                 `json:"target_device,omitempty"`
	Env             map[string]interface{} `json:"env,omitempty"`
}

// CacheKeyFactors represents the cache_key_factors.json structure
type CacheKeyFactors struct {
	CodeHash     string                 `json:"code_hash"`
	CompilerHash string                 `json:"compiler_hash"`
	ConfigHash   string                 `json:"config_hash"`
	Env          map[string]interface{} `json:"env"`
}

// AOTCompileCacheMetadata represents metadata for AOT compile cache artifacts
// These are created when VLLM_USE_AOT_COMPILE=1 and stored at:
// torch_compile_cache/torch_aot_compile/{hash}/rank_{rank}_{dp_rank}/model
type AOTCompileCacheMetadata struct {
	Hash      string `json:"hash"`       // Full hash from directory name
	Rank      string `json:"rank"`       // rank_X_Y format
	ModelFile string `json:"model_file"` // Always "model"
	FileSize  int64  `json:"file_size"`  // Size of model file in bytes
}
