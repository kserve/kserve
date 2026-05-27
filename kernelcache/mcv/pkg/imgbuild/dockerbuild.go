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

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	logging "github.com/sirupsen/logrus"
)

type dockerBuilder struct{}

// Docker implementation of the ImageBuilder interface.
func (d *dockerBuilder) CreateImage(imageName, cacheDir string) error {
	prep, err := prepareBuildContext("docker", cacheDir)
	if err != nil {
		return err
	}
	defer CleanupDirs(prep.CacheBuildDir, prep.ManifestBuildDir)

	dockerfilePath := DockerfilePath(prep.BuildRoot)

	err = GenerateDockerfile(imageName, prep.CacheTag, prep.ManifestTag, dockerfilePath)
	if err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}
	defer os.Remove(dockerfilePath)

	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	//nolint:staticcheck // SA1019: archive.TarWithOptions is deprecated but no alternative exists
	tar, err := archive.TarWithOptions(prep.BuildRoot, &archive.TarOptions{IncludeSourceDir: false})
	if err != nil {
		return fmt.Errorf("error creating tar: %w", err)
	}
	defer tar.Close()

	buildOptions := build.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Tags:       []string{imageName},
		NoCache:    true,
		Remove:     false,
		Labels:     prep.Labels,
	}

	buildResponse, err := apiClient.ImageBuild(context.Background(), tar, buildOptions)
	if err != nil {
		return fmt.Errorf("error building image: %w", err)
	}
	defer buildResponse.Body.Close()

	_, err = io.Copy(os.Stdout, buildResponse.Body)
	if err != nil {
		return fmt.Errorf("error reading build output: %w", err)
	}

	imageWithTag := NormalizeImageTag(imageName)

	err = apiClient.ImageTag(context.Background(), imageName, imageWithTag)
	if err != nil {
		return fmt.Errorf("error tagging image: %w", err)
	}
	logging.Info("Docker image built successfully")

	// Cleanup
	if err := CleanupWithTimeout(); err != nil {
		return fmt.Errorf("cleanup error: %w", err)
	}
	return nil
}
