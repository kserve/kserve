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
	"fmt"

	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/utils"
)

const (
	Buildah = "buildah"
	Docker  = "docker"
)

type ImageBuilder interface {
	CreateImage(imgName string, cacheDir string) error
}

var HasApp = utils.HasApp

func New() (ImageBuilder, error) {
	if HasApp(Buildah) {
		logging.Infof("Using buildah to build the image")
		return &buildahBuilder{}, nil
	} else if HasApp(Docker) {
		logging.Infof("Using docker to build the image")
		return &dockerBuilder{}, nil
	}
	return nil, fmt.Errorf("unsupported builder: neither buildah nor docker found")
}

func NewWithBuilder(builder string) (ImageBuilder, error) {
	switch builder {
	case Buildah:
		if HasApp(Buildah) {
			logging.Infof("Using buildah to build the image")
			return &buildahBuilder{}, nil
		}
		return nil, fmt.Errorf("buildah is not available on this system")
	case Docker:
		if HasApp(Docker) {
			logging.Infof("Using docker to build the image")
			return &dockerBuilder{}, nil
		}
		return nil, fmt.Errorf("docker is not available on this system")
	case "":
		return New() // Fallback to auto-detection
	default:
		return nil, fmt.Errorf("unsupported builder: %s", builder)
	}
}
