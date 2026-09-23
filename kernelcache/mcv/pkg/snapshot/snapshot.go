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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	// Version is the current snapshot document version.
	Version = 2
	// LegacyVersion is the directory-only snapshot format written by older MCV versions.
	LegacyVersion = 1
	// DefaultPath is the default location shared by snapshot and create actions.
	DefaultPath = "/tmp/mcv/cache-snapshot.json"
)

// DefaultExcludedDirectories returns directory names that do not represent cache content.
func DefaultExcludedDirectories() []string {
	return []string{"dummy_cache"}
}

// Document contains directory snapshots for one or more cache roots.
type Document struct {
	Version int    `json:"version"`
	Roots   []Root `json:"roots"`
}

// Root contains the recursive directories and regular files for one cache root.
type Root struct {
	Source              string   `json:"source"`
	ExcludedDirectories []string `json:"excludedDirectories,omitempty"`
	Directories         []string `json:"directories"`
	Files               []File   `json:"files"`
}

// File contains the content identity of one regular file in a cache root.
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// RootOptions configures a recursive directory snapshot for one cache root.
type RootOptions struct {
	Source              string
	ExcludedDirectories []string
}

// Delta contains changed cache entries and the directories required in an OCI layer.
type Delta struct {
	Source              string
	AddedDirectories    []string
	ChangedFiles        []string
	DeletedFiles        []string
	ContentDirectories  []string
	RequiredDirectories []string
	RequiresFullImage   bool
}

// Capture records recursive directory and regular file state.
func Capture(roots []string) (*Document, error) {
	options := make([]RootOptions, 0, len(roots))
	for _, root := range roots {
		options = append(options, RootOptions{Source: root})
	}
	return CaptureRoots(options)
}

// CaptureRoots records recursive directory and regular file state.
func CaptureRoots(roots []RootOptions) (*Document, error) {
	if len(roots) == 0 {
		return nil, errors.New("at least one snapshot root is required")
	}
	document := &Document{Version: Version, Roots: make([]Root, 0, len(roots))}
	seen := make(map[string]struct{}, len(roots))
	for _, options := range roots {
		root := filepath.Clean(options.Source)
		if root == "." || !filepath.IsAbs(root) {
			return nil, fmt.Errorf("snapshot root must be an absolute path: %q", options.Source)
		}
		if _, exists := seen[root]; exists {
			return nil, fmt.Errorf("duplicate snapshot root: %s", root)
		}
		seen[root] = struct{}{}
		excluded, err := normalizeExcludedDirectories(options.ExcludedDirectories)
		if err != nil {
			return nil, fmt.Errorf("invalid excluded directories for snapshot root %s: %w", root, err)
		}

		info, err := os.Lstat(root)
		if err != nil {
			return nil, fmt.Errorf("stat snapshot root %s: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("snapshot root is not a directory: %s", root)
		}
		rootFS, err := os.OpenRoot(root)
		if err != nil {
			return nil, fmt.Errorf("open snapshot root %s: %w", root, err)
		}

		directories := make([]string, 0)
		files := make([]File, 0)
		walkErr := fs.WalkDir(rootFS.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if relative == "." {
				return nil
			}
			relative = filepath.ToSlash(relative)
			if excludedDirectory(relative, excluded) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				directories = append(directories, relative)
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			digest, err := fileDigest(rootFS, filepath.FromSlash(relative))
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files = append(files, File{Path: relative, Size: info.Size(), Digest: digest})
			return nil
		})
		closeErr := rootFS.Close()
		if err := errors.Join(walkErr, closeErr); err != nil {
			return nil, fmt.Errorf("walk snapshot root %s: %w", root, err)
		}
		sort.Strings(directories)
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		document.Roots = append(document.Roots, Root{
			Source:              root,
			ExcludedDirectories: excluded,
			Directories:         directories,
			Files:               files,
		})
	}
	sort.Slice(document.Roots, func(i, j int) bool {
		return document.Roots[i].Source < document.Roots[j].Source
	})
	return document, nil
}

