package cacheplan

import (
	"testing"

	"github.com/redhat-et/GKM/mcv/pkg/constants"
)

const (
	testVLLMCacheDir        = constants.KServeHome + "/" + constants.VLLMCache   // testVLLMCacheDir
	testHabanaCacheDir      = constants.KServeHome + "/" + constants.HabanaCache // testHabanaCacheDir
	testHabanaCacheEnvValue = testHabanaCacheDir + ",false,8192"
)

func TestDeriveVLLM(t *testing.T) {
	labels := map[string]string{
		LabelCacheType:         constants.CacheTypeVLLMTorchCompile,
		LabelCacheRootEnv:      constants.VLLMCacheRoot + "=" + testVLLMCacheDir,
		LabelCacheMountSubpath: "torch_compile_cache",
		LabelCacheHash:         "abc123",
	}

	plan, err := Derive(labels)
	if err != nil {
		t.Fatalf("Derive returned error: %v", err)
	}
	if plan.CacheType != constants.CacheTypeVLLMTorchCompile {
		t.Errorf("CacheType: got %q, want %q", plan.CacheType, constants.CacheTypeVLLMTorchCompile)
	}
	if len(plan.Env) != 1 || plan.Env[0].Name != constants.VLLMCacheRoot || plan.Env[0].Value != testVLLMCacheDir {
		t.Errorf("Env: got %+v", plan.Env)
	}
	if plan.MountDir != testVLLMCacheDir {
		t.Errorf("MountDir: got %q", plan.MountDir)
	}
	if plan.SubPath != "torch_compile_cache" {
		t.Errorf("SubPath: got %q", plan.SubPath)
	}
	if plan.PayloadPrefix != constants.MCVVLLMCacheDir {
		t.Errorf("PayloadPrefix: got %q, want %q", plan.PayloadPrefix, constants.MCVVLLMCacheDir)
	}
	if plan.RequiresWritable {
		t.Error("RequiresWritable: got true, want false for vLLM")
	}
}

func TestDeriveHabana(t *testing.T) {
	labels := map[string]string{
		LabelCacheType:         constants.CacheTypeHabanaRecipe,
		LabelCacheRootEnv:      "PT_HPU_RECIPE_CACHE_CONFIG=/home/kserve/.cache/habana,false,8192",
		LabelCacheMountSubpath: ".",
	}

	plan, err := Derive(labels)
	if err != nil {
		t.Fatalf("Derive returned error: %v", err)
	}
	if plan.CacheType != constants.CacheTypeHabanaRecipe {
		t.Errorf("CacheType: got %q, want %q", plan.CacheType, constants.CacheTypeHabanaRecipe)
	}
	// Env must be preserved verbatim, including the ",false,8192" tunables.
	if len(plan.Env) != 1 ||
		plan.Env[0].Name != constants.HabanaRecipeCacheEnv ||
		plan.Env[0].Value != testHabanaCacheEnvValue {
		t.Errorf("Env: got %+v", plan.Env)
	}
	// MountDir must strip the tunable suffix.
	if plan.MountDir != testHabanaCacheDir {
		t.Errorf("MountDir: got %q, want %q", plan.MountDir, testHabanaCacheDir)
	}
	if plan.PayloadPrefix != constants.MCVHabanaCacheDir {
		t.Errorf("PayloadPrefix: got %q, want %q", plan.PayloadPrefix, constants.MCVHabanaCacheDir)
	}
	if !plan.RequiresWritable {
		t.Error("RequiresWritable: got false, want true for Habana")
	}
}

// TestDeriveInfersFromSummary verifies the fallback path for older images that
// carry the per-class summary label but no io.kserve.km/cache-type label.
func TestDeriveInfersFromSummary(t *testing.T) {
	labels := map[string]string{
		summaryLabelHabana: `{"targets":[{"backend":"hpu"}]}`,
		LabelCacheRootEnv:  "PT_HPU_RECIPE_CACHE_CONFIG=/home/kserve/.cache/habana,false,8192",
	}
	plan, err := Derive(labels)
	if err != nil {
		t.Fatalf("Derive returned error: %v", err)
	}
	if plan.CacheType != constants.CacheTypeHabanaRecipe {
		t.Errorf("CacheType: got %q, want %q", plan.CacheType, constants.CacheTypeHabanaRecipe)
	}
}

