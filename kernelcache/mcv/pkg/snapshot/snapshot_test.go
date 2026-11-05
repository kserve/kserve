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

package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureAndCompare(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "torch_compile_cache", "torch_aot_compile", "existing"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "root-file"), []byte("ignored"), 0o600))

	before, err := Capture([]string{root})
	require.NoError(t, err)
	require.Equal(t, []string{
		"torch_compile_cache",
		"torch_compile_cache/torch_aot_compile",
		"torch_compile_cache/torch_aot_compile/existing",
	}, before.Roots[0].Directories)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "torch_compile_cache", "torch_aot_compile", "new-hash", "nested"), 0o700))
	after, err := Capture([]string{root})
	require.NoError(t, err)
	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.Equal(t, []string{
		"torch_compile_cache/torch_aot_compile/new-hash",
		"torch_compile_cache/torch_aot_compile/new-hash/nested",
	}, deltas[0].AddedDirectories)
	require.Equal(t, []string{
		"torch_compile_cache/torch_aot_compile/new-hash",
	}, deltas[0].ContentDirectories)
	require.Equal(t, []string{
		"torch_compile_cache",
		"torch_compile_cache/torch_aot_compile",
		"torch_compile_cache/torch_aot_compile/new-hash",
		"torch_compile_cache/torch_aot_compile/new-hash/nested",
	}, deltas[0].RequiredDirectories)
}

// Detects a modified file in an existing cache directory.
func TestCompareDetectsModifiedFileInExistingDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "torch_compile_cache", "existing")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	file := filepath.Join(directory, "kernel.bin")
	require.NoError(t, os.WriteFile(file, []byte("before"), 0o600))

	before, err := Capture([]string{root})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("after"), 0o600))
	after, err := Capture([]string{root})
	require.NoError(t, err)

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.Empty(t, deltas[0].AddedDirectories)
	require.Equal(t, []string{"torch_compile_cache/existing/kernel.bin"}, deltas[0].ChangedFiles)
	require.Empty(t, deltas[0].DeletedFiles)
	require.Equal(t, []string{"torch_compile_cache/existing"}, deltas[0].ContentDirectories)
}

// Deduplicates content directories when multiple files change together.
func TestCompareDeduplicatesChangedFileDirectories(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "torch_compile_cache", "existing")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	firstFile := filepath.Join(directory, "first.bin")
	secondFile := filepath.Join(directory, "second.bin")
	require.NoError(t, os.WriteFile(firstFile, []byte("before-first"), 0o600))
	require.NoError(t, os.WriteFile(secondFile, []byte("before-second"), 0o600))

	before, err := Capture([]string{root})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(firstFile, []byte("after-first"), 0o600))
	require.NoError(t, os.WriteFile(secondFile, []byte("after-second"), 0o600))
	after, err := Capture([]string{root})
	require.NoError(t, err)

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.Equal(t, []string{
		"torch_compile_cache/existing/first.bin",
		"torch_compile_cache/existing/second.bin",
	}, deltas[0].ChangedFiles)
	require.Equal(t, []string{"torch_compile_cache/existing"}, deltas[0].ContentDirectories)
}

func TestCompareDetectsAddedFileInExistingDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "torch_compile_cache", "existing")
	require.NoError(t, os.MkdirAll(directory, 0o700))

	before, err := Capture([]string{root})
	require.NoError(t, err)
	file := filepath.Join(directory, "kernel.bin")
	require.NoError(t, os.WriteFile(file, []byte("after"), 0o600))
	after, err := Capture([]string{root})
	require.NoError(t, err)

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.Equal(t, []string{"torch_compile_cache/existing/kernel.bin"}, deltas[0].ChangedFiles)
	require.Empty(t, deltas[0].DeletedFiles)
	require.Equal(t, []string{"torch_compile_cache/existing"}, deltas[0].ContentDirectories)
}

func TestCompareDetectsDeletedFile(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "torch_compile_cache", "existing")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	file := filepath.Join(directory, "kernel.bin")
	require.NoError(t, os.WriteFile(file, []byte("before"), 0o600))

	before, err := Capture([]string{root})
	require.NoError(t, err)
	require.NoError(t, os.Remove(file))
	after, err := Capture([]string{root})
	require.NoError(t, err)

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.Empty(t, deltas[0].AddedDirectories)
	require.Equal(t, []string{"torch_compile_cache/existing/kernel.bin"}, deltas[0].DeletedFiles)
	require.Equal(t, []string{"torch_compile_cache/existing"}, deltas[0].ContentDirectories)
}

