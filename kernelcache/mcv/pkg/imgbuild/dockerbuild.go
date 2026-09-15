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
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	logging "github.com/sirupsen/logrus"
)

type dockerBuilder struct{}

// CreateImage loads a compat cache image into the Docker daemon using Docker
// Schema 2 media types throughout. BuildKit's docker build path can produce an
// OCI manifest with Docker layer types, which breaks docker save and kind load.
func (d *dockerBuilder) CreateImage(imageName, cacheDir string) error {
	prep, err := prepareBuildContext("docker", cacheDir)
	if err != nil {
		return err
	}
	defer CleanupDirs(prep.CacheBuildDir, prep.ManifestBuildDir)

	imageWithTag := NormalizeImageTag(imageName)
	tag, err := name.NewTag(imageWithTag)
	if err != nil {
		return fmt.Errorf("invalid image reference %q: %w", imageWithTag, err)
	}

	img, err := schema2ImageFromBuildContext(prep, imageName)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	if prep.TempLayerFile != "" {
		defer os.Remove(prep.TempLayerFile)
	}

	ctx := context.Background()
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	if err := loadImageIntoDocker(ctx, apiClient, tag, img); err != nil {
		return err
	}
	logging.Info("Docker image built successfully")

	if err := CleanupWithTimeout(); err != nil {
		return fmt.Errorf("cleanup error: %w", err)
	}
	return nil
}

func loadImageIntoDocker(ctx context.Context, apiClient *client.Client, tag name.Tag, img v1.Image) error {
	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		pw.CloseWithError(tarball.Write(tag, img, pw))
	}()

	resp, err := apiClient.ImageLoad(ctx, pr)
	if err != nil {
		return fmt.Errorf("failed to load image into Docker: %w", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("failed to read Docker load response: %w", err)
	}
	return nil
}
