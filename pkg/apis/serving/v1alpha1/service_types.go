/*
Copyright 2019 Kubeflow Community.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	// If specified, passes name to generated Revision.
	RevisionName string `json:"rollout,omitempty"`
	// Must contain either one or two elements.
	// The first is the "active" revisions and the second is the "candidate" revision.
	Revisions []string `json:"revisions,omitempty"`
	// May only be specified if revisions has two elements. Defaults to 0.
	// This percentage of traffic is routed to the second revision, the remainder to the first.
	RolloutPercent int `json:"rolloutPercent,omitempty"`

	MinReplicas string `json:"minReplicas,omitempty"`
	MaxReplicas string `json:"maxReplicas,omitempty"`

	/*
		The following fields follow a "1-of" semantic. Users must specify exactly one spec.
	*/
	Custom      *CustomSpec      `json:"custom,omitempty"`
	Tensorflow  *TensorflowSpec  `json:"tensorflow,omitempty"`
	XGBoost     *XGBoostSpec     `json:"xgBoost,omitempty"`
	ScikitLearn *ScikitLearnSpec `json:"scikitLearn,omitempty"`
}

// TensorflowSpec defines arguments for configuring Tensorflow model serving.
type TensorflowSpec struct {
	ModelUri string `json:"modelUri"`
	// Defaults to latest TF Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	ModelUri string `json:"modelUri"`
	// Defaults to latest XGBoost Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	//Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// ScikitLearnSpec defines arguments for configuring ScikitLearn model serving.
type ScikitLearnSpec struct {
	ModelURI string `json:"modelUri"`
	// Defaults to latest ScikitLearn Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// CustomSpec provides a hook for arbitrary container configuration.
type CustomSpec struct {
	Container v1.Container `json:"container"`
	// Defaults to /healthz.
	LivenessProbe *v1.Probe `json:"livenessProbe,omitempty"`
	// Defaults to /healthz.
	ReadinessProbe *v1.Probe `json:"readinessProbe,omitempty"`
}

// ServiceStatus defines the observed state of Service
type ServiceStatus struct {
	Conditions                ConditionsSpec  `json:"conditions,omitempty"`
	URI                       URISpec         `json:"uri,omitempty"`
	Revisions                 []RevisionsSpec `json:"revisions,omitempty"`
	LatestCreatedRevisionName string          `json:"latestCreatedRevisionName,omitempty"`
	LatestReadyRevisionName   string          `json:"latestReadyRevisionName,omitempty"`
}

// URISpec describes the available network endpoints for the service.
type URISpec struct {
	Internal string `json:"internal,omitempty"`
	External string `json:"external,omitempty"`
}

// RevisionsSpec describes the current revisions receiving traffic.
type RevisionsSpec struct {
	Name     string `json:"name,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
	Traffic  int    `json:"traffic,omitempty"`
}

// ConditionsSpec displays the current conditions of the resource.
type ConditionsSpec struct {
	Type   ConditionType      `json:"type"`
	Status v1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another.
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty" protobuf:"bytes,3,opt,name=lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
}

// ConditionType is the of status conditions.
type ConditionType string

// These are valid conditions of a service.
const (
	Ready              ConditionType = "Ready"
	RoutingReady       ConditionType = "RoutingReady"
	ResourcesAvailable ConditionType = "ResourcesAvailabe"
	ContainerHealthy   ConditionType = "ContainerHealthy"
	RevisionReady      ConditionType = "RevisionReady"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service is the Schema for the services API
// +k8s:openapi-gen=true
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
