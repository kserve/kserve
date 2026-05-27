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
	"os"
	"os/user"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/images"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	logging "github.com/sirupsen/logrus"
)

type podmanFetcher struct {
	client PodmanClient
}

func newPodmanFetcher() (*podmanFetcher, error) {
	socket := getPodmanSock()
	if socket == "" {
		return nil, fmt.Errorf("could not determine Podman socket")
	}

	ctx, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to podman: %w", err)
	}

	return &podmanFetcher{client: &realPodmanClient{ctx: ctx}}, nil
}

func (p *podmanFetcher) FetchImg(imgName string) (v1.Image, error) {
	logging.Debugf("Checking for image: %s via Podman", imgName)

	found, err := p.client.Exists(context.Background(), imgName, &images.ExistsOptions{})
	if err != nil || !found {
		return nil, fmt.Errorf("podman image not found: %w", err)
	}

	imageFunc := func(w io.Writer) error {
		var compress = true
		var format = "docker-archive"
		return p.client.Export(context.Background(), []string{imgName}, w, &images.ExportOptions{
			Compress: &compress,
			Format:   &format,
		})
	}

	return fetchToTempTar(imageFunc)
}

func getPodmanSock() string {
	// Try default rootful
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		return "unix:///run/podman/podman.sock"
	}

	// Try rootless
	usr, err := user.Current()
	if err != nil {
		return ""
	}
	sock := fmt.Sprintf("/run/user/%s/podman/podman.sock", usr.Uid)
	if _, err := os.Stat(sock); err == nil {
		return "unix://" + sock
	}
	return ""
}

type realPodmanClient struct {
	ctx context.Context
}

func (r *realPodmanClient) Exists(ctx context.Context, name string, opts *images.ExistsOptions) (bool, error) {
	return images.Exists(r.ctx, name, opts)
}

func (r *realPodmanClient) Export(ctx context.Context, names []string, w io.Writer, opts *images.ExportOptions) error {
	return images.Export(r.ctx, names, w, opts)
}

var _ Fetcher = (*podmanFetcher)(nil)
