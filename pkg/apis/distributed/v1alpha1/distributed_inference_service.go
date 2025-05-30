/*
Copyright 2025 The KServe Authors.

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
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// DistributedInferenceServiceSpec is the top level for this resource.
type DistributedInferenceServiceSpec struct {
	// Replicas is the desired number of inferenceservice
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`
	// Labels that will be added to the inference service.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations that will be added to the inference service.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// The deployment strategy to use to replace existing distributed inference service with new ones. Only applicable for raw deployment mode.
	// +optional
	DeploymentStrategy DeploymentStrategy `json:"deploymentStrategy,omitempty"`
	// This is for distributedinferenceservice autoscaling
	DistributedInferenceServiceScalerSpec `json:",inline"`
	// This is for inferenceservice
	// +required
	v1beta1.InferenceServiceSpec `json:"inferenservice"`
}

type DeploymentStrategy struct {
	Type          DeploymentStrategyType `json:"type"`
	RollingUpdate *RollingUpdateStrategy `json:"rollingUpdate,omitempty"`
}

type DeploymentStrategyType string

const (
	RolloutStrategyRollingUpdate DeploymentStrategyType = "RollingUpdate"
	RolloutStrategyRecreate      DeploymentStrategyType = "Recreate"
)

type RollingUpdateStrategy struct {
	MaxUnavailable intstr.IntOrString `json:"maxUnavailable,omitempty"`
	MaxSurge       intstr.IntOrString `json:"maxSurge,omitempty"`
}

type DistributedInferenceServiceScalerSpec struct {
	// Minimum number of replicas(inference service), defaults to 1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// MaximumReplicas defines the upper limit for autoscaling.
	// If not set, it defaults to the value of MinimumReplicas.
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	// AutoScaling autoscaling spec which is backed up HPA or KEDA.
	// +optional
	AutoScaling *v1beta1.AutoScalingSpec `json:"autoScaling,omitempty"`
}

// DistributedInferenceService is the Schema for the distributedinferenceservices API.
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Desired Replicas",type="integer",JSONPath=".status.desiredReplicas"
// +kubebuilder:printcolumn:name="Current Version",type="integer",JSONPath=".status.currentVersion"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:resource:path=distributedinferenceservices,shortName=disvc
// +kubebuilder:storageversion
type DistributedInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DistributedInferenceServiceSpec   `json:"spec,omitempty"`
	Status DistributedInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:openapi-gen=true
// DistributedInferenceServiceList contains a list of DistributedInferenceService.
type DistributedInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DistributedInferenceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DistributedInferenceService{}, &DistributedInferenceServiceList{})
}
