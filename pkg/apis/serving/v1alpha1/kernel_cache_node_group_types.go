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

// KernelCacheNodeGroup selects the Kubernetes nodes that host prepared
// KernelCache artifacts. Node selection is mandatory.
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=kcng
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCacheNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KernelCacheNodeGroupSpec `json:"spec,omitempty"`
}

// KernelCacheNodeGroupSpec defines node membership and workload tolerations
// for a KernelCacheNodeGroup.
// +k8s:openapi-gen=true
type KernelCacheNodeGroupSpec struct {
	// NodeSelector selects the nodes that belong to this group. A node is a
	// member of the group only when it carries every listed label. At least
	// one label is required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinProperties=1
	NodeSelector map[string]string `json:"nodeSelector"`

	// Tolerations are applied to controller-managed workloads that run on the
	// selected nodes (for example, node-local preparation jobs).
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// KernelCacheNodeGroupList contains a list of KernelCacheNodeGroup objects.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type KernelCacheNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheNodeGroup `json:"items"`
}
