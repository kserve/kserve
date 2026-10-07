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

package inferencegraph

import (
	"context"

	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// The hooks below are no-ops upstream. Distribution-specific builds (compiled with -tags distro)
// can provide their own implementations. Errors returned by the hooks are passed through
// unwrapped, so implementations own the error text.

// reconcilePlatformFinalizer is a hook for managing a finalizer that guards platform state
// owner references cannot garbage-collect (e.g. shared cluster-scoped objects).
// Returning true stops this reconcile, e.g. when the graph is being deleted.
func (r *InferenceGraphReconciler) reconcilePlatformFinalizer(_ context.Context, _ *v1alpha1.InferenceGraph) (bool, error) {
	return false, nil
}

// reconcileRawPlatformPrerequisites is a hook for resources the router depends on in raw
// deployment mode (e.g. identity, permissions). It runs before the router Deployment is reconciled.
func (r *InferenceGraphReconciler) reconcileRawPlatformPrerequisites(_ context.Context, _ *v1alpha1.InferenceGraph) error {
	return nil
}

// reconcileRawPlatformNetworking is a hook for exposing the graph through platform networking
// in raw deployment mode. It runs once the router Deployment is reconciled and available, and
// returns the externally reachable URL for the graph status, which may differ from url.
// It also runs for stopped graphs, whose status URL is cleared afterwards.
func (r *InferenceGraphReconciler) reconcileRawPlatformNetworking(_ context.Context, _ *v1alpha1.InferenceGraph, url *apis.URL) (*apis.URL, error) {
	return url, nil
}

// customizeRouterKnativeService is a hook for platform-specific customization of the desired
// router Knative Service. Implementations must tolerate a nil ksvc.
func customizeRouterKnativeService(_ *v1alpha1.InferenceGraph, _ *knservingv1.Service) {}
