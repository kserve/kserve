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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

// LLMInferenceService is the Schema for the llminferenceservices API, representing a single LLM deployment.
// It orchestrates the creation of underlying Kubernetes resources like Deployments and Services,
// and configures networking for exposing the model.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="URLs",type="string",JSONPath=".status.addresses[*].url",priority=1
// +kubebuilder:resource:path=llminferenceservices,shortName=llmisvc
type LLMInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec   `json:"spec,omitempty"`
	Status LLMInferenceServiceStatus `json:"status,omitempty"`
}

// LLMInferenceServiceConfig is the Schema for the llminferenceserviceconfigs API.
// It acts as a template to provide base configurations that can be inherited by multiple LLMInferenceService instances.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LLMInferenceServiceSpec `json:"spec,omitempty"`
}

// LLMInferenceServiceSpec defines the desired state of LLMInferenceService.
type LLMInferenceServiceSpec struct {
	// Model specification, including its URI, potential LoRA adapters, and storage details.
	Model LLMModelSpec `json:"model"`

	// WorkloadSpec configurations for the primary inference deployment.
	// In a standard setup, this defines the main model server deployment.
	// In a disaggregated setup (when 'prefill' is specified), this configures the 'decode' workload.
	// +optional
	WorkloadSpec `json:",inline,omitempty"`

	// Router configuration for how the service is exposed. This section dictates the creation and management
	// of networking resources like Ingress or Gateway API objects (HTTPRoute, Gateway).
	// +optional
	Router *RouterSpec `json:"router,omitempty"`

	// Prefill configuration for disaggregated serving.
	// When this section is included, the controller creates a separate deployment for prompt processing (prefill)
	// in addition to the main 'decode' deployment, inspired by the llm-d architecture.
	// This allows for independent scaling and hardware allocation for prefill and decode steps.
	// +optional
	Prefill *WorkloadSpec `json:"prefill,omitempty"`

	// BaseRefs allows inheriting and overriding configurations from one or more LLMInferenceServiceConfig instances.
	// The controller merges these base configurations, with the current LLMInferenceService spec taking the highest precedence.
	// When multiple baseRefs are provided, the last one in the list overrides previous ones.
	// +optional
	BaseRefs []corev1.LocalObjectReference `json:"baseRefs,omitempty"`
}

// WorkloadSpec defines the configuration for a deployment workload, such as replicas and pod specifications.
type WorkloadSpec struct {
	// Number of replicas for the deployment.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Parallelism configurations for the runtime, such as tensor and pipeline parallelism.
	// These values are used to configure the underlying inference runtime (e.g., vLLM).
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Template for the main pod spec.
	// In a multi-node deployment, this configures the "head" or "master" pod.
	// In a disaggregated deployment, this configures the "decode" pod if it's the top-level template,
	// or the "prefill" pod if it's within the Prefill block.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`

	// Worker configuration for multi-node deployments.
	// The presence of this field triggers the creation of a multi-node (distributed) setup.
	// This spec defines the configuration for the worker pods, while the main 'Template' field defines the head pod.
	// The controller is responsible for enabling discovery between head and worker pods.
	// +optional
	Worker *corev1.PodSpec `json:"worker,omitempty"`
}

// LLMModelSpec defines the model source and its characteristics.
type LLMModelSpec struct {
	// URI of the model, specifying its location, e.g., hf://meta-llama/Llama-4-Scout-17B-16E-Instruct
	// The storage-initializer init container uses this URI to download the model.
	URI apis.URL `json:"uri"`

	// Name is the name of the model as it will be set in the "model" parameter for an incoming request.
	// If omitted, it will default to `metadata.name`. For LoRA adapters, this field is required.
	// +optional
	Name *string `json:"name,omitempty"`

	// Criticality defines how important it is to serve the model compared to other models.
	// This is used by the Inference Gateway scheduler.
	// +optional
	Criticality *igwapi.Criticality `json:"criticality,omitempty"`

	// LoRA (Low-Rank Adaptation) adapters configurations.
	// Allows for specifying one or more LoRA adapters to be applied to the base model.
	// +optional
	LoRA *LoRASpec `json:"lora,omitempty"`

	// Storage specification for the model, such as path and credentials.
	// This is used by the storage-initializer to correctly download the model from the specified URI.
	Storage *LLMStorageSpec `json:"storage,omitempty"`
}

// LoRASpec defines the configuration for LoRA adapters.
type LoRASpec struct {
	// Adapters is the static specification for one or more LoRA adapters.
	// Each adapter is defined by its own ModelSpec.
	// +optional
	Adapters []ModelSpec `json:"adapters,omitempty"`
}

