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

package llmisvc

import (
	"context"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// SetUseVersionedConfigForTest overrides the useVersionedConfig flag for testing
// and returns a cleanup function that restores the original value.
func SetUseVersionedConfigForTest(enabled bool) func() {
	original := useVersionedConfig
	useVersionedConfig = enabled
	return func() {
		useVersionedConfig = original
	}
}

// SelectSingleNodeTemplateName exposes selectSingleNodeTemplateName for testing.
var SelectSingleNodeTemplateName = selectSingleNodeTemplateName

// ResolveRuntimeSpec exposes (*LLMISVCReconciler).resolveRuntimeSpec for testing.
func (r *LLMISVCReconciler) ResolveRuntimeSpec(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*v1alpha2.LLMInferenceServiceSpec, error) {
	return r.resolveRuntimeSpec(ctx, llmSvc)
}
