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
)

// KernelCacheArtifact is the portable description of a completed OCI cache artifact.
// It is shared by KernelCacheCapture status and KernelCache spec.
// +k8s:openapi-gen=true
type KernelCacheArtifact struct {
	// ImageReference is a full immutable OCI reference containing registry,
	// repository, and digest.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^.+@sha256:[a-f0-9]{64}$`
	ImageReference string `json:"imageReference"`

	// CachePaths describes the cache directories represented in the artifact.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +listType=atomic
	CachePaths []KernelCachePath `json:"cachePaths"`

	// Identity contains the workload and compatibility identities of the artifact.
	// +kubebuilder:validation:Required
	Identity KernelCacheIdentity `json:"identity"`
}

// KernelCachePath describes one cache directory in the source container and OCI artifact.
// +k8s:openapi-gen=true
type KernelCachePath struct {
	// ContainerName is the container that owns the cache directory. If omitted,
	// KServe resolves the standard runtime container; set it for non-standard containers.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ContainerName string `json:"containerName,omitempty"`

	// ContainerPath is the absolute path of the cache directory in the container. If omitted,
	// KServe resolves VLLM_CACHE_ROOT and falls back to /root/.cache/vllm.
	// +optional
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Pattern=`^/[^$\x00]+$`
	// +kubebuilder:validation:XValidation:rule="self != '/' && self.split('/').all(segment, segment != '.' && segment != '..')",message="containerPath must be a canonical non-root absolute path"
	ContainerPath string `json:"containerPath,omitempty"`

	// OCIPath is the relative path of the cache directory in the OCI artifact.
	// +optional
	// +kubebuilder:default=io.vllm.cache
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Pattern=`^[^/\x00][^\x00]*$`
	// +kubebuilder:validation:XValidation:rule="self != '.' && self.split('/').all(segment, segment != '..')",message="ociPath must not contain parent-directory segments"
	OCIPath string `json:"ociPath,omitempty"`
}

// KernelCacheFootprints contains the stable identities used to identify a cache artifact.
// +k8s:openapi-gen=true
type KernelCacheFootprints struct {
	// WorkloadFootprint is the SHA-256 identity of the cache-relevant workload.
	// It is omitted when the declared runtime image is not digest-pinned.
	// +optional
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	WorkloadFootprint string `json:"workloadFootprint,omitempty"`

	// CompatibilityFootprint is the SHA-256 identity of the compatibility factors.
	// It is omitted for latest or implicitly-latest runtime images.
	// +optional
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	CompatibilityFootprint string `json:"compatibilityFootprint,omitempty"`
}

// KernelCacheIdentity contains values used to identify and select a cache artifact.
// +k8s:openapi-gen=true
type KernelCacheIdentity struct {
	// Footprints contains the stable workload and compatibility identities.
	// +kubebuilder:validation:Required
	Footprints KernelCacheFootprints `json:"footprints"`

	// Factors contains the normalized values used to calculate the footprints.
	// +optional
	Factors map[string]string `json:"factors,omitempty"`
}

// KernelCacheSourceRef identifies the source InferenceService for a capture.
// +k8s:openapi-gen=true
type KernelCacheSourceRef struct {
	// Kind is the kind of the source workload.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=InferenceService
	Kind string `json:"kind"`

	// Name is the name of the source workload.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// KernelCacheMountType defines the cache delivery mode.
type KernelCacheMountType string

const (
	// KernelCacheMountTypeOCI mounts the OCI image as the cache source.
	KernelCacheMountTypeOCI KernelCacheMountType = "oci"
)

// KernelCacheSigningSpec selects the operator signing profile for a capture.
// Private keys and credentials are not stored in the API object.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!has(self.profileRef) || self.profileRef.name.size() > 0",message="profileRef.name must not be empty when profileRef is set"
type KernelCacheSigningSpec struct {
	// ProfileRef references an operator-managed signing profile.
	// +optional
	ProfileRef *corev1.LocalObjectReference `json:"profileRef,omitempty"`
}
