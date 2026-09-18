// Package cacheplan is the single source of truth for how a kernel cache is
// interpreted on both sides of the Kernel Manager (KM) flow:
//
//   - Producer (capture): given a framework/cache type and the directory the
//     framework should write to, ProducerEnv returns the environment variables
//     that make the framework populate that directory. Some drivers (notably the
//     Habana/Synapse recipe cache) only write to disk when their env var is set,
//     so the capture side must inject this up front.
//
//   - Consumer (serving): given the OCI image labels MCV stamped at build time,
//     Derive returns a typed CachePlan describing the env vars to set, the bare
//     directory to mount at, the payload subpath/prefix within the image, and
//     whether the cache needs a writable copy.
//
// The package is pure: it performs no I/O and depends on no Kubernetes types.
// It takes already-fetched labels and returns plain data, so it can be shared
// verbatim between MCV (which authors the labels) and KServe (which actuates the
// resulting plan onto a pod spec). The OCI label schema remains the persisted
// compatibility contract between the two repositories.
package cacheplan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/redhat-et/GKM/mcv/pkg/constants"
)

// Generic KServe Kernel Manager labels stamped by each cache class's Labels()
// method. These are the compatibility contract between MCV and KServe.
const (
	LabelCacheType         = constants.KMPrefix + "/cache-type"
	LabelCacheRootEnv      = constants.KMPrefix + "/cache-root-env"
	LabelCacheMountSubpath = constants.KMPrefix + "/cache-mount-subpath"
	LabelCacheHash         = constants.KMPrefix + "/cache-hash"
	LabelFramework         = constants.KMPrefix + "/framework"
)

// Per-class summary labels, used to infer the cache type for older images that
// predate the io.kserve.km/cache-type label.
const (
	summaryLabelTriton = "cache.triton.image/summary"
	summaryLabelVLLM   = "cache.vllm.image/summary"
	summaryLabelHabana = "cache.habana.image/summary"
)

// Habana/Synapse recipe cache tunables encoded into PT_HPU_RECIPE_CACHE_CONFIG
// (format: "<dir>,<delete>,<size_mb>"). delete=false means populate-if-empty /
// reuse, which lets a pre-seeded cache be reused instead of rebuilt and cuts
// warmup time substantially.
const (
	habanaRecipeDelete = "false"

	// DefaultHabanaRecipeCacheSizeMB is the max disk the Synapse driver will use
	// for the recipe cache (8 GB). Callers that read this from KernelCacheCapture.Spec
	// should use that value instead and fall back to this default when unset.
	DefaultHabanaRecipeCacheSizeMB = 8192

	// EnvHabanaRecipeCacheSizeMB is the env var the KServe sidecar injector sets on
	// the MCV capture container so that habana.Labels() stamps the same size into the
	// OCI image label as was set in the predictor's PT_HPU_RECIPE_CACHE_CONFIG.
	EnvHabanaRecipeCacheSizeMB = "HABANA_RECIPE_CACHE_SIZE_MB"
)

// EnvVar is a name/value environment variable pair. It deliberately mirrors the
// shape of corev1.EnvVar without depending on the Kubernetes API.
type EnvVar struct {
	Name  string
	Value string
}

// CachePlan is the typed interpretation of a kernel cache image, derived from
// its OCI labels. KServe consumes these fields to build the serving pod spec.
type CachePlan struct {
	// CacheType is the canonical cache type identifier
	// (constants.CacheTypeVLLMTorchCompile, constants.CacheTypeHabanaRecipe).
	CacheType string

	// Env is the set of environment variables to set on the serving container
	// verbatim (e.g. PT_HPU_RECIPE_CACHE_CONFIG=/dir,false,8192).
	Env []EnvVar

	// MountDir is the bare directory the cache should be mounted at, with any
	// tunable suffix (such as Habana's ",false,8192") stripped.
	MountDir string

	// SubPath is the subpath within the OCI payload to expose.
	SubPath string

	// PayloadPrefix is the top-level directory MCV writes the cache payload
	// under inside the OCI image (e.g. "io.vllm.cache", "io.habana.cache").
	PayloadPrefix string

	// RequiresWritable is true when the framework needs write access to the
	// cache directory, so a read-only mount alone is insufficient and the
	// consumer must provide a writable copy (e.g. Habana's flat recipe dir).
	RequiresWritable bool
}

