package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/redhat-et/GKM/mcv/pkg/constants"
	logging "github.com/sirupsen/logrus"
)

const (
	cacheHabanaImagePrefix     = "cache.habana.image"
	cacheHabanaImageEntryCount = cacheHabanaImagePrefix + "/entry-count"
	cacheHabanaImageCacheSize  = cacheHabanaImagePrefix + "/cache-size-bytes"
	cacheHabanaImageSummary    = cacheHabanaImagePrefix + "/summary"

	habanaBackend = "hpu"
)

// recipeFileRegex matches Habana recipe cache filenames:
//
//	{graph_hash}_{device_fingerprint}_syn{synapse_version}.recipe
//	{graph_hash}_{device_fingerprint}_syn{synapse_version}.metadata
var recipeFileRegex = regexp.MustCompile(
	`^(\d+)_([a-f0-9]+)_syn([\d.]+[a-f0-9]*)\.recipe$`,
)

// HabanaCache represents a Habana Synapse recipe cache directory.
type HabanaCache struct {
	rootPath    string
	tmpPath     string
	allMetadata []HabanaRecipeMetadata
}

// HabanaRecipeMetadata holds parsed info from a single recipe file pair.
type HabanaRecipeMetadata struct {
	GraphHash      string `json:"graphHash"`
	DeviceID       string `json:"deviceId"`
	SynapseVersion string `json:"synapseVersion"`
	RecipeSize     int64  `json:"recipeSize"`
	MetadataSize   int64  `json:"metadataSize"`
}

// DetectHabanaCache scans cacheDir for Habana recipe files.
// Returns nil if none are found.
func DetectHabanaCache(cacheDir string) *HabanaCache {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		logging.WithError(err).Debugf("Cannot read directory: %s", cacheDir)
		return nil
	}

	var metadata []HabanaRecipeMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := recipeFileRegex.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}

		recipeInfo, err := entry.Info()
		if err != nil {
			logging.WithError(err).Warnf("Failed to stat recipe file: %s", entry.Name())
			continue
		}

		// Look for the matching .metadata file
		metaName := strings.TrimSuffix(entry.Name(), ".recipe") + ".metadata"
		var metaSize int64
		if metaInfo, err := os.Stat(filepath.Join(cacheDir, metaName)); err == nil {
			metaSize = metaInfo.Size()
		}

		metadata = append(metadata, HabanaRecipeMetadata{
			GraphHash:      m[1],
			DeviceID:       m[2],
			SynapseVersion: m[3],
			RecipeSize:     recipeInfo.Size(),
			MetadataSize:   metaSize,
		})
	}

	if len(metadata) == 0 {
		logging.Debugf("No Habana recipe cache found in: %s", cacheDir)
		return nil
	}

	logging.Infof("Detected Habana recipe cache: %d recipes in %s", len(metadata), cacheDir)
	return &HabanaCache{
		rootPath:    cacheDir,
		allMetadata: metadata,
	}
}

func (h *HabanaCache) Name() string { return constants.Habana }

func (h *HabanaCache) EntryCount() int { return len(h.allMetadata) }

func (h *HabanaCache) CacheSizeBytes() int64 {
	dir := h.rootPath
	if h.tmpPath != "" {
		dir = h.tmpPath
	}
	size, _ := getTotalDirSize(dir)
	return size
}

func (h *HabanaCache) Summary() string {
	summary, err := buildHabanaSummary(h.allMetadata)
	if err != nil {
		logging.WithError(err).Error("failed to build Habana summary")
		return ""
	}
	data, err := json.Marshal(summary)
	if err != nil {
		logging.WithError(err).Error("failed to marshal Habana summary")
		return ""
	}
	return string(data)
}

func (h *HabanaCache) Metadata() []CacheEntry {
	entries := make([]CacheEntry, 0, len(h.allMetadata))
	for _, m := range h.allMetadata {
		entries = append(entries, m)
	}
	return entries
}

func (h *HabanaCache) Labels() map[string]string {
	labels := map[string]string{
		cacheHabanaImageEntryCount: strconv.Itoa(h.EntryCount()),
		cacheHabanaImageCacheSize:  strconv.FormatInt(h.CacheSizeBytes(), 10),
		cacheHabanaImageSummary:    h.Summary(),

		constants.KMPrefix + "/framework":           constants.VLLM,
		constants.KMPrefix + "/cache-type":          constants.CacheTypeHabanaRecipe,
		constants.KMPrefix + "/cache-root-env":      constants.HabanaRecipeCacheEnv,
		constants.KMPrefix + "/cache-mount-subpath": ".",
	}
	return labels
}

func (h *HabanaCache) ManifestTag() string {
	return fmt.Sprintf("./%s", constants.MCVHabanaManifestDir)
}

func (h *HabanaCache) CacheTag() string {
	return fmt.Sprintf("./%s", constants.MCVHabanaCacheDir)
}

func (h *HabanaCache) SetTmpPath(path string) {
	if path != "" {
		h.tmpPath = path
	}
}

func buildHabanaSummary(metadata []HabanaRecipeMetadata) (*Summary, error) {
	if len(metadata) == 0 {
		return nil, fmt.Errorf("no Habana metadata to summarize")
	}

	// Deduplicate by device fingerprint — each unique device ID
	// maps to an architecture via the Gaudi device layer (gaudi2, gaudi3).
	// We don't know the arch from recipe filenames alone, so we leave
	// arch empty here; the preflight check matches on backend only.
	seen := make(map[string]bool)
	var targets []SummaryTargetInfo
	for _, m := range metadata {
		if seen[m.DeviceID] {
			continue
		}
		seen[m.DeviceID] = true
		targets = append(targets, SummaryTargetInfo{
			Backend:  habanaBackend,
			WarpSize: 0,
		})
	}
	return &Summary{Targets: targets}, nil
}

func ExtractHabanaCacheDirectory(r io.Reader) (dirs []string, bytesWritten int64, err error) {
	return extractCacheAndManifestDirectory(
		r,
		constants.MCVHabanaCacheDir,
		"io.habana.manifest/",
		constants.ExtractCacheDir,
		constants.ExtractManifestDir,
	)
}
