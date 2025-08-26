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
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	RefsInvalidReason = "RefsInvalid"
)

// ValidationError represents a validation failure that should be reported via conditions
// rather than causing reconciliation to fail completely
type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

func NewValidationError(msg string) *ValidationError {
	return &ValidationError{
		message: msg,
	}
}

func IsValidationError(err error) bool {
	var validationError *ValidationError
	return errors.As(err, &validationError)
}

// validateRouterReferences performs comprehensive validation of all router-related references
// including gateway references, HTTPRoute references, managed HTTPRoute specs, and route targets.
// It handles condition marking internally and returns validation or unexpected errors.
func (r *LLMISVCReconciler) validateRouterReferences(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("validateRouterReferences")

	if err := r.validateGatewayReferences(ctx, llmSvc); err != nil {
		if IsValidationError(err) {
			llmSvc.MarkGatewaysNotReady(RefsInvalidReason, err.Error())
			logger.Info("Gateway reference validation failed", "error", err)
			return err
		}
		return fmt.Errorf("gateway reference validation failed: %w", err)
	}

	if err := r.validateHTTPRouteReferences(ctx, llmSvc); err != nil {
		if IsValidationError(err) {
			llmSvc.MarkHTTPRoutesNotReady(RefsInvalidReason, err.Error())
			logger.Info("HTTPRoute reference validation failed", "error", err)
			return err
		}
		return fmt.Errorf("HTTPRoute reference validation failed: %w", err)
	}

	if err := r.validateManagedHTTPRouteSpec(ctx, llmSvc); err != nil {
		if IsValidationError(err) {
			llmSvc.MarkHTTPRoutesNotReady(RefsInvalidReason, err.Error())
			logger.Info("Managed HTTPRoute spec validation failed", "error", err)
			return err
		}
		return fmt.Errorf("managed HTTPRoute spec validation failed: %w", err)
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Route != nil && llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		referencedRoutes, err := r.collectReferencedRoutes(ctx, llmSvc)
		if err != nil {
			return fmt.Errorf("failed to collect referenced routes for target validation: %w", err)
		}

		if err := r.validateHTTPRouteTargets(ctx, referencedRoutes); err != nil {
			if IsValidationError(err) {
				llmSvc.MarkHTTPRoutesNotReady(RefsInvalidReason, err.Error())
				logger.Info("HTTPRoute target validation failed", "error", err)
				return err
			}
			return fmt.Errorf("HTTPRoute target validation failed: %w", err)
		}
	}

	return nil
}

// validateGatewayReferences checks if all referenced gateways exist
func (r *LLMISVCReconciler) validateGatewayReferences(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("validateGatewayReferences")

	// If no router or gateway configuration, skip validation
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Gateway == nil || !llmSvc.Spec.Router.Gateway.HasRefs() {
		return nil
	}

	var missingGateways []string

	for _, ref := range llmSvc.Spec.Router.Gateway.Refs {
		gateway := &gatewayapi.Gateway{}
		gatewayKey := types.NamespacedName{
			Name:      string(ref.Name),
			Namespace: string(ref.Namespace),
		}

		if gatewayKey.Namespace == "" {
			gatewayKey.Namespace = llmSvc.GetNamespace()
		}

		err := r.Client.Get(ctx, gatewayKey, gateway)
		if err != nil {
			if apierrors.IsNotFound(err) {
				missingGateways = append(missingGateways, fmt.Sprintf("Gateway %s/%s does not exist", gatewayKey.Namespace, gatewayKey.Name))
				continue
			}
			logger.Error(err, "Error fetching Gateway", "name", gatewayKey.Name, "namespace", gatewayKey.Namespace)
			return fmt.Errorf("failed to get Gateway %s: %w", gatewayKey, err)
		}
	}

	if len(missingGateways) > 0 {
		message := strings.Join(missingGateways, "; ")
		return NewValidationError(message)
	}

	return nil
}

