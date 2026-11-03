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

package validation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

var DefaultBlockedEnvVars = []string{
	"PYTHONPATH",
}

func ValidateBlockedEnvVars(containers []corev1.Container, blockedVars []string) error {
	blocked := make(map[string]bool, len(blockedVars))
	for _, v := range blockedVars {
		blocked[v] = true
	}

	for _, container := range containers {
		for _, env := range container.Env {
			if blocked[env.Name] {
				return fmt.Errorf("setting %s in container %q is not allowed for security reasons", env.Name, container.Name)
			}
		}
	}
	return nil
}
