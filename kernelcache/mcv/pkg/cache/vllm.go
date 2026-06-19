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

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/constants"
)

var hashDirRegex = regexp.MustCompile(`^[a-f0-9]{32}$`) // Adjust the regex as needed

const (
	cacheVLLMImagePrefix     = "cache.vllm.image"
	cacheVLLMImageEntryCount = cacheVLLMImagePrefix + "/entry-count"
	cacheVLLMImageCacheSize  = cacheVLLMImagePrefix + "/cache-size-bytes"
	cacheVLLMImageSummary    = cacheVLLMImagePrefix + "/summary"
	cacheVLLMImageFormat     = cacheVLLMImagePrefix + "/format"

	// Cache format constants
	BinaryCacheFormat     = "binary"
	AOTCompileCacheFormat = "aot_compile"
	TritonCacheFormat     = "triton"
	CUDABackend           = "cuda"
	ROCmBackend           = "rocm"
	HIPBackend            = "hip"
	UnknownBackend        = "UnknownBackend"

	// torchAOTCompileDirName is the extra directory vLLM introduces above
	// the per-model hash dir when VLLM_USE_AOT_COMPILE is enabled.
	torchAOTCompileDirName = "torch_aot_compile"
	// megaAOTSaveFormat marks a BinaryCacheMetadata entry as produced by
	// the mega-AOT flow (single bundled "model" blob per rank dir).
	megaAOTSaveFormat = "mega-aot"
	// modelFileName is the standard filename for AOT and Mega-AOT model artifacts
	modelFileName = "model"
)

// VLLMCache represents a VLLM-style compile cache (e.g., torch_inductor or fxgraph)
type VLLMCache struct {
	rootPath    string
	tmpPath     string
	count       int
	tritonCache *TritonCache
	allMetadata []VLLMCacheMetadata
}

type VLLMCacheMetadata struct {
	VllmHash           string                    `json:"vllmHash"`
	CacheFormat        string                    `json:"cacheFormat"` // "triton", "binary", or "aot_compile"
	TritonCacheEntries []CacheEntry              `json:"triton,omitempty"`
	BinaryCacheEntries []BinaryCacheMetadata     `json:"binary,omitempty"`
	AOTCompileEntries  []AOTCompileCacheMetadata `json:"aot_compile,omitempty"`
}