// UnsupportedCacheTypeError is returned when the labels describe a cache type
// this version of cacheplan does not know how to interpret (e.g. an image built
// by a newer MCV). Consumers should treat it as a signal to fall back to legacy
// handling rather than a hard failure.
type UnsupportedCacheTypeError struct {
	CacheType string
}

func (e *UnsupportedCacheTypeError) Error() string {
	if e.CacheType == "" {
		return "cacheplan: unsupported or unknown cache type"
	}
	return fmt.Sprintf("cacheplan: unsupported cache type %q", e.CacheType)
}

// IsUnsupportedCacheType reports whether err is (or wraps) an
// UnsupportedCacheTypeError.
func IsUnsupportedCacheType(err error) bool {
	var e *UnsupportedCacheTypeError
	return errors.As(err, &e)
}

// Derive decodes OCI image labels into a typed CachePlan. It dispatches on the
// io.kserve.km/cache-type label, falling back to the per-class summary labels
// for older images. It returns an *UnsupportedCacheTypeError for cache types
// with no serving plan (e.g. bare Triton, or types added by a newer MCV).
func Derive(labels map[string]string) (CachePlan, error) {
	if len(labels) == 0 {
		return CachePlan{}, fmt.Errorf("cacheplan: no labels provided")
	}

	switch detectCacheType(labels) {
	case constants.CacheTypeVLLMTorchCompile:
		return deriveVLLM(labels)
	case constants.CacheTypeHabanaRecipe:
		return deriveHabana(labels)
	case constants.Triton:
		// Bare Triton images carry no mounting metadata; there is no serving
		// plan for them today.
		return CachePlan{}, &UnsupportedCacheTypeError{CacheType: constants.Triton}
	default:
		return CachePlan{}, &UnsupportedCacheTypeError{CacheType: labels[LabelCacheType]}
	}
}

// ProducerEnv returns the environment variables that make the given framework
// write its kernel cache to path. This is the capture-side counterpart of
// Derive: the capture pod has no labels to read (nothing has been built yet), so
// the env must be synthesized from the framework/cache type, target path, and
// (for Habana) the maximum cache size in MB.
//
// For non-Habana cache types the sizeMB parameter is ignored.
// Pass DefaultHabanaRecipeCacheSizeMB when no explicit size is configured.
func ProducerEnv(cacheType, path string, sizeMB int) ([]EnvVar, error) {
	if path == "" {
		return nil, fmt.Errorf("cacheplan: empty cache path")
	}

	switch normalizeCacheType(cacheType) {
	case constants.CacheTypeHabanaRecipe:
		return []EnvVar{{
			Name:  constants.HabanaRecipeCacheEnv,
			Value: HabanaRecipeEnvValue(path, sizeMB),
		}}, nil
	case constants.CacheTypeVLLMTorchCompile:
		return []EnvVar{{
			Name:  constants.VLLMCacheRoot,
			Value: path,
		}}, nil
	default:
		return nil, &UnsupportedCacheTypeError{CacheType: cacheType}
	}
}

// RootEnvLabel returns the "NAME=VALUE" string for the io.kserve.km/cache-root-env
// label of a producer cache. It is used by the cache classes' Labels() encoders
// so that the label MCV stamps and the env cacheplan derives stay identical by
// construction.
//
// For Habana caches, sizeMB is the max cache size in MB (see DefaultHabanaRecipeCacheSizeMB).
// For other cache types the sizeMB parameter is ignored.
func RootEnvLabel(cacheType, path string, sizeMB int) (string, error) {
	env, err := ProducerEnv(cacheType, path, sizeMB)
	if err != nil {
		return "", err
	}
	return env[0].Name + "=" + env[0].Value, nil
}

// HabanaRecipeEnvValue formats the PT_HPU_RECIPE_CACHE_CONFIG value
// ("<dir>,false,<size_mb>") for the given cache directory and size limit.
func HabanaRecipeEnvValue(path string, sizeMB int) string {
	return fmt.Sprintf("%s,%s,%d", path, habanaRecipeDelete, sizeMB)
}

