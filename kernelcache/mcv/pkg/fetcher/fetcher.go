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

package fetcher

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/constants"
	"github.com/kserve/kserve/mcv/pkg/utils"
)

type Fetcher interface {
	FetchImg(imgName string) (v1.Image, error)
}

type fetcher struct {
	local  []Fetcher
	remote Fetcher
}

// Factory function to create a new Fetcher with the specified backend.
func NewFetcher() Fetcher {
	var localFetchers []Fetcher

	addFetcher := func(fetcher Fetcher, err error) {
		if err == nil {
			localFetchers = append(localFetchers, fetcher)
		} else {
			logging.Debugf("Failed to init fetcher: %v", err)
		}
	}

	if utils.HasApp("docker") {
		addFetcher(newDockerFetcher())
	}
	if utils.HasApp("podman") {
		addFetcher(newPodmanFetcher())
	}

	return &fetcher{local: localFetchers, remote: &remoteFetcher{}}
}

func (f *fetcher) FetchImg(imgName string) (v1.Image, error) {
	// Try to fetch locally first
	for _, localFetcher := range f.local {
		logging.Debugf("Trying local fetcher: %T", localFetcher)

		img, _ := localFetcher.FetchImg(imgName)
		if img != nil {
			logging.Debugf("Image found locally using %T", localFetcher)
			return img, nil
		}

		// If error or image is nil, log and continue to the next fetcher
		logging.Debugf("Failed to fetch image locally using %T:", localFetcher)
	}

	// If local fetch fails, try fetching the image remotely
	img, err := f.remote.FetchImg(imgName)
	if err != nil || img == nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	return img, nil
}

func fetchToTempTar(fetchFn func(io.Writer) error) (v1.Image, error) {
	tmpDir := filepath.Join(constants.MCVBuildDir, constants.CacheDir)

	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, err
	}
	logging.Debugf("cache tmp extract dir: %s", tmpDir)

	tarballFilePath := path.Join(tmpDir, "tmp.tar")
	tarballFile, err := os.Create(tarballFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create tarball file: %v", err)
	}

	if err := fetchFn(tarballFile); err != nil {
		tarballFile.Close() // Close on error too
		return nil, fmt.Errorf("error writing image to tarball: %w", err)
	}

	// Close explicitly before reading
	if err := tarballFile.Close(); err != nil {
		return nil, fmt.Errorf("error closing tarball file: %w", err)
	}

	logging.Debugf("Saved image to tarball: %s", tarballFilePath)

	return loadImageFromTarball(tarballFilePath)
}