// DetectVLLMCache walks the given root directory to detect whether VLLM-style cache artifacts exist
// processHashEntry processes a single hash directory entry and returns metadata if cache is found
func processHashEntry(hashDir, entryName string, tc **TritonCache) (*VLLMCacheMetadata, error) {
	// Try to detect binary cache format first (newer format)
	binaryCacheData, binaryErr := detectBinaryCache(hashDir)
	if binaryErr == nil && len(binaryCacheData) > 0 {
		logging.Debugf("Detected binary cache format for hash: %s", entryName)
		return &VLLMCacheMetadata{
			VllmHash:           entryName,
			CacheFormat:        BinaryCacheFormat,
			BinaryCacheEntries: binaryCacheData,
		}, nil
	}

	// Fall back to triton cache format (older format)
	var tritonCachePath string
	err := filepath.WalkDir(hashDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "triton_cache" {
			tritonCachePath = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil || tritonCachePath == "" {
		return nil, errors.New("neither binary cache nor triton cache found")
	}

	// Check if tritonCachePath exists
	if _, err := os.Stat(tritonCachePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("triton cache path does not exist: %s", tritonCachePath)
	}

	logging.Debugf("Inspecting potential Triton cache at: %s", tritonCachePath)
	_tc := DetectTritonCache(tritonCachePath)
	if _tc == nil {
		return nil, errors.New("failed to detect Triton cache")
	}
	*tc = _tc
	return &VLLMCacheMetadata{
		VllmHash:           entryName,
		CacheFormat:        TritonCacheFormat,
		TritonCacheEntries: (*tc).Metadata(),
	}, nil
}

// detectAndProcessAOTCache detects AOT compile cache and returns metadata entries.
// It first tries mega-AOT detection (which stores artifacts in torch_aot_compile/),
// and only falls back to regular AOT compile detection if mega-AOT is not found.
func detectAndProcessAOTCache(torchCompileCachePath string) (metadata []VLLMCacheMetadata, count int) {
	aotCompilePath := filepath.Join(torchCompileCachePath, "torch_aot_compile")
	if _, err := os.Stat(aotCompilePath); err != nil {
		return metadata, count
	}

	// First, try to detect mega-AOT caches (which live in torch_aot_compile/)
	logging.Debugf("Detecting mega-AOT cache at: %s", aotCompilePath)
	megaMetadata, megaCount := detectMegaAOTEntries(aotCompilePath)
	if len(megaMetadata) > 0 {
		logging.Debugf("Detected mega-AOT cache format with %d entries", len(megaMetadata))
		return megaMetadata, megaCount
	}

	// Fall back to regular AOT compile detection if no mega-AOT found
	logging.Debugf("No mega-AOT cache found, trying regular AOT compile cache at: %s", aotCompilePath)
	aotCacheData, aotErr := detectAOTCompileCache(aotCompilePath)
	if aotErr != nil {
		logging.Debugf("No AOT compile cache detected: %v", aotErr)
		return metadata, count
	}
	if len(aotCacheData) == 0 {
		return metadata, count
	}

	logging.Debugf("Detected AOT compile cache format with %d entries", len(aotCacheData))
	// Group AOT cache entries by hash
	aotByHash := make(map[string][]AOTCompileCacheMetadata)
	for _, aotCache := range aotCacheData {
		aotByHash[aotCache.Hash] = append(aotByHash[aotCache.Hash], aotCache)
	}

	// Create metadata entries for each hash
	for hash, entries := range aotByHash {
		vllmMetadata := VLLMCacheMetadata{
			VllmHash:          hash,
			CacheFormat:       AOTCompileCacheFormat,
			AOTCompileEntries: entries,
		}
		logging.Debugf("Adding VLLM AOT compile cache metadata: %+v", vllmMetadata)
		metadata = append(metadata, vllmMetadata)
		count++
	}

	return metadata, count
}

func DetectVLLMCache(cacheDir string) *VLLMCache {
	found := false
	var torchCompileCachePath string
	metadata := []VLLMCacheMetadata{}
	var tc *TritonCache

	err := filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.Contains(path, "vendor") || strings.HasPrefix(d.Name(), ".")) {
			return fs.SkipDir
		}

		name := d.Name()
		if strings.HasSuffix(name, "vllm_compile_cache.py") ||
			strings.Contains(path, "inductor_cache") ||
			strings.Contains(path, "fxgraph") ||
			strings.Contains(path, torchAOTCompileDirName) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		logging.WithError(err).Warnf("Error walking vllm cache directory: %s", cacheDir)
		return nil
	}

	var count int
	if found {
		torchCompileCachePath = filepath.Join(cacheDir, "torch_compile_cache")
		if _, err := os.Stat(torchCompileCachePath); os.IsNotExist(err) {
			logging.Warnf("Torch compile cache path does not exist: %s", torchCompileCachePath)
			return nil
		}
		entries, err := os.ReadDir(torchCompileCachePath)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				// Skip torch_aot_compile directory - it's handled separately
				if entry.Name() == torchAOTCompileDirName {
					logging.Debugf("Skipping %s directory (handled by detectAOTCompileCache)", torchAOTCompileDirName)
					continue
				}

				count++
				hashDir := filepath.Join(torchCompileCachePath, entry.Name())

				// Process hash entry (binary or triton cache)
				vllmMetadata, err := processHashEntry(hashDir, entry.Name(), &tc)
				if err != nil {
					logging.Warnf("Failed to process hash entry %s: %v", entry.Name(), err)
					continue
				}
				logging.Debugf("Adding VLLM cache metadata: %+v", vllmMetadata)
				metadata = append(metadata, *vllmMetadata)
			}

			// Detect and process AOT compile cache
			aotMetadata, aotCount := detectAndProcessAOTCache(torchCompileCachePath)
			metadata = append(metadata, aotMetadata...)
			count += aotCount
		}
	}

	if found {
		return &VLLMCache{
			rootPath:    cacheDir,
			tritonCache: tc,
			count:       count,
			allMetadata: metadata,
		}
	}
	return nil
}

