//go:build !distro

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

// resolveModelExpressServer returns the ModelExpress server the service's engine
// pods connect to. Upstream builds read it from annotations; distribution builds
// (compiled with -tags distro) can resolve it from platform resources.
func (r *LLMISVCReconciler) resolveModelExpressServer(_ context.Context, llmSvc *v1alpha2.LLMInferenceService) (*modelExpressServer, error) {
	return modelExpressServerFromAnnotations(llmSvc)
}
