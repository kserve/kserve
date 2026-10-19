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

// KernelCache represents one reusable OCI kernel cache artifact.
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=kc
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="MountType",type=string,JSONPath=".status.mountType"
// +kubebuilder:printcolumn:name="Pods-Using",type=integer,JSONPath=".status.usage.totalPodsUsing"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KernelCacheSpec   `json:"spec,omitempty"`
	Status            KernelCacheStatus `json:"status,omitempty"`
}

// KernelCacheSpec defines the desired artifact and node preparation target.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="self.artifact == oldSelf.artifact",message="artifact is immutable after creation"
// +kubebuilder:validation:XValidation:rule="!has(self.nodeGroupRef) || self.nodeGroupRef.name.size() > 0",message="nodeGroupRef.name must not be empty when nodeGroupRef is set"
type KernelCacheSpec struct {
	// Artifact is the portable OCI artifact contract.
	// +kubebuilder:validation:Required
	Artifact KernelCacheArtifact `json:"artifact"`

	// MountType selects the cache delivery mode. OCI is the only supported mode.
	// +optional
	// +kubebuilder:default=oci
	// +kubebuilder:validation:Enum=oci
	MountType KernelCacheMountType `json:"mountType,omitempty"`

	// NodeGroupRef references the cluster-scoped KernelCacheNodeGroup where this cache is prepared.
	// If omitted, the controller reports an error until a node group is selected.
	// +optional
	NodeGroupRef *corev1.LocalObjectReference `json:"nodeGroupRef,omitempty"`
}

// KernelCacheList contains a list of KernelCache objects.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type KernelCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCache `json:"items"`
}
