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

package inferenceservice

import (
	"context"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// reconcilePlatformInferenceService is a hook for platform-specific InferenceService policy that
// has to be resolved before any component is reconciled. Distribution-specific builds (compiled
// with -tags distro) can provide their own implementation; the default does nothing.
//
// It runs once the finalizer is registered and status conditions are initialized. It is not called
// for InferenceServices being deleted or for ModelMesh ones without a transformer. When
// reconciliationPaused is true the components are not reconciled, and status is persisted only if
// the hook changed it. Errors are returned as is, without persisting status.
//
// The returned context replaces ctx for the rest of the reconcile. Workload customization hooks
// such as customizeDeployments only receive component metadata, so values resolved here from the
// whole InferenceService reach them through the context. It must be ctx or derived from it with
// context.WithValue, never nil, detached or with a new deadline. Those hooks also run for
// InferenceGraphs, which never pass through here, so a missing value means no policy.
func (r *InferenceServiceReconciler) reconcilePlatformInferenceService(ctx context.Context,
	_ *v1beta1.InferenceService, _ constants.DeploymentModeType, _ bool,
) (context.Context, error) {
	return ctx, nil
}
