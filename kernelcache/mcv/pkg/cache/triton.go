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
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kserve/kserve/mcv/pkg/accelerator"
	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/constants"

	logging "github.com/sirupsen/logrus"
)

type TritonCache struct {
	path        string
	tmpPath     string
	allMetadata []TritonCacheMetadata
}

func DetectTritonCache(cacheDir string) *TritonCache {
	found := false

	err := filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && (strings.Contains(path, "vendor") || strings.HasPrefix(d.Name(), ".")) {
			return fs.SkipDir
		}

		name := d.Name()
		if strings.HasPrefix(name, "__grp__") && strings.HasSuffix(name, ".json") ||
			strings.HasSuffix(name, ".ttir") ||
			name == "__triton_launcher.so" {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		logging.WithError(err).Warnf("Error walking Triton cache directory: %s", cacheDir)
		return nil
	}

	if found {
		logging.Debugf("Triton cache detected in directory: %s", cacheDir)
		metadata := getTritonMetadata(cacheDir)
		if len(metadata) > 0 {
			return &TritonCache{path: cacheDir, allMetadata: metadata}
		}
	}

	logging.Debugf("No Triton cache found in directory: %s", cacheDir)
	return nil
}

func getTritonMetadata(root string) []TritonCacheMetadata {
	logging.Debugf("getTritonMetadata:%v", root)

	files, err := findAllTritonCacheJSON(root)
	if err != nil {
		logging.WithError(err).Error("Could not enumerate Triton cache JSON files")
		return nil
	}

	var allMetadata []TritonCacheMetadata
	for _, f := range files {
		data, err := GetTritonCacheJSONData(f)
		if err != nil {
			logging.WithFields(logging.Fields{
				"file": f,
				"err":  err,
			}).Error("Failed to load Triton cache JSON")
			continue
		}
		if data == nil {
			continue
		}

		dummyKey, err := ComputeDummyTritonKey(data)
		if err != nil {
			logging.WithFields(logging.Fields{
				"file": f,
				"err":  err,
			}).Error("Failed to compute dummy key for Triton cache JSON")
			continue
		}

		allMetadata = append(allMetadata, TritonCacheMetadata{
			Hash: data.Hash,
			Target: Target{
				Backend:  data.Target.Backend,
				Arch:     ConvertArchToString(data.Target.Arch),
				WarpSize: data.Target.WarpSize,
			},
			PtxVersion: func() int {
				if data.PtxVersion != nil {
					return *data.PtxVersion
				}
				return 0
			}(),
			NumStages: data.NumStages,
			NumWarps:  data.NumWarps,
			Debug:     data.Debug,
			DummyKey:  dummyKey,
		})
	}

	return allMetadata
}

func (t *TritonCache) Name() string {
	return "triton"
}

func (t *TritonCache) ManifestTag() string {
	return constants.MCVTritonManifestDir
}

func (t *TritonCache) CacheTag() string {
	return constants.MCVTritonCacheDir
}

func (t *TritonCache) EntryCount() int {
	return len(t.allMetadata)
}

func (t *TritonCache) CacheSizeBytes() int64 {
	if t.tmpPath != "" {
		totalSize, err := getTotalDirSize(t.tmpPath)
		if err != nil {
			return 0
		}
		return totalSize
	}
	return 0
}

func (t *TritonCache) Summary() string {
	summary, err := BuildTritonSummary(t.allMetadata)
	if err != nil {
		logging.WithError(err).Error("failed to build Triton summary")
		return ""
	}

	jsonData, err := json.Marshal(summary)
	if err != nil {
		logging.WithError(err).Error("failed to marshal Triton summary")
		return ""
	}

	return string(jsonData)
}

func (t *TritonCache) Labels() map[string]string {
	return map[string]string{
		"cache.triton.image/entry-count":      strconv.Itoa(t.EntryCount()),
		"cache.triton.image/summary":          t.Summary(),
		"cache.triton.image/cache-size-bytes": strconv.FormatInt(t.CacheSizeBytes(), 10),
	}
}

func (t *TritonCache) Metadata() []CacheEntry {
	entries := make([]CacheEntry, 0, len(t.allMetadata))
	for _, meta := range t.allMetadata {
		entries = append(entries, meta)
	}
	return entries
}

func GetTritonCacheJSONData(filePath string) (*TritonCacheData, error) {
	content, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var data TritonCacheData
	if err = json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON in file %s: %w", filePath, err)
	}

	if data.Hash == "" {
		logging.WithField("file", filePath).Debug("Skipping file: missing required 'hash' field")
		return nil, nil // gracefully skip
	}

	if logging.IsLevelEnabled(logging.DebugLevel) {
		pretty, _ := json.MarshalIndent(data, "", "  ")
		logging.WithField("json", string(pretty)).Debug("Parsed Triton cache JSON")
	}

	return &data, nil
}