func TestDeriveUnsupported(t *testing.T) {
	cases := map[string]map[string]string{
		"bare triton":  {summaryLabelTriton: `{"targets":[]}`},
		"unknown type": {LabelCacheType: "future-cache-type"},
	}
	for name, labels := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Derive(labels)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !IsUnsupportedCacheType(err) {
				t.Errorf("expected UnsupportedCacheTypeError, got %v", err)
			}
		})
	}
}

func TestDeriveErrors(t *testing.T) {
	t.Run("nil labels", func(t *testing.T) {
		if _, err := Derive(nil); err == nil {
			t.Error("expected error for nil labels")
		}
	})
	t.Run("missing cache-root-env", func(t *testing.T) {
		_, err := Derive(map[string]string{LabelCacheType: constants.CacheTypeVLLMTorchCompile})
		if err == nil {
			t.Error("expected error for missing cache-root-env")
		}
	})
	t.Run("malformed cache-root-env", func(t *testing.T) {
		_, err := Derive(map[string]string{
			LabelCacheType:    constants.CacheTypeVLLMTorchCompile,
			LabelCacheRootEnv: "NOTANENVVAR",
		})
		if err == nil {
			t.Error("expected error for malformed cache-root-env")
		}
	})
}

func TestProducerEnv(t *testing.T) {
	tests := []struct {
		name      string
		cacheType string
		path      string
		wantName  string
		wantValue string
	}{
		{"habana alias", "habana", testHabanaCacheDir, constants.HabanaRecipeCacheEnv, testHabanaCacheEnvValue},
		{"gaudi alias", "gaudi", "/data/habana", constants.HabanaRecipeCacheEnv, "/data/habana,false,8192"},
		{"habana-recipe id", constants.CacheTypeHabanaRecipe, "/c", constants.HabanaRecipeCacheEnv, "/c,false,8192"},
		{"vllm alias", "vllm", testVLLMCacheDir, constants.VLLMCacheRoot, testVLLMCacheDir},
		{"torch-compile id", constants.CacheTypeVLLMTorchCompile, "/v", constants.VLLMCacheRoot, "/v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ProducerEnv(tt.cacheType, tt.path, DefaultHabanaRecipeCacheSizeMB)
			if err != nil {
				t.Fatalf("ProducerEnv error: %v", err)
			}
			if len(env) != 1 || env[0].Name != tt.wantName || env[0].Value != tt.wantValue {
				t.Errorf("got %+v, want {%s %s}", env, tt.wantName, tt.wantValue)
			}
		})
	}
}

func TestProducerEnvErrors(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if _, err := ProducerEnv("vllm", "", DefaultHabanaRecipeCacheSizeMB); err == nil {
			t.Error("expected error for empty path")
		}
	})
	t.Run("unsupported type", func(t *testing.T) {
		_, err := ProducerEnv("triton", "/c", DefaultHabanaRecipeCacheSizeMB)
		if err == nil || !IsUnsupportedCacheType(err) {
			t.Errorf("expected UnsupportedCacheTypeError, got %v", err)
		}
	})
}

// TestRootEnvLabelRoundTrip proves the produce/consume symmetry: the label
// RootEnvLabel writes decodes back to the same env and mount dir via Derive.
func TestRootEnvLabelRoundTrip(t *testing.T) {
	rootEnv, err := RootEnvLabel(constants.Habana, testHabanaCacheDir, DefaultHabanaRecipeCacheSizeMB)
	if err != nil {
		t.Fatalf("RootEnvLabel error: %v", err)
	}

	plan, err := Derive(map[string]string{
		LabelCacheType:    constants.CacheTypeHabanaRecipe,
		LabelCacheRootEnv: rootEnv,
	})
	if err != nil {
		t.Fatalf("Derive error: %v", err)
	}
	if plan.Env[0].Value != testHabanaCacheEnvValue {
		t.Errorf("round-trip env value: got %q", plan.Env[0].Value)
	}
	if plan.MountDir != testHabanaCacheDir {
		t.Errorf("round-trip mount dir: got %q", plan.MountDir)
	}
}
