/*
Copyright 2021 The KServe Authors.

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
	"errors"

	"gopkg.in/go-playground/validator.v9"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kserve/kserve/pkg/constants"
)

// +k8s:openapi-gen=true
type SupportedModelFormat struct {
	// Name of the model format.
	// +required
	Name string `json:"name"`
	// Version of the model format.
	// Used in validating that a predictor is supported by a runtime.
	// Can be "major", "major.minor" or "major.minor.patch".
	// +optional
	Version *string `json:"version,omitempty"`
	// Set to true to allow the ServingRuntime to be used for automatic model placement if
	// this model format is specified with no explicit runtime.
	// +optional
	AutoSelect *bool `json:"autoSelect,omitempty"`

	// +kubebuilder:validation:Minimum=1

	// Priority of this serving runtime for auto selection.
	// This is used to select the serving runtime if more than one serving runtime supports the same model format.
	// The value should be greater than zero.  The higher the value, the higher the priority.
	// Priority is not considered if AutoSelect is either false or not specified.
	// Priority can be overridden by specifying the runtime in the InferenceService.
	// +optional
	Priority *int32 `json:"priority,omitempty"`
}

// +k8s:openapi-gen=true
type StorageHelper struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// +k8s:openapi-gen=true
type ServingRuntimePodSpec struct {
	// List of containers belonging to the pod.
	// Containers cannot currently be added or removed.
	// There must be at least one container in a Pod.
	// Cannot be updated.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []corev1.Container `json:"containers" patchStrategy:"merge" patchMergeKey:"name" validate:"required"`

	// List of volumes that can be mounted by containers belonging to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge,retainKeys
	Volumes []corev1.Volume `json:"volumes,omitempty" patchStrategy:"merge,retainKeys" patchMergeKey:"name" protobuf:"bytes,1,rep,name=volumes"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Labels that will be add to the pod.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations that will be add to the pod.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use. For example,
	// in the case of docker, only DockerConfig type secrets are honored.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	// Use the host's ipc namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostIPC bool `json:"hostIPC,omitempty" protobuf:"varint,13,opt,name=hostIPC"`

	// Possibly other things here
}

// ServingRuntimeSpec defines the desired state of ServingRuntime. This spec is currently provisional
// and are subject to change as details regarding single-model serving and multi-model serving
// are hammered out.
// +k8s:openapi-gen=true
type ServingRuntimeSpec struct {
	// Model formats and version supported by this runtime
	SupportedModelFormats []SupportedModelFormat `json:"supportedModelFormats,omitempty"`

	// Whether this ServingRuntime is intended for multi-model usage or not.
	// +optional
	MultiModel *bool `json:"multiModel,omitempty"`

	// Set to true to disable use of this runtime
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// Supported protocol versions (i.e. v1 or v2 or grpc-v1 or grpc-v2)
	// +optional
	ProtocolVersions []constants.InferenceServiceProtocol `json:"protocolVersions,omitempty"`

	// Set WorkerSpec to enable multi-node/multi-gpu
	// +optional
	WorkerSpec *WorkerSpec `json:"workerSpec,omitempty"`

	ServingRuntimePodSpec `json:",inline"`

	// The following fields apply to ModelMesh deployments.

	// Name for each of the Endpoint fields is either like "port:1234" or "unix:/tmp/kserve/grpc.sock"

	// Grpc endpoint for internal model-management (implementing mmesh.ModelRuntime gRPC service)
	// Assumed to be single-model runtime if omitted
	// +optional
	GrpcMultiModelManagementEndpoint *string `json:"grpcEndpoint,omitempty"`

	// Grpc endpoint for inferencing
	// +optional
	GrpcDataEndpoint *string `json:"grpcDataEndpoint,omitempty"`
	// HTTP endpoint for inferencing
	// +optional
	HTTPDataEndpoint *string `json:"httpDataEndpoint,omitempty"`

	// Configure the number of replicas in the Deployment generated by this ServingRuntime
	// If specified, this overrides the podsPerRuntime configuration value
	// +optional
	Replicas *uint16 `json:"replicas,omitempty"`

	// Configuration for this runtime's use of the storage helper (model puller)
	// It is enabled unless explicitly disabled
	// +optional
	StorageHelper *StorageHelper `json:"storageHelper,omitempty"`

	// Provide the details about built-in runtime adapter
	// +optional
	BuiltInAdapter *BuiltInAdapter `json:"builtInAdapter,omitempty"`
}

// ServingRuntimeStatus defines the observed state of ServingRuntime
// +k8s:openapi-gen=true
type ServingRuntimeStatus struct{}

// ServerType constant for specifying the runtime name
// +k8s:openapi-gen=true
type ServerType string

// Built-in ServerTypes (others may be supported)
const (
	// Model server is Triton
	Triton ServerType = "triton"
	// Model server is MLServer
	MLServer ServerType = "mlserver"
	// Model server is OpenVino Model Server
	OVMS ServerType = "ovms"
)

// +k8s:openapi-gen=true
type BuiltInAdapter struct {
	// ServerType must be one of the supported built-in types such as "triton" or "mlserver",
	// and the runtime's container must have the same name
	ServerType ServerType `json:"serverType,omitempty"`
	// Port which the runtime server listens for model management requests
	RuntimeManagementPort int `json:"runtimeManagementPort,omitempty"`
	// Fixed memory overhead to subtract from runtime container's memory allocation to determine model capacity
	MemBufferBytes int `json:"memBufferBytes,omitempty"`
	// Timeout for model loading operations in milliseconds
	ModelLoadingTimeoutMillis int `json:"modelLoadingTimeoutMillis,omitempty"`
	// Environment variables used to control other aspects of the built-in adapter's behaviour (uncommon)
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// ServingRuntime is the Schema for the servingruntimes API
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Disabled",type="boolean",JSONPath=".spec.disabled"
// +kubebuilder:printcolumn:name="ModelType",type="string",JSONPath=".spec.supportedModelFormats[*].name"
// +kubebuilder:printcolumn:name="Containers",type="string",JSONPath=".spec.containers[*].name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type ServingRuntime struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServingRuntimeSpec   `json:"spec,omitempty"`
	Status ServingRuntimeStatus `json:"status,omitempty"`
}

// ServingRuntimeList contains a list of ServingRuntime
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type ServingRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServingRuntime `json:"items"`
}

// ClusterServingRuntime is the Schema for the servingruntimes API
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
// +kubebuilder:printcolumn:name="Disabled",type="boolean",JSONPath=".spec.disabled"
// +kubebuilder:printcolumn:name="ModelType",type="string",JSONPath=".spec.supportedModelFormats[*].name"
// +kubebuilder:printcolumn:name="Containers",type="string",JSONPath=".spec.containers[*].name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type ClusterServingRuntime struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServingRuntimeSpec   `json:"spec,omitempty"`
	Status ServingRuntimeStatus `json:"status,omitempty"`
}

// ClusterServingRuntimeList contains a list of ServingRuntime
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type ClusterServingRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterServingRuntime `json:"items"`
}

// SupportedRuntime is the schema for supported runtime result of automatic selection
type SupportedRuntime struct {
	Name string
	Spec ServingRuntimeSpec
}

// WorkerSpec is the schema for multi-node/multi-GPU feature
type WorkerSpec struct {
	ServingRuntimePodSpec `json:",inline"`

	// PipelineParallelSize defines the number of parallel workers.
	// It specifies the number of model partitions across multiple devices, allowing large models to be split and processed concurrently across these partitions
	// It also represents the number of replicas in the worker set, where each worker set serves as a scaling unit.
	// +optional
	PipelineParallelSize *int `json:"pipelineParallelSize,omitempty"`

	// TensorParallelSize specifies the number of GPUs to be used per node.
	// It indicates the degree of parallelism for tensor computations across the available GPUs.
	// +optional
	TensorParallelSize *int `json:"tensorParallelSize,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ServingRuntime{}, &ServingRuntimeList{})
	SchemeBuilder.Register(&ClusterServingRuntime{}, &ClusterServingRuntimeList{})
}

func (srSpec *ServingRuntimeSpec) IsDisabled() bool {
	return srSpec.Disabled != nil && *srSpec.Disabled
}

func (srSpec *ServingRuntimeSpec) IsMultiModelRuntime() bool {
	return srSpec.MultiModel != nil && *srSpec.MultiModel
}

func (srSpec *ServingRuntimeSpec) IsMultiNodeRuntime() bool {
	return srSpec.WorkerSpec != nil
}

func (srSpec *ServingRuntimeSpec) IsProtocolVersionSupported(modelProtocolVersion constants.InferenceServiceProtocol) bool {
	if len(modelProtocolVersion) == 0 || srSpec.ProtocolVersions == nil || len(srSpec.ProtocolVersions) == 0 {
		return true
	}
	for _, srProtocolVersion := range srSpec.ProtocolVersions {
		if srProtocolVersion == modelProtocolVersion {
			return true
		}
	}
	return false
}

// GetPriority returns the priority of the specified model. It returns nil if priority is not set or the model is not found.
func (srSpec *ServingRuntimeSpec) GetPriority(modelName string) *int32 {
	for _, model := range srSpec.SupportedModelFormats {
		if model.Name == modelName {
			return model.Priority
		}
	}
	return nil
}

func (m *SupportedModelFormat) IsAutoSelectEnabled() bool {
	return m.AutoSelect != nil && *m.AutoSelect
}

func (srSpec *ServingRuntimeSpec) IsValid() bool {
	if err := srSpec.validatePodSpecAndWorkerSpec(); err != nil {
		return false
	}
	return true
}

// common validation logic
func (srSpec *ServingRuntimeSpec) validatePodSpecAndWorkerSpec() error {
	// Respect for ServingRuntimePodSpec validate fields
	if err := validate.Struct(srSpec.ServingRuntimePodSpec); err != nil {
		return err
	}

	// Additional validation for WorkerSpec
	if srSpec.WorkerSpec != nil {
		if len(srSpec.WorkerSpec.Containers) == 0 {
			return errors.New("spec.workerSpec.containers: Required value")
		}
	}

	return nil
}

var validate = validator.New()

func (srSpec *ServingRuntimeSpec) ValidateCreate() error {
	return srSpec.validatePodSpecAndWorkerSpec()
}

func (srSpec *ServingRuntimeSpec) ValidateUpdate(old runtime.Object) error {
	return srSpec.validatePodSpecAndWorkerSpec()
}

func (srSpec *ServingRuntimeSpec) ValidateDelete() error {
	return nil
}