func CompareTritonCacheToGPU(cacheData *TritonCacheData, acc accelerator.Accelerator) error {
	if cacheData == nil {
		return errors.New("cache data is nil")
	}
	if acc == nil {
		return errors.New("no Accelerator detected")
	}

	var devInfo []devices.TritonGPUInfo
	if config.IsGPUEnabled() {
		if gpu := accelerator.GetActiveAcceleratorByType(config.GPU); gpu != nil {
			device := gpu.Device()
			info, err := device.GetAllGPUInfo()
			if err != nil {
				return fmt.Errorf("could not retrieve accelerator info: %w", err)
			}
			devInfo = info
		}
	}

	for _, gpu := range devInfo {
		backendMatch := cacheData.Target.Backend == gpu.Backend
		archMatch := ConvertArchToString(cacheData.Target.Arch) == gpu.Arch
		warpMatch := cacheData.Target.WarpSize == gpu.WarpSize
		ptxMatch := true

		if gpu.Backend == "cuda" && cacheData.PtxVersion != nil {
			ptxMatch = *cacheData.PtxVersion == gpu.PTXVersion
			if !ptxMatch {
				logging.WithFields(logging.Fields{
					"cache_ptx": *cacheData.PtxVersion,
					"gpu_ptx":   gpu.PTXVersion,
				}).Debug("PTX version mismatch")
			}
		}

		if backendMatch && archMatch && warpMatch && ptxMatch {
			return nil // match found
		}

		logging.WithFields(logging.Fields{
			"backend_match": backendMatch,
			"arch_match":    archMatch,
			"warp_match":    warpMatch,
			"ptx_match":     ptxMatch,
			"cache_backend": cacheData.Target.Backend,
			"gpu_backend":   gpu.Backend,
			"cache_arch":    ConvertArchToString(cacheData.Target.Arch),
			"gpu_arch":      gpu.Arch,
			"cache_warp":    cacheData.Target.WarpSize,
			"gpu_warp":      gpu.WarpSize,
		}).Debug("Triton cache entry mismatch")
	}

	return errors.New("no compatible GPU found for Triton cache metadata")
}

// checkFirstKeyHash checks if the first key in the JSON file is "Hash": "hashvalue"
func checkFirstKeyHash(filePath string) (bool, error) {
	logging.Debugf("checkFirstKeyHash:%v", filePath)

	// Read the JSON file
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return false, err
	}

	// Unmarshal into a generic map
	var jsonData TritonCacheData
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return false, err
	}

	// Check if the "hash" field is present and valid
	if jsonData.Hash == "" {
		// DO NOT return an error.
		return false, nil
	}

	return true, nil
}

func findAllTritonCacheJSON(cacheDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			match, err := checkFirstKeyHash(path)
			if err != nil {
				log.Printf("Error checking file %s: %v\n", path, err)
				return nil
			}
			if match {
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no valid Triton cache JSON files found in %s", cacheDir)
	}

	return files, nil
}

func ConvertArchToString(arch any) string {
	switch v := arch.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		logging.Warnf("Unexpected arch type: %T", v)
		return ""
	}
}

// BuildTritonSummary deduplicates kernel targets and produces a compact summary for labeling.
func BuildTritonSummary(metadata []TritonCacheMetadata) (*Summary, error) {
	if len(metadata) == 0 {
		return nil, errors.New("no metadata provided to summarize")
	}

	seen := make(map[string]SummaryTargetInfo)

	for _, entry := range metadata {
		key := fmt.Sprintf("%s-%s-%d", entry.Backend, entry.Arch, entry.WarpSize)
		if _, exists := seen[key]; !exists {
			seen[key] = SummaryTargetInfo{
				Backend:  entry.Backend,
				Arch:     ConvertArchToString(entry.Arch),
				WarpSize: entry.WarpSize,
			}
		}
	}

	targets := make([]SummaryTargetInfo, 0, len(seen))
	for _, v := range seen {
		targets = append(targets, v)
	}

	return &Summary{Targets: targets}, nil
}

func (t *TritonCache) SetTmpPath(path string) {
	if path != "" {
		t.tmpPath = path
	}
}

func ExtractTritonCacheDirectory(r io.Reader) ([]string, error) {
	return extractCacheAndManifestDirectory(
		r,
		constants.MCVTritonCacheDir,
		"io.triton.manifest/",
		constants.ExtractCacheDir,
		constants.ExtractManifestDir,
	)
}
