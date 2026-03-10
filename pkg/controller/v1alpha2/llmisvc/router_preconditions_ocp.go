//go:build distro

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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/env"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// authDisabled indicates whether auth precondition checks are disabled at runtime.
// When set to "true", the controller will skip AuthPolicy CRD validation,
// useful for environments where authentication is managed externally.
var authDisabled, _ = env.GetBool("LLMISVC_AUTH_DISABLED", false)

var authPolicyGVK = schema.GroupVersionKind{
	Group:   "kuadrant.io",
	Version: "v1",
	Kind:    "AuthPolicy",
}

// ensureGatewayPreconditions checks if Gateway on OCP can be configured correctly.
// If the AuthPolicy CRD (Red Hat Connectivity Link / Kuadrant) is not installed and auth is enabled,
// the HTTPRoute is deleted to prevent exposing the service without authentication.
//
// Returns an error both for transient failures (discovery/RBAC) and for missing CRD.
// The caller is responsible for distinguishing retryable vs non-retryable errors
// (e.g. by not propagating ErrPreconditionNotMet to avoid infinite requeue).
func (r *LLMISVCReconciler) ensureGatewayPreconditions(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("ensureGatewayPreconditions")

	if authDisabled {
		logger.V(2).Info("Auth precondition checks disabled via LLMISVC_AUTH_DISABLED")
		return nil
	}

	ok, err := utils.IsCrdAvailable(r.Config, authPolicyGVK.GroupVersion().String(), authPolicyGVK.Kind)
	if err != nil {
		return fmt.Errorf("failed to check AuthPolicy CRD availability: %w", err)
	}
	if !ok && llmSvc.IsAuthEnabled() {
		route := r.expectedHTTPRoute(ctx, llmSvc)
		if err := Delete(ctx, r, llmSvc, route); err != nil {
			return fmt.Errorf("AuthPolicy CRD is not available, please install Red Hat Connectivity Link: %w", err)
		}
		logger.Info("AuthPolicy CRD is not available, HTTPRoute deleted to prevent unauthenticated exposure")
		return fmt.Errorf("AuthPolicy CRD is not available, please install Red Hat Connectivity Link: %w", ErrPreconditionNotMet)
	}

	logger.V(2).Info("Red Hat Connectivity Link (Kuadrant) is installed or Auth is disabled", "authEnabled", llmSvc.IsAuthEnabled())

	return nil
}
