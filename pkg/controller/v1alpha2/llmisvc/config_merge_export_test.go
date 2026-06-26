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

// ConfigNotFoundError exposes the package-internal configNotFoundError type
// to the llmisvc_test black-box package so the Error()-format unit tests
// can construct fixtures directly, without leaking the type into the
// production API surface.
type ConfigNotFoundError = configNotFoundError

// ListAvailableConfigsForTest exposes the package-internal
// listAvailableConfigs helper to the llmisvc_test black-box package so
// the best-effort-skip-failing-namespace case (which needs per-namespace
// LIST error injection that envtest can't easily reproduce) can be
// exercised via a fake client + interceptor.Funcs.
func (r *LLMISVCReconciler) ListAvailableConfigsForTest(ctx context.Context, namespaces []string) []string {
	return r.listAvailableConfigs(ctx, namespaces)
}
