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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KernelCacheCapture captures runtime-generated cache data as an OCI artifact.
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=kcc
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=".status.artifact.imageReference"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCacheCapture struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KernelCacheCaptureSpec   `json:"spec,omitempty"`
	Status            KernelCacheCaptureStatus `json:"status,omitempty"`
}

// KernelCacheCaptureSpec defines the desired capture configuration.
// +k8s:openapi-gen=true
type KernelCacheCaptureSpec struct {
	// SourceRef identifies the source InferenceService.
	// +kubebuilder:validation:Required
	SourceRef KernelCacheSourceRef `json:"sourceRef"`

	// TargetImage is the capture destination. If empty, the capture integration generates one.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TargetImage string `json:"targetImage,omitempty"`

	// CachePaths overrides the cache paths passed to the capture sidecar.
	// +optional
	// +kubebuilder:validation:MaxItems=64
	// +listType=atomic
	CachePaths []KernelCachePath `json:"cachePaths,omitempty"`

	// Signing selects the signing profile when artifact signing is enabled.
	// +optional
	Signing *KernelCacheSigningSpec `json:"signing,omitempty"`
}

// KernelCacheCapturePhase represents the current capture phase.
// +kubebuilder:validation:Enum=Pending;WaitingForWorkload;Capturing;Pushing;Complete;Unchanged;Failed
type KernelCacheCapturePhase string

const (
	KernelCacheCapturePhasePending            KernelCacheCapturePhase = "Pending"
	KernelCacheCapturePhaseWaitingForWorkload KernelCacheCapturePhase = "WaitingForWorkload"
	KernelCacheCapturePhaseCapturing          KernelCacheCapturePhase = "Capturing"
	KernelCacheCapturePhasePushing            KernelCacheCapturePhase = "Pushing"
	KernelCacheCapturePhaseComplete           KernelCacheCapturePhase = "Complete"
	KernelCacheCapturePhaseUnchanged          KernelCacheCapturePhase = "Unchanged"
	KernelCacheCapturePhaseFailed             KernelCacheCapturePhase = "Failed"
)

// KernelCacheCaptureStatus defines the observed capture result.
// +k8s:openapi-gen=true
type KernelCacheCaptureStatus struct {
	// ActiveSession identifies the session allowed to report capture results.
	// +optional
	ActiveSession *KernelCacheCaptureSession `json:"activeSession,omitempty"`

	// RuntimeResult contains the raw key-value result reported by the capture sidecar.
	// +optional
	RuntimeResult map[string]string `json:"runtimeResult,omitempty"`

	// Phase indicates the current capture phase.
	// +optional
	Phase KernelCacheCapturePhase `json:"phase,omitempty"`

	// Artifact is the normalized captured artifact.
	// +optional
	Artifact *KernelCacheArtifact `json:"artifact,omitempty"`

	// CapturedAt is the time reported for the completed capture.
	// +optional
	CapturedAt *metav1.Time `json:"capturedAt,omitempty"`

	// CapturedCacheSizeBytes is the captured cache size.
	// +optional
	CapturedCacheSizeBytes *int64 `json:"capturedCacheSizeBytes,omitempty"`

	// KernelCacheRef references the latest KernelCache generated from this capture.
	// +optional
	KernelCacheRef *NamespacedName `json:"kernelCacheRef,omitempty"`

	// Signing contains the operator signing result for the captured artifact.
	// +optional
	Signing *KernelCacheSigningStatus `json:"signing,omitempty"`

	// Conditions report whether the capture is ready and why it is not ready.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// KernelCacheCaptureSession identifies one capture attempt.
// +k8s:openapi-gen=true
type KernelCacheCaptureSession struct {
	// ID uniquely identifies the capture session for a producer Pod.
	// The sidecar includes this value in its status reports so the controller can
	// reject stale reports from another capture session.
	ID      string `json:"id"`
	PodName string `json:"podName"`
	// NodeName is the node assigned to the producer Pod.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// RequestedNodeGroup is the node group requested by the producer Pod.
	// +optional
	RequestedNodeGroup string `json:"requestedNodeGroup,omitempty"`
}

// KernelCacheCaptureList contains a list of KernelCacheCapture objects.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type KernelCacheCaptureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheCapture `json:"items"`
}
