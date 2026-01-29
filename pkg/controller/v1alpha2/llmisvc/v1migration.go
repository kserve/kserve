/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// inferencePoolMigratedValueV1 is the annotation value indicating migration to v1 is complete
const inferencePoolMigratedValueV1 = "v1"

// InferencePoolV1Alpha2UnsupportedAnnotationKey is set when the Gateway rejects v1alpha2 backendRefs.
const InferencePoolV1Alpha2UnsupportedAnnotationKey = constants.KServeAPIGroupName + "/inferencepool-v1alpha2-unsupported"

// isMigratedToV1 checks if the LLMInferenceService has completed migration to v1 InferencePool.
// Migration status is tracked via the inferencepool-migrated annotation.
func isMigratedToV1(llmSvc *v1alpha2.LLMInferenceService) bool {
	if llmSvc == nil || llmSvc.Annotations == nil {
		return false
	}
	return llmSvc.Annotations[constants.InferencePoolMigratedAnnotationKey] == inferencePoolMigratedValueV1
}

// setMigratedToV1 sets the migration annotation indicating v1 migration is complete.
// This acts as a one-way lock to prevent flapping back to v1alpha2.
func setMigratedToV1(llmSvc *v1alpha2.LLMInferenceService) {
	if llmSvc.Annotations == nil {
		llmSvc.Annotations = make(map[string]string)
	}
	llmSvc.Annotations[constants.InferencePoolMigratedAnnotationKey] = inferencePoolMigratedValueV1
}

// getActivePoolAPIGroup returns the API group of the active InferencePool.
// Returns v1 if migrated, v1alpha2 otherwise.
func getActivePoolAPIGroup(llmSvc *v1alpha2.LLMInferenceService) string {
	if isMigratedToV1(llmSvc) {
		return constants.InferencePoolV1APIGroupName
	}
	return constants.InferencePoolV1Alpha2APIGroupName
}

// isV1Alpha2Unsupported checks if the Gateway has rejected v1alpha2 backendRefs.
func isV1Alpha2Unsupported(llmSvc *v1alpha2.LLMInferenceService) bool {
	if llmSvc == nil || llmSvc.Annotations == nil {
		return false
	}
	return llmSvc.Annotations[InferencePoolV1Alpha2UnsupportedAnnotationKey] == "true"
}

// setV1Alpha2Unsupported marks that the Gateway doesn't support v1alpha2 InferencePool.
// Once set, the controller will always use v1 backendRefs.
func setV1Alpha2Unsupported(llmSvc *v1alpha2.LLMInferenceService) {
	if llmSvc.Annotations == nil {
		llmSvc.Annotations = make(map[string]string)
	}
	llmSvc.Annotations[InferencePoolV1Alpha2UnsupportedAnnotationKey] = "true"
}
