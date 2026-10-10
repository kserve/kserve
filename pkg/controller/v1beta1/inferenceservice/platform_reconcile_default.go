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

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// The hooks below are no-ops upstream. Distribution-specific builds (compiled with -tags distro)
// can provide their own implementations. Errors returned by the hooks are passed through
// unwrapped and without persisting status, so implementations own the error text.

// preReconcilePlatform is a hook for platform-specific state that has to be resolved before any
// component is reconciled.
//
// It runs once finalization is handled and status conditions are initialized. It is not called
// for InferenceServices being deleted (see finalizePlatform) or for ModelMesh ones without a
// transformer. When reconciliationPaused is true the components are not reconciled, and status is
// persisted only if the hook changed it.
//
// The returned context replaces ctx for the rest of the reconcile. Workload customization hooks
// such as customizeDeployments only receive component metadata, so values resolved here from the
// whole InferenceService reach them through the context. It must be ctx or derived from it with
// context.WithValue, never nil, detached or with a new deadline. Those hooks also run for
// InferenceGraphs, which never pass through here, so a missing value means no policy.
func (r *InferenceServiceReconciler) preReconcilePlatform(ctx context.Context,
	_ *v1beta1.InferenceService, _ constants.DeploymentModeType, _ bool,
) (context.Context, error) {
	return ctx, nil
}

// postReconcilePlatform is a hook for platform-specific state that depends on the outcome of
// component reconciliation, and receives the inferenceservice-config ConfigMap.
//
// It runs once all components are reconciled, so the serving runtime selected for the predictor
// is recorded in status, and before ingress. It is not called when a component fails or requests
// a requeue, or when reconciliation is paused.
func (r *InferenceServiceReconciler) postReconcilePlatform(_ context.Context, _ *v1beta1.InferenceService, _ *corev1.ConfigMap) error {
	return nil
}

// finalizePlatform is a hook for cleaning up platform state that owner references cannot
// garbage-collect, e.g. cluster-scoped objects shared across InferenceServices. It pairs with
// preReconcilePlatform, which does not run for InferenceServices being deleted.
//
// It runs while the InferenceService finalizer is present, once the upstream external resources
// are deleted. An error keeps the finalizer, so the cleanup is retried on the next reconcile.
func (r *InferenceServiceReconciler) finalizePlatform(_ context.Context, _ *v1beta1.InferenceService) error {
	return nil
}
