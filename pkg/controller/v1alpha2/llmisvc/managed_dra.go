package llmisvc

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	managedDRASuffix      = "-managed-dra"
	managedDRAClaimName   = "managed-gpu"
	defaultManagedDRAName = "gpu"
)

// hasManagedDRA checks if the LLMInferenceService has the required annotations to enable Managed DRA.
func hasManagedDRA(llmSvc *v1alpha2.LLMInferenceService) bool {
	_, ok := llmSvc.Annotations[constants.ManagedDRADeviceClassAnnotationKey]
	return ok
}

// managedDRAResourceName generates the name of the ResourceClaim or ResourceClaimTemplate.
func managedDRAResourceName(llmSvc *v1alpha2.LLMInferenceService) string {
	return llmSvc.GetName() + managedDRASuffix
}

// parseManagedDRAGpuCount extracts the number of GPUs requested from the annotations.
func parseManagedDRAGpuCount(llmSvc *v1alpha2.LLMInferenceService) (int, error) {
	raw, ok := llmSvc.Annotations[constants.ManagedDRAGpuCountAnnotationKey]
	if !ok || raw == "" {
		return 1, nil
	}
	count, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", constants.ManagedDRAGpuCountAnnotationKey, raw, err)
	}
	if count < 1 {
		return 0, fmt.Errorf("invalid %s value %q: must be >= 1", constants.ManagedDRAGpuCountAnnotationKey, raw)
	}
	return count, nil
}

// buildDeviceRequests creates the slice of DeviceRequest objects based on the requested count and class.
func buildDeviceRequests(deviceClass, celSelector string, gpuCount int) []resourcev1.DeviceRequest {
	requests := make([]resourcev1.DeviceRequest, gpuCount)
	for i := range requests {
		name := defaultManagedDRAName
		if gpuCount > 1 {
			name = fmt.Sprintf("%s-%d", defaultManagedDRAName, i+1)
		}
		req := resourcev1.DeviceRequest{
			Name: name,
			Exactly: &resourcev1.ExactDeviceRequest{
				DeviceClassName: deviceClass,
			},
		}
		if celSelector != "" {
			req.Exactly.Selectors = []resourcev1.DeviceSelector{
				{
					CEL: &resourcev1.CELDeviceSelector{
						Expression: celSelector,
					},
				},
			}
		}
		requests[i] = req
	}
	return requests
}

// reconcileManagedDRA creates or updates the ResourceClaim/ResourceClaimTemplate
// that backs managed DRA for this LLMInferenceService.
func (r *LLMISVCReconciler) reconcileManagedDRA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if !hasManagedDRA(llmSvc) {
		return nil
	}

	deviceClass := llmSvc.Annotations[constants.ManagedDRADeviceClassAnnotationKey]
	celSelector := llmSvc.Annotations[constants.ManagedDRACelSelectorAnnotationKey]
	isShared := llmSvc.Annotations[constants.ManagedDRASharingAnnotationKey] == "true"

	gpuCount, err := parseManagedDRAGpuCount(llmSvc)
	if err != nil {
		return err
	}

	deviceRequests := buildDeviceRequests(deviceClass, celSelector, gpuCount)

	if isShared {
		expected := expectedManagedDRAClaim(llmSvc, deviceRequests)
		if err := Reconcile(ctx, r, llmSvc, &resourcev1.ResourceClaim{}, expected, semanticResourceClaimIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile Managed DRA ResourceClaim: %w", err)
		}
	} else {
		expected := expectedManagedDRATemplate(llmSvc, deviceRequests)
		if err := Reconcile(ctx, r, llmSvc, &resourcev1.ResourceClaimTemplate{}, expected, semanticResourceClaimTemplateIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile Managed DRA ResourceClaimTemplate: %w", err)
		}
	}

	return nil
}

func expectedManagedDRATemplate(llmSvc *v1alpha2.LLMInferenceService, requests []resourcev1.DeviceRequest) *resourcev1.ResourceClaimTemplate {
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
					Requests: requests,
				},
			},
		},
	}
}

func expectedManagedDRAClaim(llmSvc *v1alpha2.LLMInferenceService, requests []resourcev1.DeviceRequest) *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedDRAResourceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: requests,
			},
		},
	}
}

func semanticResourceClaimTemplateIsEqual(expected *resourcev1.ResourceClaimTemplate, curr *resourcev1.ResourceClaimTemplate) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec)
}

func semanticResourceClaimIsEqual(expected *resourcev1.ResourceClaim, curr *resourcev1.ResourceClaim) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec)
}

// injectManagedDRA wires the managed DRA claim into the PodSpec:
//   - Adds a pod-level resourceClaim entry pointing to the generated ResourceClaim or ResourceClaimTemplate
//   - Adds container-level resources.claims entries so every container can access the GPU(s)
func injectManagedDRA(llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if !hasManagedDRA(llmSvc) {
		return
	}

	isShared := llmSvc.Annotations[constants.ManagedDRASharingAnnotationKey] == "true"
	resourceName := managedDRAResourceName(llmSvc)

	// Inject into PodSpec.ResourceClaims (the pod-level alias)
	hasPodClaim := false
	for _, claim := range podSpec.ResourceClaims {
		if claim.Name == managedDRAClaimName {
			hasPodClaim = true
			break
		}
	}
	if !hasPodClaim {
		podResourceClaim := corev1.PodResourceClaim{
			Name: managedDRAClaimName,
		}
		if isShared {
			podResourceClaim.ResourceClaimName = &resourceName
		} else {
			podResourceClaim.ResourceClaimTemplateName = &resourceName
		}
		podSpec.ResourceClaims = append(podSpec.ResourceClaims, podResourceClaim)
	}

	// Inject into ALL containers
	for i := range podSpec.Containers {
		hasContainerClaim := false
		for _, claim := range podSpec.Containers[i].Resources.Claims {
			if claim.Name == managedDRAClaimName {
				hasContainerClaim = true
				break
			}
		}
		if !hasContainerClaim {
			podSpec.Containers[i].Resources.Claims = append(
				podSpec.Containers[i].Resources.Claims,
				corev1.ResourceClaim{Name: managedDRAClaimName},
			)
		}
	}
}