// Compare returns directories that were added after the previous snapshot.
func Compare(before, after *Document) ([]Delta, error) {
	if err := Validate(before); err != nil {
		return nil, fmt.Errorf("invalid previous snapshot: %w", err)
	}
	if err := Validate(after); err != nil {
		return nil, fmt.Errorf("invalid current snapshot: %w", err)
	}
	if before.Version != Version || after.Version != Version {
		return nil, errors.New("snapshot version does not contain file state")
	}

	previous := make(map[string]Root, len(before.Roots))
	for _, root := range before.Roots {
		previous[root.Source] = root
	}

	deltas := make([]Delta, 0, len(after.Roots))
	for _, root := range after.Roots {
		previousRoot, exists := previous[root.Source]
		if !exists {
			return nil, fmt.Errorf("snapshot root was not present in previous snapshot: %s", root.Source)
		}
		if !sameDirectories(previousRoot.ExcludedDirectories, root.ExcludedDirectories) {
			return nil, fmt.Errorf("snapshot exclusions changed for root: %s", root.Source)
		}
		known := make(map[string]struct{}, len(previousRoot.Directories))
		for _, directory := range previousRoot.Directories {
			known[directory] = struct{}{}
		}
		added := make([]string, 0)
		for _, directory := range root.Directories {
			if _, exists := known[directory]; !exists {
				added = append(added, directory)
			}
		}
		previousFiles := make(map[string]File, len(previousRoot.Files))
		for _, file := range previousRoot.Files {
			previousFiles[file.Path] = file
		}
		currentFiles := make(map[string]File, len(root.Files))
		changedFiles := make([]string, 0)
		for _, file := range root.Files {
			currentFiles[file.Path] = file
			previousFile, exists := previousFiles[file.Path]
			if !exists || previousFile.Size != file.Size || previousFile.Digest != file.Digest {
				changedFiles = append(changedFiles, file.Path)
			}
		}
		deletedFiles := make([]string, 0)
		for path := range previousFiles {
			if _, exists := currentFiles[path]; !exists {
				deletedFiles = append(deletedFiles, path)
			}
		}
		sort.Strings(changedFiles)
		sort.Strings(deletedFiles)
		if len(added) == 0 && len(changedFiles) == 0 && len(deletedFiles) == 0 {
			continue
		}
		changedParents, changedRootFiles := fileParentDirectories(changedFiles)
		deletedParents, deletedRootFiles := fileParentDirectories(deletedFiles)
		contentInputs := append(append([]string(nil), added...), changedParents...)
		contentInputs = append(contentInputs, deletedParents...)
		deltas = append(deltas, Delta{
			Source:              root.Source,
			AddedDirectories:    added,
			ChangedFiles:        changedFiles,
			DeletedFiles:        deletedFiles,
			ContentDirectories:  contentDirectories(contentInputs),
			RequiredDirectories: requiredDirectories(contentInputs),
			RequiresFullImage:   changedRootFiles || deletedRootFiles || len(deletedFiles) > 0,
		})
	}
	return deltas, nil
}