func TestCompareMarksRootFileChangeForFullImage(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "cache.bin")
	require.NoError(t, os.WriteFile(file, []byte("before"), 0o600))
	before, err := Capture([]string{root})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("after"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "new-cache"), 0o700))
	after, err := Capture([]string{root})
	require.NoError(t, err)

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	require.True(t, deltas[0].RequiresFullImage)
	require.Equal(t, []string{"new-cache"}, deltas[0].ContentDirectories)
}

func TestWriteAndRead(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vllm", "cache"), 0o700))
	document, err := Capture([]string{root})
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "state", "snapshot.json")
	require.NoError(t, Write(path, document))
	loaded, err := Read(path)
	require.NoError(t, err)
	require.Equal(t, document, loaded)
}

func TestCompareNormalizesUnsortedDirectoryOrder(t *testing.T) {
	root := "/cache"
	before := &Document{Version: Version, Roots: []Root{{Source: root}}}
	after := &Document{Version: Version, Roots: []Root{{
		Source: root,
		Directories: []string{
			"parent/new/nested",
			"parent",
			"parent/new",
		},
	}}}

	deltas, err := Compare(before, after)
	require.NoError(t, err)
	require.Equal(t, []string{"parent"}, deltas[0].ContentDirectories)
}

func TestCaptureRootsExcludesDirectoriesPerRoot(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(firstRoot, "included", "nested"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(firstRoot, "dummy_cache", "nested"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(firstRoot, "dummy_cache2"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(secondRoot, "temporary", "nested"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(secondRoot, "dummy_cache", "nested"), 0o700))

	document, err := CaptureRoots([]RootOptions{
		{
			Source:              firstRoot,
			ExcludedDirectories: []string{"dummy_cache"},
		},
		{
			Source:              secondRoot,
			ExcludedDirectories: []string{"temporary"},
		},
	})
	require.NoError(t, err)

	roots := make(map[string]Root, len(document.Roots))
	for _, root := range document.Roots {
		roots[root.Source] = root
	}
	require.Equal(t, []string{"dummy_cache"}, roots[firstRoot].ExcludedDirectories)
	require.Equal(t, []string{"dummy_cache2", "included", "included/nested"}, roots[firstRoot].Directories)
	require.Equal(t, []string{"temporary"}, roots[secondRoot].ExcludedDirectories)
	require.Equal(t, []string{"dummy_cache", "dummy_cache/nested"}, roots[secondRoot].Directories)
}

func TestCaptureRootsRejectsInvalidExcludedDirectory(t *testing.T) {
	root := t.TempDir()
	for _, excluded := range []string{"", ".", "..", "../cache", "/cache", "parent/../child"} {
		t.Run(excluded, func(t *testing.T) {
			_, err := CaptureRoots([]RootOptions{{
				Source:              root,
				ExcludedDirectories: []string{excluded},
			}})
			require.Error(t, err)
		})
	}
}

func TestCompareRejectsChangedExcludedDirectories(t *testing.T) {
	root := "/cache"
	before := &Document{Version: Version, Roots: []Root{{
		Source:              root,
		ExcludedDirectories: []string{"dummy_cache"},
		Directories:         []string{"torch_compile_cache"},
	}}}
	after := &Document{Version: Version, Roots: []Root{{
		Source:      root,
		Directories: []string{"torch_compile_cache"},
	}}}

	_, err := Compare(before, after)
	require.ErrorContains(t, err, "snapshot exclusions changed")
}

func TestReadLegacySnapshotWithoutExcludedDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "version": 1,
  "roots": [
    {
      "source": "/cache",
      "directories": ["torch_compile_cache"]
    }
  ]
}`), 0o600))

	document, err := Read(path)
	require.NoError(t, err)
	require.Empty(t, document.Roots[0].ExcludedDirectories)
}

// Rejects directory-only snapshots on either side of a file-state comparison.
func TestCompareRejectsLegacySnapshotWithoutFileState(t *testing.T) {
	root := t.TempDir()
	legacy := &Document{
		Version: LegacyVersion,
		Roots:   []Root{{Source: root, Directories: []string{"cache"}}},
	}
	current, err := Capture([]string{root})
	require.NoError(t, err)

	for _, test := range []struct {
		name   string
		before *Document
		after  *Document
	}{
		{name: "legacy previous", before: legacy, after: current},
		{name: "legacy current", before: current, after: legacy},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compare(test.before, test.after)
			require.ErrorContains(t, err, "does not contain file state")
		})
	}
}
