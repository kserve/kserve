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
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KernelCacheSpec   `json:"spec,omitempty"`
	Status            KernelCacheStatus `json:"status,omitempty"`
}

// KernelCacheSpec defines the desired state of KernelCache
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="self.mountType != 'imageVolume' || !has(self.pvc)",message="pvc must not be set when mountType is imageVolume"
type KernelCacheSpec struct {
	// Image is the OCI image URL containing the kernel cache. Both tags and digests are
	// accepted (e.g. myrepo/cache:v1 or myrepo/cache@sha256:abc123). The webhook resolves
	// tags to digests and pins the resolved digest in status to prevent cache drift.
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// MountType specifies how the cache image is delivered to serving containers.
	// pvc: extracts the OCI image into a PersistentVolumeClaim; requires spec.pvc.
	// imageVolume: mounts the OCI image directly without extraction (Kubernetes 1.33+).
	// Immutable after creation.
	// +kubebuilder:validation:Enum=pvc;imageVolume
	// +kubebuilder:default=pvc
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="mountType is immutable after creation"
	// +optional
	MountType KernelCacheMountType `json:"mountType,omitempty"`

	// PVC holds configuration for PVC-based cache delivery.
	// Required when mountType is pvc; must not be set when mountType is imageVolume.
	// +optional
	PVC *KernelCachePVCConfig `json:"pvc,omitempty"`

	// ImagePullPolicy controls when the cache image is pulled.
	// For pvc mode: governs when the extraction job pulls the image.
	// For imageVolume mode: governs when Kubernetes pulls the image for volume mounting.
	// +kubebuilder:validation:Enum=IfNotPresent;Always;Never
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// MountPath in the container filesystem where the cache is mounted.
	// When empty (recommended), auto-computed from OCI image labels to maintain
	// framework compatibility. Override only when automatic detection is insufficient.
	// +optional
	MountPath string `json:"mountPath,omitempty"`
}

// KernelCachePVCConfig holds PVC-specific configuration for cache extraction and serving.
// Only applies when mountType is pvc.
//
// Provisioning modes are mutually exclusive:
//   - Dynamic (storageClassName): the StorageClass provisioner creates the PV automatically.
//   - Static (volumeName): binds to a pre-existing PV by name.
//
// The storage fields (storageClassName, volumeName, storageSize, accessModes) are
// immutable after the PVC is provisioned. Changing them after creation conflicts with
// the existing PVC and has undefined behavior.
//
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!(has(self.storageClassName) && has(self.volumeName))",message="storageClassName and volumeName are mutually exclusive; set one or the other"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.storageClassName) || (has(self.storageClassName) && self.storageClassName == oldSelf.storageClassName)",message="storageClassName is immutable after creation"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.volumeName) || (has(self.volumeName) && self.volumeName == oldSelf.volumeName)",message="volumeName is immutable after creation"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.storageSize) || (has(self.storageSize) && self.storageSize == oldSelf.storageSize)",message="storageSize is immutable after creation"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.accessModes) || self.accessModes == oldSelf.accessModes",message="accessModes is immutable after creation"
type KernelCachePVCConfig struct {
	// StorageClassName names the StorageClass used for dynamic PV provisioning.
	// The StorageClass provisioner creates a PV automatically when the PVC is created.
	// Omit to use the cluster's default StorageClass.
	// Mutually exclusive with volumeName.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// VolumeName binds the PVC to a pre-existing PV by name (static provisioning).
	// Use when you have pre-created a PV with specific storage type or topology.
	// storageSize is ignored when volumeName is set (capacity is defined on the PV).
	// Mutually exclusive with storageClassName.
	// +optional
	VolumeName *string `json:"volumeName,omitempty"`

	// StorageSize is the storage capacity requested by the PVC.
	// Applies to dynamic provisioning (storageClassName) only; ignored when volumeName
	// is set because the capacity is already defined on the pre-created PV.
	// +optional
	StorageSize *resource.Quantity `json:"storageSize,omitempty"`

	// AccessModes for the PVC. When unset, the StorageClass default applies.
	// Multi-node serving requires ReadWriteMany (RWX) so the extraction job and ISVC
	// pods on different nodes can all mount the PVC concurrently; ensure the
	// StorageClass supports RWX before setting it. Single-node deployments can use
	// ReadWriteOnce (RWO).
	// +optional
	// +listType=atomic
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// PodTemplate customizes the extraction Job pod (resource requests, tolerations,
	// node selector, etc.). Mutable: changes take effect on the next extraction run.
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
