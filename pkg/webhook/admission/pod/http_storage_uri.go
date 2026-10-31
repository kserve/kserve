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

package pod

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

// injectHTTPStorageURIAllowlist applies the administrator policy after container
// customizations and credentials so inherited environment variables cannot widen it.
func injectHTTPStorageURIAllowlist(container *corev1.Container, allowlist []string) error {
	if allowlist == nil {
		allowlist = []string{}
	}
	allowlistJSON, err := json.Marshal(allowlist)
	if err != nil {
		return fmt.Errorf("failed to encode HTTP storage URI allowlist: %w", err)
	}
	policyEnv := corev1.EnvVar{Name: constants.HTTPStorageURIAllowlistEnvVar, Value: string(allowlistJSON)}
	policyEnvSet := false
	for i := range container.Env {
		if container.Env[i].Name == policyEnv.Name {
			container.Env[i] = policyEnv
			policyEnvSet = true
		}
	}
	if !policyEnvSet && len(allowlist) > 0 {
		container.Env = append(container.Env, policyEnv)
	}
	return nil
}
