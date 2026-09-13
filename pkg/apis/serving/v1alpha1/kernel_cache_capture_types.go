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
// +kubebuilder:validation:XValidation:rule="has(self.cacheFramework) || has(self.cachePathOverride)",message="at least one of cacheFramework or cachePathOverride must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.cacheFramework) && has(self.cachePathOverride))",message="cacheFramework and cachePathOverride are mutually exclusive; set one or the other"
type KernelCacheCaptureSpec struct {
	// TargetImage is the OCI image URL where captured cache will be pushed
	// +kubebuilder:validation:Required
	TargetImage string `json:"targetImage"`

	// CacheFramework identifies the inference framework running in the ISVC.
	// The controller uses this to locate the framework's default cache directory
	// and stamp the correct cache-type OCI label on the captured image
	// (e.g. vllm → /root/.cache/vllm, gaudi → /home/kserve/.cache/habana).
	// Exactly one of CacheFramework or CachePathOverride must be set.
	// +kubebuilder:validation:Enum=vllm;tgi;triton-python;gaudi
	// +optional
	CacheFramework string `json:"cacheFramework,omitempty"`

	// CachePathOverride specifies the exact directory path to capture when the
	// framework does not write its cache to a standard location known to the
	// controller. Use this when the ISVC is configured to write cache files to
	// a custom path. The controller infers the cache type from the captured
	// content rather than from a framework preset.
	// Exactly one of CacheFramework or CachePathOverride must be set.
	// +optional
	CachePathOverride string `json:"cachePathOverride,omitempty"`
}

// Canonical CacheFramework values. These are the SINGLE SOURCE OF TRUTH for the
// allowed KernelCacheCaptureSpec.CacheFramework set, shared by the webhook
// validator (ValidateCreate) and the pod injector (cacheFrameworks map,
// producerCacheType). The +kubebuilder:validation:Enum marker on the field
// restates these literals because kubebuilder markers cannot reference Go
// consts; TestCacheFrameworkEnumMarkerMatchesCanonical keeps the two in sync.
const (
	CacheFrameworkVLLM         = "vllm"
	CacheFrameworkTGI          = "tgi"
	CacheFrameworkTritonPython = "triton-python"
	CacheFrameworkGaudi        = "gaudi"
)

// ValidCacheFrameworks is the ordered canonical set of allowed CacheFramework values.
// Keep in lockstep with the CacheFramework Enum marker (see the const block above).
var ValidCacheFrameworks = []string{
	CacheFrameworkVLLM,
	CacheFrameworkTGI,
	CacheFrameworkTritonPython,
	CacheFrameworkGaudi,
}

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
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true

// KernelCacheCaptureList contains a list of KernelCacheCapture
type KernelCacheCaptureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheCapture `json:"items"`
}
