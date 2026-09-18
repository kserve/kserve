package cache

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testDeviceID       = "a16fb581eee"
	testSynapseVersion = "1.24.1.6210fda"
	testSynapseShort   = "1.24.0"
)

func TestRecipeFileRegex(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantHash string
		wantDev  string
		wantVer  string
		wantOK   bool
	}{
		{
			name:     "valid recipe file",
			filename: "10034702590219030139_a16fb581eee_syn1.24.1.6210fda.recipe",
			wantHash: "10034702590219030139",
			wantDev:  testDeviceID,
			wantVer:  testSynapseVersion,
			wantOK:   true,
		},
		{
			name:     "metadata file does not match recipe regex",
			filename: "10034702590219030139_a16fb581eee_syn1.24.1.6210fda.metadata",
			wantOK:   false,
		},
		{
			name:     "debug dir does not match",
			filename: "10034702590219030139_a16fb581eee_syn1.24.1.6210fda.recipe_debug_files",
			wantOK:   false,
		},
		{
			name:     "random file",
			filename: "README.md",
			wantOK:   false,
		},
		{
			name:     "short hash",
			filename: "123_abc_syn2.0.recipe",
			wantHash: "123",
			wantDev:  "abc",
			wantVer:  "2.0",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := recipeFileRegex.FindStringSubmatch(tt.filename)
			if !tt.wantOK {
				if m != nil {
					t.Errorf("expected no match for %q, got %v", tt.filename, m)
				}
				return
			}
			if m == nil {
				t.Fatalf("expected match for %q, got nil", tt.filename)
			}
			if m[1] != tt.wantHash {
				t.Errorf("hash: got %q, want %q", m[1], tt.wantHash)
			}
			if m[2] != tt.wantDev {
				t.Errorf("device: got %q, want %q", m[2], tt.wantDev)
			}
			if m[3] != tt.wantVer {
				t.Errorf("version: got %q, want %q", m[3], tt.wantVer)
			}
		})
	}
}

func TestDetectHabanaCache(t *testing.T) {
	t.Run("detects valid cache", func(t *testing.T) {
		dir := t.TempDir()
		// Create two recipe triplets
		files := []struct {
			name string
			size int
		}{
			{"111_aaa_syn" + testSynapseShort + ".recipe", 1024},
			{"111_aaa_syn" + testSynapseShort + ".metadata", 128},
			{"222_aaa_syn" + testSynapseShort + ".recipe", 2048},
			{"222_aaa_syn" + testSynapseShort + ".metadata", 256},
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(dir, f.name), make([]byte, f.size), 0644); err != nil {
				t.Fatal(err)
			}
		}
		// Also create a debug dir (should be ignored)
		os.Mkdir(filepath.Join(dir, "111_aaa_syn"+testSynapseShort+".recipe_debug_files"), 0755)

		cache := DetectHabanaCache(dir)
		if cache == nil {
			t.Fatal("expected cache to be detected")
		}
		if cache.EntryCount() != 2 {
			t.Errorf("entry count: got %d, want 2", cache.EntryCount())
		}
		if cache.Name() != "habana" {
			t.Errorf("name: got %q, want %q", cache.Name(), "habana")
		}
		// Verify metadata
		for _, m := range cache.allMetadata {
			if m.DeviceID != "aaa" {
				t.Errorf("deviceID: got %q, want %q", m.DeviceID, "aaa")
			}
			if m.SynapseVersion != testSynapseShort {
				t.Errorf("version: got %q, want %q", m.SynapseVersion, testSynapseShort)
			}
		}
	})

	t.Run("returns nil for empty dir", func(t *testing.T) {
		dir := t.TempDir()
		cache := DetectHabanaCache(dir)
		if cache != nil {
			t.Error("expected nil for empty directory")
		}
	})

	t.Run("returns nil for non-habana files", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "somefile.txt"), []byte("hello"), 0644)
		cache := DetectHabanaCache(dir)
		if cache != nil {
			t.Error("expected nil for non-habana files")
		}
	})

	t.Run("returns nil for nonexistent dir", func(t *testing.T) {
		cache := DetectHabanaCache("/nonexistent/path")
		if cache != nil {
			t.Error("expected nil for nonexistent directory")
		}
	})
}

func TestHabanaLabels(t *testing.T) {
	h := &HabanaCache{
		rootPath: t.TempDir(),
		allMetadata: []HabanaRecipeMetadata{
			{GraphHash: "111", DeviceID: testDeviceID, SynapseVersion: testSynapseVersion, RecipeSize: 1024},
		},
	}

	labels := h.Labels()

	// The cache-root-env label must carry the full PT_HPU_RECIPE_CACHE_CONFIG
	// value (name, dir, and the ",false,8192" tunables), not just the env name.
	want := "PT_HPU_RECIPE_CACHE_CONFIG=/home/kserve/.cache/habana,false,8192"
	if got := labels["io.kserve.km/cache-root-env"]; got != want {
		t.Errorf("cache-root-env: got %q, want %q", got, want)
	}
	if got := labels["io.kserve.km/cache-type"]; got != "habana-recipe" {
		t.Errorf("cache-type: got %q, want %q", got, "habana-recipe")
	}
	if got := labels["io.kserve.km/cache-mount-subpath"]; got != "." {
		t.Errorf("cache-mount-subpath: got %q, want %q", got, ".")
	}
}

func TestBuildHabanaSummary(t *testing.T) {
	metadata := []HabanaRecipeMetadata{
		{GraphHash: "111", DeviceID: testDeviceID, SynapseVersion: testSynapseVersion, RecipeSize: 1024},
		{GraphHash: "222", DeviceID: testDeviceID, SynapseVersion: testSynapseVersion, RecipeSize: 2048},
	}

	summary, err := buildHabanaSummary(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Targets) != 1 {
		t.Fatalf("targets: got %d, want 1", len(summary.Targets))
	}
	if summary.Targets[0].Backend != "hpu" {
		t.Errorf("backend: got %q, want %q", summary.Targets[0].Backend, "hpu")
	}
	if summary.Targets[0].WarpSize != 0 {
		t.Errorf("warpSize: got %d, want 0", summary.Targets[0].WarpSize)
	}

	t.Run("empty metadata returns error", func(t *testing.T) {
		_, err := buildHabanaSummary(nil)
		if err == nil {
			t.Error("expected error for nil metadata")
		}
	})

	t.Run("multiple device IDs produce multiple targets", func(t *testing.T) {
		meta := []HabanaRecipeMetadata{
			{GraphHash: "111", DeviceID: "aaa", SynapseVersion: testSynapseShort},
			{GraphHash: "222", DeviceID: "bbb", SynapseVersion: testSynapseShort},
		}
		s, err := buildHabanaSummary(meta)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Targets) != 2 {
			t.Errorf("targets: got %d, want 2", len(s.Targets))
		}
	})
}
