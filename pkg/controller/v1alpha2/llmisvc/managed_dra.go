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

// Package llmisvc contains the controller logic for the LLMInferenceService.
//
// Note on Managed DRA:
// The Managed DRA feature implemented here via the serving.kserve.io/exp-dra-* annotations
// is an intentionally limited-scope convenience feature for basic DRA use cases.
// It provides a simplified mechanism for users to dynamically request accelerator
// devices (GPUs, TPUs, NICs, etc.) via DeviceClass, avoiding the need to manually
// create and manage ResourceClaimTemplate objects.
//
// Complex DRA topologies or advanced use cases should bypass this managed feature
// and use native Kubernetes ResourceClaimTemplate objects directly by attaching
// them to the PodSpec.
package llmisvc

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	managedDRASuffix      = "-managed-dra"
	managedDRAClaimName   = "managed-device"
	defaultManagedDRAName = "device"
)

// reconcileManagedDRA creates, updates, or deletes the ResourceClaimTemplate
// that backs managed DRA for this LLMInferenceService.
func (r *LLMISVCReconciler) reconcileManagedDRA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if !llmSvc.HasManagedDRA() || utils.GetForceStopRuntime(llmSvc) {
		return r.cleanupManagedDRA(ctx, llmSvc)
	}

	expected, err := expectedManagedDRATemplate(llmSvc)
	if err != nil {
		return err
	}

	if err := Reconcile(ctx, r, llmSvc, &resourcev1.ResourceClaimTemplate{}, expected, semanticResourceClaimTemplateIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile Managed DRA ResourceClaimTemplate: %w", err)
	}

	return nil
}

// cleanupManagedDRA removes any previously generated ResourceClaimTemplate
// owned by this LLMInferenceService.
func (r *LLMISVCReconciler) cleanupManagedDRA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	stale := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedDRAResourceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
		},
	}
	if err := Delete(ctx, r, llmSvc, stale); err != nil {
		return fmt.Errorf("failed to cleanup Managed DRA ResourceClaimTemplate: %w", err)
	}
	return nil
}

func expectedManagedDRATemplate(llmSvc *v1alpha2.LLMInferenceService) (*resourcev1.ResourceClaimTemplate, error) {
	deviceClass, _ := llmSvc.ManagedDRADeviceClass()
	celSelectors := llmSvc.ManagedDRACelSelectors()

	deviceCount, err := llmSvc.ManagedDRADeviceCount()
	if err != nil {
		return nil, err
	}

	return &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedDRAResourceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: resourcev1.ResourceClaimTemplateSpec{
			Spec: resourcev1.ResourceClaimSpec{
				Devices: resourcev1.DeviceClaim{
					Requests: buildDeviceRequests(deviceClass, celSelectors, deviceCount),
				},
			},
		},
	}, nil
}

// buildDeviceRequests creates the slice of DeviceRequest objects based on the requested count and class.
func buildDeviceRequests(deviceClass string, celSelectors []string, deviceCount int) []resourcev1.DeviceRequest {
	req := resourcev1.DeviceRequest{
		Name: defaultManagedDRAName,
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: deviceClass,
		},
	}

	if deviceCount > 1 {
		req.Exactly.Count = int64(deviceCount)
	}

	if len(celSelectors) > 0 {
		selectors := make([]resourcev1.DeviceSelector, len(celSelectors))
		for j, expr := range celSelectors {
			selectors[j] = resourcev1.DeviceSelector{
				CEL: &resourcev1.CELDeviceSelector{
					Expression: expr,
				},
			}
		}
		req.Exactly.Selectors = selectors
	}

	return []resourcev1.DeviceRequest{req}
}

func semanticResourceClaimTemplateIsEqual(expected *resourcev1.ResourceClaimTemplate, curr *resourcev1.ResourceClaimTemplate) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec)
}

// managedDRAResourceName returns the name of the ResourceClaimTemplate
func managedDRAResourceName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), managedDRASuffix)
}

// injectManagedDRAIntoConfig fans injectManagedDRA out to every workload
// PodSpec of the merged config (Template, Worker, Prefill.Template, Prefill.Worker).
func injectManagedDRAIntoConfig(llmSvc *v1alpha2.LLMInferenceService, cfg *v1alpha2.LLMInferenceServiceConfig) {
	if cfg == nil || !llmSvc.HasManagedDRA() {
		return
	}
	if cfg.Spec.Template != nil {
		injectManagedDRA(llmSvc, cfg.Spec.Template)
	}
	if cfg.Spec.Worker != nil {
		injectManagedDRA(llmSvc, cfg.Spec.Worker)
	}
	if cfg.Spec.Prefill != nil {
		if cfg.Spec.Prefill.Template != nil {
			injectManagedDRA(llmSvc, cfg.Spec.Prefill.Template)
		}
		if cfg.Spec.Prefill.Worker != nil {
			injectManagedDRA(llmSvc, cfg.Spec.Prefill.Worker)
		}
	}
}

// injectManagedDRA adds the managed DRA claim to the PodSpec at both the
// pod level and the target container chosen by targetContainerForDRA.
func injectManagedDRA(llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if !llmSvc.HasManagedDRA() {
		return
	}

	resourceName := managedDRAResourceName(llmSvc)

	hasPodClaim := false
	for _, claim := range podSpec.ResourceClaims {
		if claim.Name == managedDRAClaimName {
			hasPodClaim = true
			break
		}
	}
	if !hasPodClaim {
		podSpec.ResourceClaims = append(podSpec.ResourceClaims, corev1.PodResourceClaim{
			Name:                      managedDRAClaimName,
			ResourceClaimTemplateName: &resourceName,
		})
	}

	idx := targetContainerForDRA(llmSvc, podSpec)
	if idx < 0 {
		return
	}
	for _, claim := range podSpec.Containers[idx].Resources.Claims {
		if claim.Name == managedDRAClaimName {
			return
		}
	}
	podSpec.Containers[idx].Resources.Claims = append(
		podSpec.Containers[idx].Resources.Claims,
		corev1.ResourceClaim{Name: managedDRAClaimName},
	)
}

// targetContainerForDRA returns the index of the container that should receive
// the DRA claim: the annotated container when set and present, otherwise the
// first container. Returns -1 if no suitable container exists.
func targetContainerForDRA(llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) int {
	if len(podSpec.Containers) == 0 {
		return -1
	}
	want, present := llmSvc.ManagedDRAContainerName()
	if !present || want == "" {
		return 0
	}
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == want {
			return i
		}
	}
	return -1
}
