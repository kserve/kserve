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

import "github.com/kserve/kserve/mcv/pkg/cache"

const DockerfileTemplate = `FROM scratch
LABEL org.opencontainers.image.title={{ .ImageTitle }}
COPY "./{{ .CacheDir }}" "./{{ .CacheDir }}"
COPY "./{{ .ManifestDir }}/manifest.json" "./{{ .ManifestDir }}/manifest.json"
`

type DockerfileData struct {
	ImageTitle  string
	CacheDir    string
	ManifestDir string
}

type buildContext struct {
	Caches           []cache.Cache
	Labels           map[string]string
	ManifestTag      string
	CacheTag         string
	CacheBuildDir    string
	ManifestBuildDir string
	ManifestPath     string
	BuildRoot        string
}
