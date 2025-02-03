/*
Copyright 2022 The KServe Authors.

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

package inferencegraph

import (
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func buildEnvVars(spec v1alpha1api.InferenceGraphSpec, config *RouterConfig) []v1.EnvVar {
	timeouts := map[string]*int64{
		"SERVER_READ_TIMEOUT_SECONDS":    spec.ServerReadTimeoutSeconds,
		"SERVER_WRITE_TIMEOUT_SECONDS":   spec.ServerWriteTimeoutSeconds,
		"SERVER_IDLE_TIMEOUT_SECONDS":    spec.ServerIdleTimeoutSeconds,
		"CLIENT_SERVICE_TIMEOUT_SECONDS": spec.ClientServiceTimeoutSeconds,
	}

	var envVars []v1.EnvVar
	for name, value := range timeouts {
		if value != nil {
			envVars = append(envVars, v1.EnvVar{
				Name:  name,
				Value: strconv.FormatInt(*value, 10),
			})
		}
	}

	// Only adding this env variable "PROPAGATE_HEADERS" if router's headers config has the key "propagate"
	if headers, exists := config.Headers["propagate"]; exists {
		envVars = append(envVars, v1.EnvVar{
			Name:  constants.RouterHeadersPropagateEnvVar,
			Value: strings.Join(headers, ","),
		})
	}

	return envVars
}
