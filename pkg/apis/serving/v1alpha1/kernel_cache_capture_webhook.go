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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	kernelcachecaptureLog                         = logf.Log.WithName("kernelcachecapture-webhook")
	_                     webhook.CustomDefaulter = &KernelCacheCapture{}
	_                     webhook.CustomValidator = &KernelCacheCapture{}
)

// SetupWebhookWithManager registers the webhook with the controller-runtime manager
func (kcc *KernelCacheCapture) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(kcc).
		WithDefaulter(kcc).
		WithValidator(kcc).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-serving-kserve-io-v1alpha1-kernelcachecapture,mutating=true,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=kernelcachecaptures,verbs=create;update,versions=v1alpha1,name=kernelcachecapture.kserve-webhook-server.defaulter,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha1-kernelcachecapture,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=kernelcachecaptures,verbs=create;update,versions=v1alpha1,name=kernelcachecapture.kserve-webhook-server.validator,admissionReviewVersions=v1

// Default implements webhook.CustomDefaulter (mutating webhook)
func (kcc *KernelCacheCapture) Default(ctx context.Context, obj runtime.Object) error {
	kernelcachecaptureLog.V(1).Info("Mutating webhook called", "object", obj)

	capture, ok := obj.(*KernelCacheCapture)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected KernelCacheCapture, got %T", obj))
	}

	kernelcachecaptureLog.V(1).Info("Decoded KernelCacheCapture object",
		"name", capture.Name,
		"namespace", capture.Namespace)

	// Default volumeStrategy to "shared"
	if capture.Spec.VolumeStrategy == "" {
		capture.Spec.VolumeStrategy = "shared"
		kernelcachecaptureLog.Info("Defaulting volumeStrategy to shared")
	}

	// Default createKernelCache.enabled to true if not explicitly set
	if capture.Spec.CreateKernelCache == nil {
		capture.Spec.CreateKernelCache = &CreateKernelCacheConfig{
			Enabled: ptr.To(true),
		}
		kernelcachecaptureLog.Info("Defaulting createKernelCache.enabled to true")
	} else if capture.Spec.CreateKernelCache.Enabled == nil {
		capture.Spec.CreateKernelCache.Enabled = ptr.To(true)
		kernelcachecaptureLog.Info("Defaulting createKernelCache.enabled to true")
	}

	// Default createKernelCache.mountType to imageVolume if not set
	if capture.Spec.CreateKernelCache != nil &&
		ptr.Deref(capture.Spec.CreateKernelCache.Enabled, false) &&
		capture.Spec.CreateKernelCache.MountType == "" {
		capture.Spec.CreateKernelCache.MountType = KernelCacheMountTypeImageVolume
		kernelcachecaptureLog.Info("Defaulting createKernelCache.mountType to imageVolume")
	}

	// Default registrySecretRef.key to .dockerconfigjson if secret is provided
	if capture.Spec.RegistrySecretRef != nil && capture.Spec.RegistrySecretRef.Key == "" {
		capture.Spec.RegistrySecretRef.Key = ".dockerconfigjson"
		kernelcachecaptureLog.Info("Defaulting registrySecretRef.key to .dockerconfigjson")
	}

	// Default trigger to false if not set
	if !capture.Spec.Trigger {
		kernelcachecaptureLog.V(1).Info("Trigger is false (default)")
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator for CREATE operations
func (kcc *KernelCacheCapture) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	capture, ok := obj.(*KernelCacheCapture)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected KernelCacheCapture, got %T", obj))
	}

	kernelcachecaptureLog.Info("Validating KernelCacheCapture create",
		"name", capture.Name,
		"namespace", capture.Namespace)

	// Check if KernelCache feature is enabled
	enabled, err := isKernelCacheEnabled(ctx)
	if err != nil {
		kernelcachecaptureLog.Error(err, "failed to check KernelCache enabled state")
		return nil, err
	}
	if !enabled {
		return nil, errors.New("KernelCacheCapture feature is disabled (kernelcache.enabled=false in inferenceservice-config ConfigMap)")
	}

	// Validate required fields
	if capture.Spec.TargetImage == "" {
		return nil, errors.New("spec.targetImage must be set")
	}

	// registrySecretRef is optional (for insecure/local registries)
	// If provided, validate the name is set
	if capture.Spec.RegistrySecretRef != nil && capture.Spec.RegistrySecretRef.Name == "" {
		return nil, errors.New("spec.registrySecretRef.name must be set when registrySecretRef is provided")
	}

	// Validate that either cachePreset or cachePath is set (mutually exclusive)
	if capture.Spec.CachePreset == "" && capture.Spec.CachePath == "" {
		return nil, errors.New("either spec.cachePreset or spec.cachePath must be set")
	}

	// Validate cachePreset is a known value if set
	if capture.Spec.CachePreset != "" {
		valid := false
		for _, p := range ValidCachePresets {
			if capture.Spec.CachePreset == p {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("spec.cachePreset must be one of %v, got %s",
				ValidCachePresets, capture.Spec.CachePreset)
		}
	}

	// Validate volumeStrategy
	if capture.Spec.VolumeStrategy != "shared" && capture.Spec.VolumeStrategy != "copy" {
		return nil, fmt.Errorf("spec.volumeStrategy must be 'shared' or 'copy', got %s",
			capture.Spec.VolumeStrategy)
	}

	// Validate createKernelCache.mountType if auto-create is enabled
	if capture.Spec.CreateKernelCache != nil &&
		ptr.Deref(capture.Spec.CreateKernelCache.Enabled, false) {
		mountType := capture.Spec.CreateKernelCache.MountType
		if mountType != "" &&
			mountType != KernelCacheMountTypePVC &&
			mountType != KernelCacheMountTypeImageVolume {
			return nil, fmt.Errorf("spec.createKernelCache.mountType must be 'pvc' or 'imageVolume', got %s",
				mountType)
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator for UPDATE operations
func (kcc *KernelCacheCapture) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldCapture, ok1 := oldObj.(*KernelCacheCapture)
	newCapture, ok2 := newObj.(*KernelCacheCapture)
	if !ok1 || !ok2 {
		return nil, apierrors.NewBadRequest("type assertion to KernelCacheCapture failed")
	}

	kernelcachecaptureLog.V(1).Info("Validating KernelCacheCapture update",
		"name", newCapture.Name,
		"namespace", newCapture.Namespace)

	// Run create validation on new object
	if warnings, err := kcc.ValidateCreate(ctx, newObj); err != nil {
		return warnings, err
	}

	// Immutable fields after capture starts (trigger=true)
	if oldCapture.Spec.Trigger {
		if oldCapture.Spec.TargetImage != newCapture.Spec.TargetImage {
			return nil, errors.New("spec.targetImage is immutable after capture is triggered")
		}
		if oldCapture.Spec.CachePreset != newCapture.Spec.CachePreset {
			return nil, errors.New("spec.cachePreset is immutable after capture is triggered")
		}
		if oldCapture.Spec.CachePath != newCapture.Spec.CachePath {
			return nil, errors.New("spec.cachePath is immutable after capture is triggered")
		}
		if oldCapture.Spec.VolumeStrategy != newCapture.Spec.VolumeStrategy {
			return nil, errors.New("spec.volumeStrategy is immutable after capture is triggered")
		}
	}

	// Don't allow trigger to be set to false after it was true
	if oldCapture.Spec.Trigger && !newCapture.Spec.Trigger {
		return nil, errors.New("spec.trigger cannot be changed from true to false")
	}

	// Don't allow changing trigger if capture is already complete
	if oldCapture.Status.Phase == KernelCacheCapturePhaseComplete &&
		oldCapture.Spec.Trigger != newCapture.Spec.Trigger {
		return nil, errors.New("spec.trigger is immutable after capture completes")
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator for DELETE operations
func (kcc *KernelCacheCapture) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	capture, ok := obj.(*KernelCacheCapture)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected KernelCacheCapture, got %T", obj))
	}

	kernelcachecaptureLog.Info("Validating KernelCacheCapture delete",
		"name", capture.Name,
		"namespace", capture.Namespace)

	// Allow deletion at any time - controller will handle cleanup
	// If auto-created KernelCache has ownerReference, it will be GC'd automatically

	return nil, nil
}