// validateHTTPRouteReferences checks if all referenced HTTPRoutes exist
func (r *LLMISVCReconciler) validateHTTPRouteReferences(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	// If no router or route configuration, skip validation
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		return nil
	}

	var missingRoutes []string

	for _, routeRef := range llmSvc.Spec.Router.Route.HTTP.Refs {
		route := &gatewayapi.HTTPRoute{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: routeRef.Name}, route); err != nil {
			if apierrors.IsNotFound(err) {
				missingRoutes = append(missingRoutes, fmt.Sprintf("HTTPRoute %s/%s does not exist", llmSvc.GetNamespace(), routeRef.Name))
				continue
			}
			return fmt.Errorf("failed to get HTTPRoute %s/%s: %w", llmSvc.GetNamespace(), routeRef.Name, err)
		}
	}

	if len(missingRoutes) > 0 {
		message := strings.Join(missingRoutes, "; ")
		return NewValidationError(message)
	}

	return nil
}

// validateHTTPRouteTargets checks if referenced HTTPRoutes properly target the inference service
func (r *LLMISVCReconciler) validateHTTPRouteTargets(ctx context.Context, routes []*gatewayapi.HTTPRoute) error {
	logger := log.FromContext(ctx).WithName("validateHTTPRouteTargets")

	var targetErrors []string

	for _, route := range routes {
		if len(route.Spec.ParentRefs) == 0 {
			targetErrors = append(targetErrors, fmt.Sprintf("HTTPRoute %s/%s has no parent gateway references", route.Namespace, route.Name))
			continue
		}

		for _, parentRef := range route.Spec.ParentRefs {
			gatewayKey := types.NamespacedName{
				Name: string(parentRef.Name),
			}

			if parentRef.Namespace != nil {
				gatewayKey.Namespace = string(*parentRef.Namespace)
			} else {
				gatewayKey.Namespace = route.Namespace
			}

			gateway := &gatewayapi.Gateway{}
			err := r.Client.Get(ctx, gatewayKey, gateway)
			if err != nil {
				if apierrors.IsNotFound(err) {
					targetErrors = append(targetErrors, fmt.Sprintf("HTTPRoute %s/%s references non-existent Gateway %s/%s", route.Namespace, route.Name, gatewayKey.Namespace, gatewayKey.Name))
				} else {
					logger.Error(err, "Error fetching parent Gateway", "route", route.Name, "gateway", gatewayKey)
					targetErrors = append(targetErrors, fmt.Sprintf("HTTPRoute %s/%s failed to validate parent Gateway %s/%s: %v", route.Namespace, route.Name, gatewayKey.Namespace, gatewayKey.Name, err))
				}
			}
		}
	}

	if len(targetErrors) > 0 {
		message := strings.Join(targetErrors, "; ")
		return NewValidationError("HTTPRoute target validation failed: " + message)
	}

	return nil
}

// validateManagedHTTPRouteSpec checks if managed HTTPRoute spec has valid parent gateway references
func (r *LLMISVCReconciler) validateManagedHTTPRouteSpec(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("validateManagedHTTPRouteSpec")

	// Only validate if there's a managed HTTPRoute spec
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route == nil || !llmSvc.Spec.Router.Route.HTTP.HasSpec() {
		return nil
	}

	var targetErrors []string
	httpSpec := llmSvc.Spec.Router.Route.HTTP.Spec

	for _, parentRef := range httpSpec.ParentRefs {
		gatewayKey := types.NamespacedName{
			Name: string(parentRef.Name),
		}

		if parentRef.Namespace != nil {
			gatewayKey.Namespace = string(*parentRef.Namespace)
		} else {
			gatewayKey.Namespace = llmSvc.GetNamespace()
		}

		gateway := &gatewayapi.Gateway{}
		err := r.Client.Get(ctx, gatewayKey, gateway)
		if err != nil {
			if apierrors.IsNotFound(err) {
				targetErrors = append(targetErrors, fmt.Sprintf("Managed HTTPRoute references non-existent Gateway %s/%s", gatewayKey.Namespace, gatewayKey.Name))
			} else {
				logger.Error(err, "Error fetching parent Gateway for managed route", "gateway", gatewayKey)
				return fmt.Errorf("failed to validate parent Gateway %s/%s: %w", gatewayKey.Namespace, gatewayKey.Name, err)
			}
		}
	}

	if len(targetErrors) > 0 {
		message := strings.Join(targetErrors, "; ")
		return NewValidationError(message)
	}

	return nil
}
