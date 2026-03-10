//go:build !distro

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
	"context"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// ensureGatewayPreconditions is a no-op in upstream builds.
// Distribution-specific builds (compiled with -tags distro) can provide a real
// implementation to enforce platform-specific prerequisites before the
// Gateway/HTTPRoute is reconciled.
func (r *LLMISVCReconciler) ensureGatewayPreconditions(_ context.Context, _ *v1alpha2.LLMInferenceService) error {
	return nil
}
