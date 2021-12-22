/*
Copyright 2021 The KServe Authors.

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

package https

import (
	v1 "k8s.io/api/core/v1"
)

// Create constants -- baseURI
const (
	HTTPSHost = "https-host"
	HEADERS   = "headers"
	NEWLINE   = "\n"
)

var (
	HeadersSuffix  = "-" + HEADERS
	ColonSeparator = ": "
)

// Can be used for http and https uris
func BuildSecretEnvs(secret *v1.Secret) []v1.EnvVar {
	envs := []v1.EnvVar{}
	uriHost, ok := secret.Data[HTTPSHost]

	if !ok {
		return envs
	}

	headers, ok := secret.Data[HEADERS]

	if !ok {
		return envs
	}

	envs = append(envs, v1.EnvVar{
		Name:  string(uriHost) + HeadersSuffix,
		Value: string(headers),
	})

	return envs
}
