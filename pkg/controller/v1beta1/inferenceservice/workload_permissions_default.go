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
)

// reconcileWorkloadPlatformPermissions is a hook for platform-specific permissions the
// InferenceService workloads need, e.g. bindings for their service accounts. Distribution-specific
// builds (compiled with -tags distro) can provide their own implementation; the default does nothing.
// It runs once all components are reconciled, so the selected serving runtime is recorded in
// status, and receives the inferenceservice-config ConfigMap. Status is not persisted when it fails.
func (r *InferenceServiceReconciler) reconcileWorkloadPlatformPermissions(_ context.Context, _ *v1beta1.InferenceService, _ *corev1.ConfigMap) error {
	return nil
}
