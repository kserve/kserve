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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KernelCacheCapture captures JIT-compiled kernel caches from running InferenceServices
// and packages them as OCI images for reuse in other deployments.
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=kcc
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=".spec.targetImage"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCacheCapture struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KernelCacheCaptureSpec   `json:"spec,omitempty"`
	Status            KernelCacheCaptureStatus `json:"status,omitempty"`
}

// KernelCacheCaptureSpec defines the desired state of KernelCacheCapture
// +k8s:openapi-gen=true
type KernelCacheCaptureSpec struct {
	// TargetImage is the OCI image URL where captured cache will be pushed
	// +kubebuilder:validation:Required
	TargetImage string `json:"targetImage"`

	// RegistrySecretRef references a Secret containing registry push credentials
	// Optional for insecure/unauthenticated registries (e.g., kind-registry, localhost)
	// +optional
	RegistrySecretRef *SecretKeySelector `json:"registrySecretRef,omitempty"`

	// CachePreset specifies a known cache location preset (vllm, tgi, triton-python)
	// Mutually exclusive with CachePath
	// +kubebuilder:validation:Enum=vllm;tgi;triton-python
	// +optional
	CachePreset string `json:"cachePreset,omitempty"`

	// CachePath specifies an explicit cache directory path
	// Overrides CachePreset if both are specified
	// +optional
	CachePath string `json:"cachePath,omitempty"`

	// VolumeStrategy specifies how to access the cache
	// shared: inject shared emptyDir volume (default)
	// copy: use kubectl cp to copy cache from main container
	// +kubebuilder:validation:Enum=shared;copy
	// +kubebuilder:default=shared
	// +optional
	VolumeStrategy string `json:"volumeStrategy,omitempty"`

	// Trigger initiates the cache capture when set to true
	// +kubebuilder:default=false
	// +optional
	Trigger bool `json:"trigger,omitempty"`

	// CreateKernelCache controls auto-creation of KernelCache CR
	// +optional
	CreateKernelCache *CreateKernelCacheConfig `json:"createKernelCache,omitempty"`
}

// CreateKernelCacheConfig controls auto-creation of KernelCache after successful capture
// +k8s:openapi-gen=true
type CreateKernelCacheConfig struct {
	// Enabled controls whether to auto-create KernelCache
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Name for the auto-created KernelCache (defaults to KCC name)
	// +optional
	Name string `json:"name,omitempty"`

	// MountType for the auto-created KernelCache
	// +kubebuilder:validation:Enum=pvc;imageVolume
	// +kubebuilder:default=imageVolume
	// +optional
	MountType KernelCacheMountType `json:"mountType,omitempty"`

	// PullSecretRef for pulling the captured image (can differ from push secret)
	// +optional
	PullSecretRef *corev1.LocalObjectReference `json:"pullSecretRef,omitempty"`
}

// SecretKeySelector references a key in a Secret
// +k8s:openapi-gen=true
type SecretKeySelector struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the secret (defaults to .dockerconfigjson)
	// +kubebuilder:default=.dockerconfigjson
	// +optional
	Key string `json:"key,omitempty"`
}

// ValidCachePresets is the canonical list of accepted CachePreset values.
// Must stay in sync with the +kubebuilder:validation:Enum marker on CachePreset.
var ValidCachePresets = []string{"vllm", "tgi", "triton-python"}

// ValidVolumeStrategies is the canonical list of accepted VolumeStrategy values.
// Must stay in sync with the +kubebuilder:validation:Enum marker on VolumeStrategy.
var ValidVolumeStrategies = []string{"shared", "copy"}

// KernelCacheCapturePhase represents the current phase of capture
type KernelCacheCapturePhase string

const (
	// KernelCacheCapturePhasePending indicates capture not yet triggered
	KernelCacheCapturePhasePending KernelCacheCapturePhase = "Pending"
	// KernelCacheCapturePhaseCapturing indicates MCV is building the image
	KernelCacheCapturePhaseCapturing KernelCacheCapturePhase = "Capturing"
	// KernelCacheCapturePhasePushing indicates image is being pushed to registry
	KernelCacheCapturePhasePushing KernelCacheCapturePhase = "Pushing"
	// KernelCacheCapturePhaseComplete indicates successful capture
	KernelCacheCapturePhaseComplete KernelCacheCapturePhase = "Complete"
	// KernelCacheCapturePhaseFailed indicates capture failed
	KernelCacheCapturePhaseFailed KernelCacheCapturePhase = "Failed"
)

// KernelCacheCaptureStatus defines the observed state of KernelCacheCapture
// +k8s:openapi-gen=true
type KernelCacheCaptureStatus struct {
	// Phase indicates current state of the capture
	// +optional
	Phase KernelCacheCapturePhase `json:"phase,omitempty"`

	// ImageDigest is the sha256 digest of the captured image
	// +optional
	ImageDigest string `json:"imageDigest,omitempty"`

	// CapturedAt is the timestamp when capture completed
	// +optional
	CapturedAt *metav1.Time `json:"capturedAt,omitempty"`

	// CapturedCacheSizeBytes is the size of the captured cache in bytes
	// +optional
	CapturedCacheSizeBytes *int64 `json:"capturedCacheSizeBytes,omitempty"`

	// KernelCacheRef references the auto-created KernelCache
	// +optional
	KernelCacheRef *NamespacedName `json:"kernelCacheRef,omitempty"`

	// DetectedCachePath is the resolved cache path used for capture
	// +optional
	DetectedCachePath string `json:"detectedCachePath,omitempty"`

	// PodName is the pod from which cache was captured
	// +optional
	PodName string `json:"podName,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true

// KernelCacheCaptureList contains a list of KernelCacheCapture
type KernelCacheCaptureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheCapture `json:"items"`
}