func fileDigest(root *os.Root, path string) (string, error) {
	file, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() {
		return "", fmt.Errorf("snapshot file is not regular: %s", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	after, err := file.Stat()
	if err != nil {
		return "", err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("snapshot file changed during hashing: %s", path)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileParentDirectories(files []string) ([]string, bool) {
	parents := make([]string, 0, len(files))
	rootFile := false
	for _, file := range files {
		parent := filepath.ToSlash(filepath.Dir(file))
		if parent != "." {
			parents = append(parents, parent)
		} else {
			rootFile = true
		}
	}
	return parents, rootFile
}

func contentDirectories(added []string) []string {
	ordered := make([]string, 0, len(added))
	seen := make(map[string]struct{}, len(added))
	for _, directory := range added {
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		ordered = append(ordered, directory)
	}
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth := pathDepth(ordered[i])
		rightDepth := pathDepth(ordered[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return ordered[i] < ordered[j]
	})
	content := make([]string, 0, len(ordered))
	for _, directory := range ordered {
		covered := false
		for _, parent := range content {
			if strings.HasPrefix(directory, parent+"/") {
				covered = true
				break
			}
		}
		if !covered {
			content = append(content, directory)
		}
	}
	return content
}

// Read loads and validates a snapshot document.
func Read(path string) (*Document, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the CLI accepts an explicit snapshot path.
	if err != nil {
		return nil, err
	}
	document := &Document{}
	if err := json.Unmarshal(data, document); err != nil {
		return nil, err
	}
	if err := Validate(document); err != nil {
		return nil, err
	}
	return document, nil
}

// Write atomically writes a validated snapshot document.
func Write(path string, document *Document) error {
	if err := Validate(document); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".mcv-snapshot-")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		return errors.Join(err, file.Close())
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

// Validate checks the snapshot version, roots, and relative directory paths.
func Validate(document *Document) error {
	if document == nil {
		return errors.New("snapshot document is required")
	}
	if document.Version != Version && document.Version != LegacyVersion {
		return fmt.Errorf("unsupported snapshot version: %d", document.Version)
	}
	if len(document.Roots) == 0 {
		return errors.New("snapshot must contain at least one root")
	}
	seenRoots := make(map[string]struct{}, len(document.Roots))
	for _, root := range document.Roots {
		if root.Source == "" || !filepath.IsAbs(root.Source) || filepath.Clean(root.Source) != root.Source {
			return fmt.Errorf("invalid snapshot root: %q", root.Source)
		}
		if _, exists := seenRoots[root.Source]; exists {
			return fmt.Errorf("duplicate snapshot root: %s", root.Source)
		}
		seenRoots[root.Source] = struct{}{}
		if _, err := normalizeExcludedDirectories(root.ExcludedDirectories); err != nil {
			return fmt.Errorf("invalid excluded directory for snapshot root %s: %w", root.Source, err)
		}
		seenDirectories := make(map[string]struct{}, len(root.Directories))
		for _, directory := range root.Directories {
			clean := filepath.ToSlash(filepath.Clean(directory))
			if directory == "" || clean == "." || clean != directory || filepath.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, "../") {
				return fmt.Errorf("invalid directory %q for snapshot root %s", directory, root.Source)
			}
			if _, exists := seenDirectories[directory]; exists {
				return fmt.Errorf("duplicate directory %q for snapshot root %s", directory, root.Source)
			}
			seenDirectories[directory] = struct{}{}
		}
		seenFiles := make(map[string]struct{}, len(root.Files))
		for _, file := range root.Files {
			clean := filepath.ToSlash(filepath.Clean(file.Path))
			if file.Path == "" || clean == "." || clean != file.Path || filepath.IsAbs(file.Path) || file.Path == ".." || strings.HasPrefix(file.Path, "../") {
				return fmt.Errorf("invalid file %q for snapshot root %s", file.Path, root.Source)
			}
			if _, exists := seenFiles[file.Path]; exists {
				return fmt.Errorf("duplicate file %q for snapshot root %s", file.Path, root.Source)
			}
			seenFiles[file.Path] = struct{}{}
			if file.Size < 0 || len(file.Digest) != sha256.Size*2 {
				return fmt.Errorf("invalid file state for %q in snapshot root %s", file.Path, root.Source)
			}
			if _, err := hex.DecodeString(file.Digest); err != nil {
				return fmt.Errorf("invalid file digest for %q in snapshot root %s: %w", file.Path, root.Source, err)
			}
		}
	}
	return nil
}

func normalizeExcludedDirectories(directories []string) ([]string, error) {
	if len(directories) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(directories))
	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		clean := filepath.ToSlash(filepath.Clean(directory))
		if directory == "" || clean == "." || clean != directory || filepath.IsAbs(directory) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("invalid excluded directory %q", directory)
		}
		if _, exists := seen[clean]; exists {
			return nil, fmt.Errorf("duplicate excluded directory %q", directory)
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func excludedDirectory(relative string, excluded []string) bool {
	for _, directory := range excluded {
		if relative == directory || strings.HasPrefix(relative, directory+"/") {
			return true
		}
	}
	return false
}

func sameDirectories(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, directory := range left {
		values[directory] = struct{}{}
	}
	for _, directory := range right {
		if _, exists := values[directory]; !exists {
			return false
		}
	}
	return true
}

func requiredDirectories(added []string) []string {
	required := make(map[string]struct{})
	for _, directory := range added {
		for current := directory; current != "." && current != ""; current = filepath.ToSlash(filepath.Dir(current)) {
			required[current] = struct{}{}
		}
	}
	directories := make([]string, 0, len(required))
	for directory := range required {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := pathDepth(directories[i])
		rightDepth := pathDepth(directories[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return directories[i] < directories[j]
	})
	return directories
}

func pathDepth(path string) int {
	depth := 1
	for _, character := range path {
		if character == '/' {
			depth++
		}
	}
	return depth
}
