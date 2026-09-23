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

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/registryauth"
)

type remoteFetcher struct{}

func (r *remoteFetcher) FetchImg(imgName string) (v1.Image, error) {
	// Parse the image name into a reference (e.g., quay.io/gkm/triton-cache)
	ref, err := name.ParseReference(imgName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image name: %w", err)
	}

	logging.Debugf("Retrieve remote Img %s", imgName)
	options, err := registryauth.RemoteOptions(context.Background(), ref.Context().RegistryStr())
	if err != nil {
		return nil, fmt.Errorf("failed to configure registry access: %w", err)
	}
	img, err := remote.Image(ref, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	// Print the image details
	logging.Debug("Img fetched successfully")
	return img, nil
}
