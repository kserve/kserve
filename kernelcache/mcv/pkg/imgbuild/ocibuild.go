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

package imgbuild

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/registryauth"
	cachesnapshot "github.com/kserve/kserve/kernelcache/mcv/pkg/snapshot"
)

type ociBuilder struct{}

func (b *ociBuilder) CreateImage(imageName, cacheDir string) error {
	_, err := b.CreateImageWithResult(imageName, cacheDir)
	return err
}

func (b *ociBuilder) CreateImageWithResult(imageName, cacheDir string) (*CreateResult, error) {
	return b.createImageWithResult(imageName, cacheDir, nil, nil)
}

func (b *ociBuilder) CreateDeltaImageWithResult(imageName, cacheDir, snapshotPath string) (*CreateResult, error) {
	previous, err := cachesnapshot.Read(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read delta snapshot %s: %w", snapshotPath, err)
	}
	cacheDir = filepath.Clean(cacheDir)
	previousRoot, found := snapshotRoot(previous, cacheDir)
	if !found {
		return nil, fmt.Errorf("delta snapshot does not contain the requested cache root: %s", cacheDir)
	}
	if previous.Version != cachesnapshot.Version {
		logging.Infof("Snapshot version %d has no file state; creating a full OCI cache image", previous.Version)
		return b.createImageWithResult(imageName, cacheDir, nil, previousRoot.ExcludedDirectories)
	}
	current, err := cachesnapshot.CaptureRoots([]cachesnapshot.RootOptions{{
		Source:              cacheDir,
		ExcludedDirectories: previousRoot.ExcludedDirectories,
	}})
	if err != nil {
		return nil, fmt.Errorf("capture current cache directories: %w", err)
	}
	if len(previousRoot.Directories) == 0 && len(previousRoot.Files) == 0 {
		if !snapshotHasContent(current) {
			return unchangedResult(), nil
		}
		logging.Info("No existing cache directories found; creating initial OCI cache image")
		return b.createImageWithResult(imageName, cacheDir, nil, previousRoot.ExcludedDirectories)
	}
	deltas, err := cachesnapshot.Compare(previous, current)
	if err != nil {
		return nil, fmt.Errorf("compare cache snapshots: %w", err)
	}
	if len(deltas) == 0 {
		return unchangedResult(), nil
	}
	for _, delta := range deltas {
		logging.Infof("Snapshot delta detected: source=%s addedDirectories=%v changedFiles=%v deletedFiles=%v contentDirectories=%v", delta.Source, delta.AddedDirectories, delta.ChangedFiles, delta.DeletedFiles, delta.ContentDirectories)
	}
	if len(deltas) != 1 || deltas[0].Source != filepath.Clean(cacheDir) {
		return nil, errors.New("delta snapshot does not contain the requested cache root")
	}
	delta := deltas[0]
	if delta.RequiresFullImage || len(delta.ContentDirectories) == 0 {
		logging.Info("Snapshot delta cannot be represented as a directory-only OCI layer; creating a full OCI cache image")
		return b.createImageWithResult(imageName, cacheDir, nil, previousRoot.ExcludedDirectories)
	}
	logging.Info("New cache directories found; creating delta OCI cache image")
	return b.createImageWithResult(imageName, cacheDir, delta.ContentDirectories, previousRoot.ExcludedDirectories)
}

func snapshotRoot(document *cachesnapshot.Document, source string) (cachesnapshot.Root, bool) {
	for _, root := range document.Roots {
		if root.Source == source {
			return root, true
		}
	}
	return cachesnapshot.Root{}, false
}

func unchangedResult() *CreateResult {
	return &CreateResult{State: CreateStateUnchanged, CompletedAt: time.Now().UTC()}
}

func snapshotHasContent(document *cachesnapshot.Document) bool {
	for _, root := range document.Roots {
		if len(root.Directories) > 0 || len(root.Files) > 0 {
			return true
		}
	}
	return false
}

