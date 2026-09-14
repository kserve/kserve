package imgbuild

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const imageTitleLabel = "org.opencontainers.image.title"

// imageTitleFromName returns the repository component used for
// org.opencontainers.image.title, matching GenerateDockerfile.
func imageTitleFromName(imageName string) string {
	parts := strings.Split(imageName, "/")
	fullImageName := parts[len(parts)-1]
	return strings.Split(fullImageName, ":")[0]
}

// schema2ImageFromBuildContext builds a Docker Schema 2 image from the staged
// MCV build context. BuildKit often emits an OCI manifest with Docker layer
// media types, which breaks docker save (and therefore kind load). Loading a
// consistent Schema 2 image avoids that hybrid manifest.
func schema2ImageFromBuildContext(prep *buildContext, imageName string) (v1.Image, error) {
	layer, err := compatLayerFromBuildContext(prep)
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string, len(prep.Labels)+1)
	for k, v := range prep.Labels {
		labels[k] = v
	}
	labels[imageTitleLabel] = imageTitleFromName(imageName)

	now := time.Now().UTC()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer:     layer,
		MediaType: types.DockerLayer,
		History: v1.History{
			Created:   v1.Time{Time: now},
			CreatedBy: "mcv",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to append compat layer: %w", err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read image config: %w", err)
	}
	cfg.Created = v1.Time{Time: now}
	cfg.OS = "linux"
	cfg.Architecture = runtime.GOARCH
	cfg.Config.Labels = labels

	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to update image config: %w", err)
	}

	img = mutate.MediaType(img, types.DockerManifestSchema2)
	img = mutate.ConfigMediaType(img, types.DockerConfigJSON)
	return img, nil
}

func compatLayerFromBuildContext(prep *buildContext) (v1.Layer, error) {
	f, err := os.CreateTemp("", "mcv-layer-*.tar")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp layer file: %w", err)
	}
	tmpPath := f.Name()

	if err := writeCompatLayerTar(f, prep.CacheBuildDir, prep.ManifestBuildDir, prep.CacheTag, prep.ManifestTag); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to close temp layer file: %w", err)
	}

	prep.TempLayerFile = tmpPath
	return tarball.LayerFromFile(tmpPath)
}

func writeCompatLayerTar(w io.Writer, cacheDir, manifestDir, cacheTag, manifestTag string) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	if err := appendTreeToTar(tw, cacheDir, cacheTag); err != nil {
		return fmt.Errorf("failed to tar cache directory: %w", err)
	}
	if err := appendTreeToTar(tw, manifestDir, manifestTag); err != nil {
		return fmt.Errorf("failed to tar manifest directory: %w", err)
	}
	return nil
}

func appendTreeToTar(tw *tar.Writer, srcDir, prefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		if info.IsDir() {
			header.Name += "/"
		}

		if writeErr := tw.WriteHeader(header); writeErr != nil {
			return writeErr
		}
		if info.Mode()&os.ModeType != 0 || info.IsDir() {
			return nil
		}

		var file *os.File
		file, err = os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}