// RouterSpec defines the routing configuration for exposing the service.
// It supports Kubernetes Ingress and the Gateway API. The fields are mutually exclusive.
type RouterSpec struct {
	// Route configuration for the Gateway API.
	// If an empty object `{}` is provided, the controller creates and manages a new HTTPRoute.
	// +optional
	Route *GatewayRoutesSpec `json:"route,omitempty"`

	// Gateway configuration for the Gateway API, mutually exclusive with Ingress.
	// If an empty object `{}` is provided, the controller uses a default Gateway.
	// This must be used in conjunction with the 'Route' field for managed Gateway API resources.
	// +optional
	Gateway *GatewaySpec `json:"gateway,omitempty"`

	// Ingress configuration. This is mutually exclusive with Route and Gateway.
	// If an empty object `{}` is provided, the controller creates and manages a default Ingress resource.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Scheduler configuration for the Inference Gateway extension.
	// If this field is non-empty, an InferenceModel resource will be created to integrate with the gateway's scheduler.
	// +optional
	Scheduler *SchedulerSpec `json:"scheduler,omitempty"`
}

// GatewayRoutesSpec defines the configuration for a Gateway API route.
type GatewayRoutesSpec struct {
	// HTTP route configuration.
	// +optional
	HTTP *HTTPRouteSpec `json:"http,omitempty"`
}

// HTTPRouteSpec defines configurations for a Gateway API HTTPRoute.
// 'Spec' and 'Refs' are mutually exclusive and determine whether the route is managed by the controller or user-managed.
type HTTPRouteSpec struct {
	// Refs provides references to existing, user-managed HTTPRoute objects ("Bring Your Own" route).
	// The controller will validate the existence of these routes but will not modify them.
	// +optional
	Refs []corev1.LocalObjectReference `json:"refs,omitempty"`

	// Spec allows for providing a custom specification for an HTTPRoute.
	// If provided, the controller will create and manage an HTTPRoute with this specification.
	// +optional
	Spec *gatewayapi.HTTPRouteSpec `json:"spec,omitempty"`
}

// GatewaySpec defines the configuration for a Gateway API Gateway.
type GatewaySpec struct {
	// Refs provides references to existing, user-managed Gateway objects ("Bring Your Own" gateway).
	// The controller will use the specified Gateway instead of creating one.
	// +optional
	Refs []UntypedObjectReference `json:"refs,omitempty"`
}

// IngressSpec defines the configuration for a Kubernetes Ingress.
type IngressSpec struct {
	// Refs provides a reference to an existing, user-managed Ingress object ("Bring Your Own" ingress).
	// The controller will not create an Ingress but will use the referenced one to populate status URLs.
	// +optional
	Refs []UntypedObjectReference `json:"refs,omitempty"`
}

// SchedulerSpec defines the Inference Gateway extension configuration.
type SchedulerSpec struct {
	// Pool configuration for the InferencePool, which is part of the Inference Gateway extension.
	// +optional
	Pool *InferencePoolSpec `json:"pool,omitempty"`

	// Template for the Inference Gateway Extension pod spec.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`
}

// InferencePoolSpec defines the configuration for an InferencePool.
// 'Spec' and 'Ref' are mutually exclusive.
type InferencePoolSpec struct {
	// Spec defines an inline InferencePool specification.
	// +optional
	Spec *igwapi.InferencePoolSpec `json:"spec,omitempty"`

	// Ref is a reference to an existing InferencePool.
	// +optional
	Ref *corev1.LocalObjectReference `json:"ref,omitempty"`
}

// ParallelismSpec defines the parallelism parameters for distributed inference.
type ParallelismSpec struct {
	// Tensor parallelism size.
	// +optional
	Tensor *int64 `json:"tensor,omitempty"`
	// Pipeline parallelism size.
	// +optional
	Pipeline *int64 `json:"pipeline,omitempty"`
	// TODO more to be added ...
}

// LLMStorageSpec is a copy of the v1beta1.StorageSpec. It is duplicated here to avoid
// import cycles between the v1alpha1 and v1beta1 API packages.
type LLMStorageSpec struct {
	// The path to the model object in the storage. It cannot co-exist
	// with the storageURI.
	// +optional
	Path *string `json:"path,omitempty"`
	// Parameters to override the default storage credentials and config.
	// +optional
	Parameters *map[string]string `json:"parameters,omitempty"`
	// The Storage Key in the secret for this model.
	// +optional
	StorageKey *string `json:"key,omitempty"`
}

// UntypedObjectReference is a reference to an object without a specific Group/Version/Kind.
// It's used for referencing networking resources like Gateways and Ingresses where the exact type
// might be inferred or is not strictly required by this controller.
type UntypedObjectReference struct {
	// Name of the referenced object.
	Name gatewayapi.ObjectName `json:"name,omitempty"`
	// Namespace of the referenced object.
	Namespace gatewayapi.Namespace `json:"namespace,omitempty"`
}

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService.
type LLMInferenceServiceStatus struct {
	// URL of the publicly exposed service.
	// +optional
	URL *apis.URL `json:"url,omitempty"`

	// Conditions of the resource.
	duckv1.Status `json:",inline"`

	// Addressable endpoint for the service, including cluster-local URLs.
	duckv1.AddressStatus `json:",inline,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LLMInferenceServiceList is the list type for LLMInferenceService.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceService `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LLMInferenceServiceConfigList is the list type for LLMInferenceServiceConfig.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceServiceConfig `json:"items"`
}
