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
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/client"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	logging "github.com/sirupsen/logrus"
)

type dockerFetcher struct {
	client DockerClient
}

// newDockerFetcher creates a new instance of dockerFetcher with a Docker API client.
// It initializes the client using environment variables and enables API version negotiation.
// Returns a pointer to dockerFetcher and an error if the client creation fails.
func newDockerFetcher() (*dockerFetcher, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &dockerFetcher{client: apiClient}, nil
}

func (d *dockerFetcher) FetchImg(imgName string) (v1.Image, error) {
	logging.Debugf("Saving Docker image: %s", imgName)

	imageFunc := func(w io.Writer) error {
		reader, err := d.client.ImageSave(context.Background(), []string{imgName})
		if err != nil {
			return fmt.Errorf("failed to save image: %w", err)
		}
		defer reader.Close()
		_, err = io.Copy(w, reader)
		return err
	}

	return fetchToTempTar(imageFunc)
}

var _ Fetcher = (*dockerFetcher)(nil)