// detectBinaryCache detects binary cache format in a hash directory
// It looks for rank_X_Y directories containing binary cache artifacts
func detectBinaryCache(hashDir string) ([]BinaryCacheMetadata, error) {
	var binaryCaches []BinaryCacheMetadata

	entries, err := os.ReadDir(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read hash directory: %w", err)
	}

	// Look for rank_X_Y directories
	rankDirRegex := regexp.MustCompile(`^rank_\d+_\d+$`)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !rankDirRegex.MatchString(entry.Name()) {
			continue
		}

		rankPath := filepath.Join(hashDir, entry.Name())
		logging.Debugf("Inspecting rank directory: %s", rankPath)

		// Look for prefix directories (e.g., backbone, eagle_head)
		prefixEntries, err := os.ReadDir(rankPath)
		if err != nil {
			logging.Warnf("Failed to read rank directory %s: %v", rankPath, err)
			continue
		}

		for _, prefixEntry := range prefixEntries {
			if !prefixEntry.IsDir() {
				continue
			}

			prefixPath := filepath.Join(rankPath, prefixEntry.Name())
			logging.Debugf("Inspecting prefix directory: %s", prefixPath)

			// Check for binary cache indicators:
			// 1. cache_key_factors.json
			// 2. artifact_compile_range_* files
			cacheKeyPath := filepath.Join(prefixPath, "cache_key_factors.json")
			if _, err := os.Stat(cacheKeyPath); os.IsNotExist(err) {
				logging.Debugf("No cache_key_factors.json in %s, skipping", prefixPath)
				continue
			}

			// Read cache key factors
			var keyFactors CacheKeyFactors
			data, err := os.ReadFile(filepath.Clean(cacheKeyPath))
			if err != nil {
				logging.Warnf("Failed to read cache_key_factors.json: %v", err)
				continue
			}
			if unmarshalErr := json.Unmarshal(data, &keyFactors); unmarshalErr != nil {
				logging.Warnf("Failed to parse cache_key_factors.json: %v", unmarshalErr)
				continue
			}

			// Count and collect artifact files
			var artifacts []string
			artifactRegex := regexp.MustCompile(`^artifact_compile_range_`)
			prefixFiles, err := os.ReadDir(prefixPath)
			if err != nil {
				logging.Warnf("Failed to read prefix directory %s: %v", prefixPath, err)
				continue
			}

			// Detect actual format by inspecting the first artifact
			cacheSaveFormat := BinaryCacheFormat
			foundFirstArtifact := false

			for _, file := range prefixFiles {
				if artifactRegex.MatchString(file.Name()) {
					artifacts = append(artifacts, file.Name())

					// Detect format from the first artifact: directory = unpacked, file = binary
					if !foundFirstArtifact {
						if file.IsDir() {
							cacheSaveFormat = "unpacked"
						} else {
							cacheSaveFormat = "binary"
						}
						foundFirstArtifact = true
					}
				}
			}

			if len(artifacts) == 0 {
				logging.Debugf("No binary artifacts found in %s, skipping", prefixPath)
				continue
			}

			// Extract target device (could be cuda, rocm, tpu, cpu, etc.)
			targetDevice := ""
			if env, ok := keyFactors.Env["VLLM_TARGET_DEVICE"]; ok {
				if device, ok := env.(string); ok {
					targetDevice = device
				}
			}

			binaryCache := BinaryCacheMetadata{
				Rank:            entry.Name(),
				Prefix:          prefixEntry.Name(),
				ArtifactCount:   len(artifacts),
				ArtifactNames:   artifacts,
				CodeHash:        keyFactors.CodeHash,
				ConfigHash:      keyFactors.ConfigHash,
				CompilerHash:    keyFactors.CompilerHash,
				CacheSaveFormat: cacheSaveFormat,
				TargetDevice:    targetDevice,
				Env:             keyFactors.Env,
			}

			logging.Debugf("Found binary cache: %+v", binaryCache)
			binaryCaches = append(binaryCaches, binaryCache)
		}
	}

	if len(binaryCaches) == 0 {
		return nil, errors.New("no binary cache detected")
	}

	return binaryCaches, nil
}

