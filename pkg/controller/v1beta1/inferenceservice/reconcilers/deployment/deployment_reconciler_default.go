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

package deployment

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// customizeDeployments is the default (upstream) no-op hook for platform-specific
// customization of the desired Deployments in r.DeploymentList. It runs once the
// Deployments are built, before they are reconciled against the cluster.
func (r *DeploymentReconciler) customizeDeployments(_ context.Context, _ metav1.ObjectMeta, _ *corev1.PodSpec) error {
	return nil
}