func (b *ociBuilder) createImageWithResult(imageName, cacheDir string, includedDirectories, excludedDirectories []string) (*CreateResult, error) {
	ref, err := name.NewTag(imageName, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI destination tag: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workDir, err := os.MkdirTemp("", "mcv-oci-")
	if err != nil {
		return nil, err
	}
	defer CleanupDirs(workDir)

	snapshot := filepath.Join(workDir, "snapshot")
	if err := snapshotOCICacheDirectories(ctx, cacheDir, snapshot, includedDirectories, excludedDirectories); err != nil {
		return nil, fmt.Errorf("snapshot cache: %w", err)
	}
	prep, err := prepareBuildContextAt(filepath.Join(workDir, "build"), snapshot)
	if err != nil {
		return nil, err
	}
	img, err := packageOCIImage(ctx, prep, filepath.Join(workDir, "layer.tar"))
	if err != nil {
		return nil, err
	}
	digest, err := img.Digest()
	if err != nil {
		return nil, err
	}
	remoteOptions, err := registryauth.RemoteOptions(ctx, ref.Context().RegistryStr())
	if err != nil {
		return nil, err
	}
	logging.Infof("Pushing OCI image to %s", ref.Name())
	if err := remote.Write(ref, img, remoteOptions...); err != nil {
		return nil, fmt.Errorf("push OCI image %s: %w", ref.Name(), err)
	}
	imageReference := ref.Context().Digest(digest.String()).Name()
	logging.Infof("OCI image pushed: %s", imageReference)
	var cacheSizeBytes int64
	for _, cache := range prep.Caches {
		cacheSizeBytes += cache.CacheSizeBytes()
	}
	return &CreateResult{
		State:          CreateStateSucceeded,
		ImageReference: imageReference,
		CacheSizeBytes: cacheSizeBytes,
		CompletedAt:    time.Now().UTC(),
	}, nil
}

// Snapshot before detection so metadata readers only see private, regular files.
func snapshotOCICache(ctx context.Context, source, destination string) error {
	return snapshotOCICacheDirectories(ctx, source, destination, nil, nil)
}

func snapshotOCICacheDirectories(ctx context.Context, source, destination string, includedDirectories, excludedDirectories []string) error {
	included, err := normalizeCacheDirectories(includedDirectories, "included")
	if err != nil {
		return err
	}
	excluded, err := normalizeCacheDirectories(excludedDirectories, "excluded")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	var linkRoot *os.Root
	linkPath := os.Getenv("MCV_CACHE_LINK_ROOT")
	if linkPath != "" {
		if !filepath.IsAbs(linkPath) || filepath.Clean(linkPath) == "/" {
			return errors.New("MCV_CACHE_LINK_ROOT must name an absolute cache directory")
		}
		linkRoot, err = os.OpenRoot(linkPath)
		if err != nil {
			return err
		}
		defer linkRoot.Close()
	}
	return fs.WalkDir(root.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && excludedDirectory(relative, excluded) {
			return filepath.SkipDir
		}
		if strings.HasPrefix(entry.Name(), ".wh.") {
			return fmt.Errorf("OCI whiteout name is not allowed: %s", relative)
		}
		if entry.IsDir() && !includedDirectory(relative, included) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && !includedFile(relative, included) {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		fileRoot, filePath := root, filepath.FromSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 && linkRoot != nil {
			link, err := root.Readlink(filePath)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(link) {
				return fmt.Errorf("cache link must be absolute: %s", relative)
			}
			filePath, err = filepath.Rel(linkPath, link)
			if err != nil || !filepath.IsLocal(filePath) {
				return fmt.Errorf("cache link is outside the allowed root: %s", relative)
			}
			fileRoot = linkRoot
		} else if !entry.Type().IsRegular() {
			return fmt.Errorf("cache entry must be a regular file or directory: %s", relative)
		}
		// Root confines path resolution; flags reject a swapped symlink or FIFO.
		src, err := fileRoot.OpenFile(filePath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer src.Close()
		info, err := src.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cache entry changed to a special file: %s", relative)
		}
		dst, err := os.OpenFile(filepath.Clean(target), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(dst, src, info.Size())
		closeErr := dst.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		after, err := src.Stat()
		if err != nil {
			return err
		}
		if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("cache file changed during capture; retry after writes settle: %s", relative)
		}
		return nil
	})
}

func normalizeCacheDirectories(directories []string, kind string) ([]string, error) {
	if len(directories) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(directories))
	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		clean := filepath.ToSlash(filepath.Clean(directory))
		if clean == "." || clean != directory || !filepath.IsLocal(filepath.FromSlash(clean)) {
			return nil, fmt.Errorf("invalid %s cache directory: %s", kind, directory)
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	return normalized, nil
}

func excludedDirectory(relative string, excluded []string) bool {
	if len(excluded) == 0 || relative == "." {
		return false
	}
	relative = filepath.ToSlash(relative)
	for _, directory := range excluded {
		if relative == directory || strings.HasPrefix(relative, directory+"/") {
			return true
		}
	}
	return false
}

func includedDirectory(relative string, included []string) bool {
	if len(included) == 0 || relative == "." {
		return true
	}
	relative = filepath.ToSlash(relative)
	for _, directory := range included {
		if relative == directory || strings.HasPrefix(relative, directory+"/") || strings.HasPrefix(directory, relative+"/") {
			return true
		}
	}
	return false
}

func includedFile(relative string, included []string) bool {
	if len(included) == 0 {
		return true
	}
	relative = filepath.ToSlash(relative)
	for _, directory := range included {
		if strings.HasPrefix(relative, directory+"/") {
			return true
		}
	}
	return false
}

func packageOCIImage(ctx context.Context, prep *buildContext, layerPath string) (v1.Image, error) {
	file, err := os.OpenFile(filepath.Clean(layerPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(file)
	writeErr := writeOCITree(ctx, tw, prep.CacheBuildDir, prep.CacheTag)
	if writeErr == nil {
		writeErr = writeOCITree(ctx, tw, prep.ManifestBuildDir, prep.ManifestTag)
	}
	if err := errors.Join(writeErr, tw.Close(), file.Close()); err != nil {
		return nil, fmt.Errorf("write OCI layer: %w", err)
	}
	layer, err := tarball.LayerFromFile(layerPath, tarball.WithMediaType(types.OCILayer))
	if err != nil {
		return nil, err
	}
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return nil, err
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg.OS = "linux"
	cfg.Architecture = runtime.GOARCH
	cfg.Config.Labels = prep.Labels
	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, err
	}
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	return mutate.ConfigMediaType(img, types.OCIConfigJSON), nil
}

func writeOCITree(ctx context.Context, tw *tar.Writer, directory, prefix string) error {
	normalizedPrefix, err := normalizeOCIPrefix(prefix)
	if err != nil {
		return err
	}
	return filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported OCI entry: %s", filename)
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		// Ownership and access modes belong to the artifact, not the producer UID.
		header := &tar.Header{Name: path.Join(normalizedPrefix, filepath.ToSlash(relative)), Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg}
		if info.IsDir() {
			header.Typeflag, header.Mode, header.Size = tar.TypeDir, 0o755, 0
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(filepath.Clean(filename))
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.CopyN(tw, file, info.Size())
		return err
	})
}

func normalizeOCIPrefix(prefix string) (string, error) {
	if prefix == "" || strings.ContainsAny(prefix, "\\\x00") {
		return "", fmt.Errorf("invalid OCI directory: %s", prefix)
	}
	normalized := path.Clean(filepath.ToSlash(prefix))
	if normalized == "." || path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") || !filepath.IsLocal(normalized) {
		return "", fmt.Errorf("invalid OCI directory: %s", prefix)
	}
	return normalized, nil
}
