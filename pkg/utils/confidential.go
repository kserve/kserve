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

package utils

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

// ApplyConfidentialContainerConfig configures a container for confidential model serving
// by injecting the required environment variables.
// This is shared between the InferenceService webhook (annotation-driven) and the
// LLMInferenceService controller (spec-driven).
func ApplyConfidentialContainerConfig(container *corev1.Container, resourceId string) {
	AddOrReplaceEnv(container, constants.ConfidentialEnabledEnvVar, "true")

	if resourceId != "" {
		AddOrReplaceEnv(container, constants.ConfidentialResourceIdEnvVar, resourceId)
	}
}
