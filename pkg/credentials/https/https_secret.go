/*
Copyright 2021 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
)

// Create constants -- baseURI
const (
	HTTPSHostURI = "https-base-uri"
	HEADERS      = "headers"
	HEADER       = "header"
	NEWLINE      = "\n"
)

var (
	InferenceServiceHTTPSHostURI = constants.KFServingAPIGroupName + "/" + HTTPSHostURI
	HeaderPrefix                 = HEADER + "."
	CommaSeparator               = ","
	ColonSeparator               = ": "
)

// Can be used for http and https uris
func BuildSecretEnvs(secret *v1.Secret) []v1.EnvVar {
	var fieldKeys []string
	envs := []v1.EnvVar{}
	hostURI, ok := secret.Annotations[InferenceServiceHTTPSHostURI]

	if !ok {
		return envs
	}

	for key, element := range secret.Data {
		if key == HEADERS {
			// Headers will be stored in multi-lined string
			headersKeyValue := strings.Split(string(element), NEWLINE)
			for _, headerKeyValue := range headersKeyValue {
				res := strings.Split(headerKeyValue, ColonSeparator)
				if len(res) != 2 {
					continue
				}
				headerKey, headerValue := HeaderPrefix+res[0], res[1]

				fieldKeys = append(fieldKeys, headerKey)
				envs = append(envs, v1.EnvVar{
					Name:  headerKey,
					Value: headerValue,
				})
			}
		}
	}

	if len(envs) > 0 {
		envs = append(envs, v1.EnvVar{
			Name:  hostURI,
			Value: strings.Join(fieldKeys, CommaSeparator),
		})
	}

	return envs
}
