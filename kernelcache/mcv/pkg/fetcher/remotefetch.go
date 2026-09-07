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

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	logging "github.com/sirupsen/logrus"
)

type remoteFetcher struct{}

func (r *remoteFetcher) FetchImg(imgName string) (v1.Image, error) {
	// Parse the image name into a reference (e.g., quay.io/tkm/triton-cache)
	ref, err := name.ParseReference(imgName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image name: %w", err)
	}

	logging.Debugf("Retrieve remote Img %s!!!!!!!!", imgName)
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	// Print the image details
	logging.Debug("Img fetched successfully!!!!!!!!")
	return img, nil
}
