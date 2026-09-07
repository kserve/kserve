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
	"io"

	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/docker/docker/client"
)

type DockerClient interface {
	ImageSave(ctx context.Context, images []string, options ...client.ImageSaveOption) (io.ReadCloser, error)
	Close() error
}

type PodmanClient interface {
	Export(ctx context.Context, names []string, w io.Writer, opts *images.ExportOptions) error
	Exists(ctx context.Context, name string, opts *images.ExistsOptions) (bool, error)
}