// detectCacheType returns the canonical cache type identifier for the labels,
// preferring the explicit io.kserve.km/cache-type label and falling back to the
// per-class summary labels.
func detectCacheType(labels map[string]string) string {
	if t := normalizeCacheType(labels[LabelCacheType]); t != "" {
		return t
	}

	switch {
	case has(labels, summaryLabelHabana):
		return constants.CacheTypeHabanaRecipe
	case has(labels, summaryLabelVLLM):
		return constants.CacheTypeVLLMTorchCompile
	case has(labels, summaryLabelTriton):
		return constants.Triton
	}
	return ""
}

func deriveVLLM(labels map[string]string) (CachePlan, error) {
	env, mountDir, err := parseRootEnv(labels[LabelCacheRootEnv])
	if err != nil {
		return CachePlan{}, err
	}
	if env.Name != constants.VLLMCacheRoot {
		return CachePlan{}, fmt.Errorf("cacheplan: unexpected env name %q in vLLM cache-root-env (want %s)",
			env.Name, constants.VLLMCacheRoot)
	}
	if mountDir == "" {
		return CachePlan{}, fmt.Errorf("cacheplan: empty mount directory in vLLM cache-root-env")
	}
	return CachePlan{
		CacheType:        constants.CacheTypeVLLMTorchCompile,
		Env:              []EnvVar{env},
		MountDir:         mountDir,
		SubPath:          labels[LabelCacheMountSubpath],
		PayloadPrefix:    constants.MCVVLLMCacheDir,
		RequiresWritable: false,
	}, nil
}

func deriveHabana(labels map[string]string) (CachePlan, error) {
	env, mountDir, err := parseRootEnv(labels[LabelCacheRootEnv])
	if err != nil {
		return CachePlan{}, err
	}
	if env.Name != constants.HabanaRecipeCacheEnv {
		return CachePlan{}, fmt.Errorf("cacheplan: unexpected env name %q in Habana cache-root-env (want %s)",
			env.Name, constants.HabanaRecipeCacheEnv)
	}
	if mountDir == "" {
		return CachePlan{}, fmt.Errorf("cacheplan: empty mount directory in Habana cache-root-env")
	}
	subPath := labels[LabelCacheMountSubpath]
	if subPath == "" {
		subPath = "."
	}
	return CachePlan{
		CacheType:        constants.CacheTypeHabanaRecipe,
		Env:              []EnvVar{env},
		MountDir:         mountDir,
		SubPath:          subPath,
		PayloadPrefix:    constants.MCVHabanaCacheDir,
		RequiresWritable: true,
	}, nil
}

// parseRootEnv splits a "NAME=VALUE" cache-root-env label into an EnvVar and the
// bare mount directory. The mount directory is the value up to the first comma,
// so tunable suffixes (Habana's ",false,8192") are stripped while plain values
// (vLLM's "/home/kserve/.cache/vllm") pass through unchanged.
func parseRootEnv(rootEnv string) (EnvVar, string, error) {
	if rootEnv == "" {
		return EnvVar{}, "", fmt.Errorf("cacheplan: missing %s label", LabelCacheRootEnv)
	}
	name, value, ok := strings.Cut(rootEnv, "=")
	if !ok || name == "" {
		return EnvVar{}, "", fmt.Errorf("cacheplan: invalid cache-root-env %q: expected NAME=VALUE", rootEnv)
	}

	mountDir := value
	if i := strings.IndexByte(value, ','); i >= 0 {
		mountDir = value[:i]
	}
	return EnvVar{Name: name, Value: value}, mountDir, nil
}

// normalizeCacheType maps the various framework/cache-type aliases callers may
// use onto the canonical cache type identifiers. Returns "" for the empty
// string and passes unknown values through unchanged.
func normalizeCacheType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return ""
	case constants.VLLM, constants.CacheTypeVLLMTorchCompile:
		return constants.CacheTypeVLLMTorchCompile
	case constants.Habana, "gaudi", constants.CacheTypeHabanaRecipe:
		return constants.CacheTypeHabanaRecipe
	case constants.Triton:
		return constants.Triton
	default:
		return t
	}
}

func has(labels map[string]string, key string) bool {
	_, ok := labels[key]
	return ok
}