// detectMegaAOTEntries walks torch_aot_compile/ and returns metadata for
// each child hash dir that contains a mega-AOT bundle. The count return
// is the number of hash directories considered (whether or not they
// yielded valid metadata), so the caller can keep its entry count in sync.
func detectMegaAOTEntries(aotDir string) (entries []VLLMCacheMetadata, count int) {
	dirEntries, err := os.ReadDir(aotDir)
	if err != nil {
		logging.Warnf("Failed to read %s: %v", aotDir, err)
		return nil, 0
	}

	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}
		hashDir := filepath.Join(aotDir, entry.Name())
		megaData, megaErr := detectMegaAOTCache(hashDir)
		if megaErr != nil || len(megaData) == 0 {
			logging.Warnf("No mega-AOT artifacts in %s: %v", hashDir, megaErr)
			continue
		}
		logging.Debugf("Detected mega-AOT cache for hash: %s", entry.Name())
		count++
		entries = append(entries, VLLMCacheMetadata{
			VllmHash:           entry.Name(),
			CacheFormat:        BinaryCacheFormat,
			BinaryCacheEntries: megaData,
		})
	}
	return entries, count
}

// detectMegaAOTCache detects the mega-AOT bundle layout in a hash directory.
// The layout places one bundled artifact at {hashDir}/rank_X_Y/model, with
// inductor/triton state as a shared sibling at {hashDir}/inductor_cache/.
// Unlike the per-piecewise binary format, no cache_key_factors.json is
// emitted, so hash/env fields are left empty.
func detectMegaAOTCache(hashDir string) ([]BinaryCacheMetadata, error) {
	entries, err := os.ReadDir(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read hash directory: %w", err)
	}

	var out []BinaryCacheMetadata
	rankDirRegex := regexp.MustCompile(`^rank_\d+_\d+$`)
	for _, entry := range entries {
		if !entry.IsDir() || !rankDirRegex.MatchString(entry.Name()) {
			continue
		}
		modelPath := filepath.Join(hashDir, entry.Name(), modelFileName)
		info, err := os.Stat(modelPath)
		if err != nil || info.IsDir() {
			continue
		}
		out = append(out, BinaryCacheMetadata{
			Rank:            entry.Name(),
			ArtifactCount:   1,
			ArtifactNames:   []string{modelFileName},
			CacheSaveFormat: megaAOTSaveFormat,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("no mega-AOT artifacts detected")
	}
	return out, nil
}

// detectAOTCompileCache detects AOT compile cache format
// These are created when VLLM_USE_AOT_COMPILE=1 and stored at:
// torch_compile_cache/torch_aot_compile/{hash}/rank_{rank}_{dp_rank}/model
func detectAOTCompileCache(aotPath string) ([]AOTCompileCacheMetadata, error) {
	var aotCaches []AOTCompileCacheMetadata

	if _, err := os.Stat(aotPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("AOT compile cache path does not exist: %s", aotPath)
	}

	// Walk the torch_aot_compile directory looking for {hash}/rank_X_Y/model files
	entries, err := os.ReadDir(aotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read AOT compile directory: %w", err)
	}

	for _, hashEntry := range entries {
		if !hashEntry.IsDir() {
			continue
		}

		hashDir := filepath.Join(aotPath, hashEntry.Name())
		logging.Debugf("Inspecting AOT hash directory: %s", hashDir)

		// Look for rank_X_Y directories
		rankEntries, err := os.ReadDir(hashDir)
		if err != nil {
			logging.Warnf("Failed to read AOT hash directory %s: %v", hashDir, err)
			continue
		}

		rankDirRegex := regexp.MustCompile(`^rank_\d+_\d+$`)
		for _, rankEntry := range rankEntries {
			if !rankEntry.IsDir() {
				continue
			}
			if !rankDirRegex.MatchString(rankEntry.Name()) {
				continue
			}

			// Check for model file
			modelPath := filepath.Join(hashDir, rankEntry.Name(), modelFileName)
			stat, err := os.Stat(modelPath)
			if err != nil {
				logging.Debugf("No model file found at %s: %v", modelPath, err)
				continue
			}

			aotCache := AOTCompileCacheMetadata{
				Hash:      hashEntry.Name(),
				Rank:      rankEntry.Name(),
				ModelFile: modelFileName,
				FileSize:  stat.Size(),
			}

			logging.Debugf("Found AOT compile cache: %+v", aotCache)
			aotCaches = append(aotCaches, aotCache)
		}
	}

	if len(aotCaches) == 0 {
		return nil, errors.New("no AOT compile cache detected")
	}

	return aotCaches, nil
}

func (v *VLLMCache) Name() string { return constants.VLLM }

func (v *VLLMCache) EntryCount() int {
	return v.count
}

func (v *VLLMCache) CacheSizeBytes() int64 {
	size, _ := getTotalDirSize(v.rootPath)
	return size
}

func (v *VLLMCache) Summary() string {
	// The summary should include the target hardware summary from the Triton cache
	// as well as any relevant VLLM-specific details (if applicable)
	var summary *Summary
	var err error

	// Check if we have binary cache metadata
	hasBinaryCache := false
	for _, meta := range v.allMetadata {
		if meta.CacheFormat == BinaryCacheFormat && len(meta.BinaryCacheEntries) > 0 {
			hasBinaryCache = true
			break
		}
	}

	if hasBinaryCache {
		logging.Debugf("Building VLLM summary from binary cache metadata")
		summary, err = buildBinaryCacheSummary(v.allMetadata)
		if err != nil {
			logging.WithError(err).Error("failed to build binary cache summary")
			return ""
		}
	} else if v.tritonCache != nil && len(v.tritonCache.allMetadata) > 0 {
		logging.Debugf("Building VLLM summary from Triton metadata")
		tempSummary, tempErr := BuildTritonSummary(v.tritonCache.allMetadata)
		if tempErr != nil {
			logging.WithError(tempErr).Error("failed to build vLLM summary")
			return ""
		}
		summary = tempSummary
	}

	jsonData, err := json.Marshal(summary)
	if err != nil {
		logging.WithError(err).Error("failed to marshal vLLM summary")
		return ""
	}

	logging.Debugf("VLLM Summary: %s", string(jsonData))

	return string(jsonData)
}

// detectActualGPUInfo detects the actual GPU architecture from the current system
// This is called during cache image creation to detect the real hardware,
// regardless of what VLLM_TARGET_DEVICE says in the cache metadata.
// Returns backend, arch, warpSize, and ptxVersion
func detectActualGPUInfo() (backend, arch string, warpSize, ptxVersion int) {
	// Initialize config if not already done
	if !config.IsInitialized() {
		if _, err := config.Initialize(config.ConfDir); err != nil {
			logging.WithError(err).Debug("Failed to initialize config for GPU detection")
			return UnknownBackend, UnknownBackend, 0, 0
		}
	}

	// Get device registry
	registry := devices.GetRegistry()
	if registry == nil {
		logging.Debug("Failed to get device registry")
		return UnknownBackend, UnknownBackend, 0, 0
	}

	// Try to start GPU device - this will auto-detect CUDA/ROCm
	device := devices.Startup(config.GPU, registry)
	if device == nil {
		logging.Debug("No GPU detected on system")
		return UnknownBackend, UnknownBackend, 0, 0
	}

	// Initialize the device to ensure GPU info is populated
	// This is important when the device was restored from cache
	if err := device.Init(); err != nil {
		logging.WithError(err).Debug("Failed to initialize device")
		return UnknownBackend, UnknownBackend, 0, 0
	}

	// Get GPU info for the first GPU (index 0)
	gpuInfo, err := device.GetGPUInfo(0)
	if err != nil {
		logging.WithError(err).Debug("Failed to get GPU info from device")
		return UnknownBackend, UnknownBackend, 0, 0
	}

	// Determine backend and warp size from the detected GPU
	detectedBackend := gpuInfo.Backend
	if detectedBackend == "" {
		// Fallback: try to infer from device type
		switch device.DevType() {
		case devices.NVML:
			detectedBackend = CUDABackend
		case devices.ROCM, devices.AMD:
			detectedBackend = ROCmBackend
		default:
			detectedBackend = UnknownBackend
		}
	}

	detectedWarpSize := gpuInfo.WarpSize
	if detectedWarpSize == 0 {
		// Fallback to defaults
		switch detectedBackend {
		case CUDABackend:
			detectedWarpSize = 32
		case ROCmBackend, HIPBackend:
			detectedWarpSize = 64
		}
	}

	logging.Infof("Detected GPU: backend=%s, arch=%s, warpSize=%d, PTX=%d",
		detectedBackend, gpuInfo.Arch, detectedWarpSize, gpuInfo.PTXVersion)

	return detectedBackend, gpuInfo.Arch, detectedWarpSize, gpuInfo.PTXVersion
}

// buildBinaryCacheSummary builds a summary from binary cache metadata
func buildBinaryCacheSummary(metadata []VLLMCacheMetadata) (*Summary, error) {
	targetMap := make(map[string]SummaryTargetInfo)

	// Detect actual GPU from the system once (not per metadata entry)
	// NOTE: We detect the actual system GPU rather than trusting VLLM_TARGET_DEVICE
	// because caches may be copied from other systems
	detectedBackend, detectedArch, detectedWarpSize, detectedPTX := detectActualGPUInfo()

	for _, meta := range metadata {
		if meta.CacheFormat != BinaryCacheFormat {
			continue
		}

		for i := range meta.BinaryCacheEntries {
			binaryCache := &meta.BinaryCacheEntries[i]

			// Use detected GPU info from actual system
			backend := detectedBackend
			arch := detectedArch
			warpSize := detectedWarpSize
			ptxVersion := detectedPTX

			// Extract toolkit versions from cache environment for reference
			cudaVersion := ""
			rocmVersion := ""

			// Handle special cases where no GPU is detected
			if backend == UnknownBackend {
				logging.Warn("Could not detect GPU on system, using cache metadata as fallback")
				// Fallback to cache metadata if GPU detection failed
				backend = binaryCache.TargetDevice
				if backend == "" {
					backend = CUDABackend // Default if not specified
				}

				// Also try to extract arch from cache metadata environment
				if archEnv, ok := binaryCache.Env["VLLM_PAGED_ATTN_ARCH"]; ok {
					if archStr, ok := archEnv.(string); ok && archStr != "" {
						arch = archStr
						logging.Debugf("Using arch from cache env VLLM_PAGED_ATTN_ARCH: %s", arch)
					}
				}

				// If arch is still unknown, set a default based on backend
				if arch == UnknownBackend || arch == "" {
					switch backend {
					case CUDABackend:
						arch = "75" // Default to sm_75 (Turing/T4) for CUDA
						logging.Debugf("Using default CUDA arch: %s", arch)
					case ROCmBackend, HIPBackend:
						arch = "gfx908" // Default to gfx908 (MI100) for AMD
						logging.Debugf("Using default ROCm arch: %s", arch)
					default:
						arch = "unknown"
					}
				}

				// Set default warp sizes
				switch backend {
				case ROCmBackend, HIPBackend:
					warpSize = 64
				case CUDABackend:
					warpSize = 32
				case "tpu":
					warpSize = 128
				case "cpu":
					warpSize = 1
				}
			}

			// For vLLM binary cache, CUDA uses sm_ prefix (e.g., sm_75)
			// AMD/ROCm already has gfx prefix (e.g., gfx1151)
			// Apply sm_ prefix AFTER fallback logic so arch is properly set
			if backend == CUDABackend && !strings.HasPrefix(arch, "sm_") {
				arch = "sm_" + arch
			}

			// Extract toolkit version info from environment
			switch backend {
			case CUDABackend:
				if cudaVer, ok := binaryCache.Env["VLLM_MAIN_CUDA_VERSION"]; ok {
					if ver, ok := cudaVer.(string); ok {
						cudaVersion = ver
						logging.Debugf("CUDA toolkit version from cache: %s", cudaVersion)
					}
				}
			case ROCmBackend, HIPBackend:
				if rocmVer, ok := binaryCache.Env["ROCM_VERSION"]; ok {
					if ver, ok := rocmVer.(string); ok {
						rocmVersion = ver
						logging.Debugf("ROCm version from cache: %s", rocmVersion)
					}
				}
			}

			// Create unique key including version info for better cache matching
			key := fmt.Sprintf("%s-%s-%d-%s-%s", backend, arch, warpSize, cudaVersion, rocmVersion)
			if _, exists := targetMap[key]; !exists {
				targetInfo := SummaryTargetInfo{
					Backend:  backend,
					Arch:     arch,
					WarpSize: warpSize,
				}
				// Add version info if available
				if ptxVersion > 0 {
					targetInfo.PTXVersion = ptxVersion
				}
				if cudaVersion != "" {
					targetInfo.CUDAVersion = cudaVersion
				}
				targetMap[key] = targetInfo
			}
		}
	}

	targets := make([]SummaryTargetInfo, 0, len(targetMap))
	for _, target := range targetMap {
		targets = append(targets, target)
	}

	if len(targets) == 0 {
		return nil, errors.New("no targets found in binary cache metadata")
	}

	return &Summary{Targets: targets}, nil
}

func (v *VLLMCache) Labels() map[string]string {
	// Determine the cache format(s) from metadata
	// Collect all unique formats present
	formatSet := make(map[string]bool)
	for _, meta := range v.allMetadata {
		formatSet[meta.CacheFormat] = true
	}

	// Build comma-separated list of formats, prioritizing: aot_compile, binary, triton
	var formats []string
	if formatSet[AOTCompileCacheFormat] {
		formats = append(formats, AOTCompileCacheFormat)
	}
	if formatSet[BinaryCacheFormat] {
		formats = append(formats, BinaryCacheFormat)
	}
	if formatSet[TritonCacheFormat] {
		formats = append(formats, TritonCacheFormat)
	}

	// Default to "unpacked" if no recognized formats found
	cacheFormat := "unpacked"
	if len(formats) > 0 {
		cacheFormat = strings.Join(formats, ",")
	}

	return map[string]string{
		cacheVLLMImageEntryCount: strconv.Itoa(v.EntryCount()),
		cacheVLLMImageCacheSize:  strconv.FormatInt(v.CacheSizeBytes(), 10),
		cacheVLLMImageSummary:    v.Summary(),
		cacheVLLMImageFormat:     cacheFormat,
	}
}

func (v *VLLMCache) Metadata() []CacheEntry {
	entries := make([]CacheEntry, 0, len(v.allMetadata))
	for _, meta := range v.allMetadata {
		entries = append(entries, meta)
	}
	return entries
}

func (v *VLLMCache) ManifestTag() string {
	return "./" + constants.MCVVLLMManifestDir
}

func (v *VLLMCache) CacheTag() string {
	return "./" + constants.MCVVLLMCacheDir
}

func (v *VLLMCache) SetTmpPath(path string) {
	if path != "" {
		v.tmpPath = path
	}
}

// Extracts the vllm cache and manifest in a given reader for tar.gz.
// This is only used for *compat* variant.
func ExtractVLLMCacheDirectory(r io.Reader) ([]string, error) {
	return extractCacheAndManifestDirectory(
		r,
		constants.MCVVLLMCacheDir,
		"io.vllm.manifest/",
		constants.ExtractCacheDir,
		constants.ExtractManifestDir,
	)
}
