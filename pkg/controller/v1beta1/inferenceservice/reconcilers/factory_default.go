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

package reconcilers

// resolvePlatformIngressReconciler is a hook for exposing InferenceServices through platform
// networking in Standard (raw) deployment mode. Distribution-specific builds (compiled with
// -tags distro) can provide their own implementation; the default does nothing.
//
// It runs before the Gateway API and Kubernetes Ingress reconcilers are considered. A non-nil
// reconciler or an error is returned from CreateIngressReconciler as is; a nil reconciler and
// nil error fall through to the upstream selection. It is not called in Knative or ModelMesh mode.
func resolvePlatformIngressReconciler(_ IngressReconcilerParams) (IngressReconciler, error) {
	return nil, nil
}
