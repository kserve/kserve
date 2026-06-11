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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KernelCache packages GPU kernel caches (PyTorch, Triton, vLLM JIT-compiled kernels)
// into OCI images and extracts them to PVCs for accelerated workload startup.
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=kc
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Nodes",type=integer,JSONPath=".status.counts.nodeCnt"
// +kubebuilder:printcolumn:name="Node-In-Use",type=integer,JSONPath=".status.counts.nodeInUseCnt"
// +kubebuilder:printcolumn:name="Node-Not-In-Use",type=integer,JSONPath=".status.counts.nodeNotInUseCnt"
// +kubebuilder:printcolumn:name="Node-Error",type=integer,JSONPath=".status.counts.nodeErrorCnt"
// +kubebuilder:printcolumn:name="Pods-Using",type=integer,JSONPath=".status.servingStatus.totalPodsUsing"
// +kubebuilder:printcolumn:name="Pods-Terminating",type=integer,JSONPath=".status.servingStatus.totalPodsTerminating"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KernelCacheSpec   `json:"spec,omitempty"`
	Status            KernelCacheStatus `json:"status,omitempty"`
}

// KernelCacheSpec defines the desired state of KernelCache
// +k8s:openapi-gen=true
type KernelCacheSpec struct {
	// Image is the OCI image URL containing kernel cache
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// GPU metadata for automatic ISVC matching (Phase 2 webhook uses this)
	// Populated from MCV GPU detection or sidecar auto-creation
	// +optional
	GPUType string `json:"gpuType,omitempty"` // "nvidia-a100", "amd-mi300x" (from MCV)

	// +optional
	Framework string `json:"framework,omitempty"` // "vllm", "pytorch", "triton"

	// +optional
	FrameworkVersion string `json:"frameworkVersion,omitempty"` // "0.6.0"

	// +optional
	MinCUDAVersion string `json:"minCUDAVersion,omitempty"` // "12.0" (compatibility check)

	// +optional
	MinDriverVersion string `json:"minDriverVersion,omitempty"` // "535" (compatibility check)

	// Phase 1 simple mode storage fields (removed in Phase 2 when NodeGroups added)
	// StorageClassName for PV/PVC (optional, uses cluster default if unset)
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// StorageSize for PV/PVC (optional, default: 10Gi if unset)
	// +optional
	StorageSize *resource.Quantity `json:"storageSize,omitempty"`

	// AccessModes for PV/PVC (optional, default: [ReadWriteMany] for Phase 1)
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// PodTemplate for extraction Job customization (nodeSelector, tolerations, etc.)
	// +optional
	PodTemplate *KernelCachePodTemplate `json:"podTemplate,omitempty"`
}

// KernelCacheList contains a list of KernelCache
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type KernelCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCache `json:"items"`
}
