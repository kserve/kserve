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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

// LLMInferenceService is the Schema for the llminferenceservices API.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec   `json:"spec,omitempty"`
	Status LLMInferenceServiceStatus `json:"status,omitempty"`
}

// LLMInferenceServiceConfig is the Schema for the llminferenceserviceconfigs API.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec         `json:"spec,omitempty"`
	Status LLMInferenceServiceConfigStatus `json:"status,omitempty"`
}

// LLMInferenceServiceSpec defines the desired state of LLMInferenceService
type LLMInferenceServiceSpec struct {
	// Type of the deployment, e.g., 'default' or 'llm-d'. 'default' uses Raw deployments.
	Type string `json:"type"`

	// Model specification.
	Model ModelSpec `json:"model"`

	// Template for the main pod spec. In a multi-node case, this configures the "head" pod.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`

	// WorkloadSpec configurations of the inference deployment.
	// +optional
	*WorkloadSpec `json:",inline,omitempty"`

	// Parallelism configurations for the runtime.
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Router configuration for the service.
	// +optional
	Router *RouterSpec `json:"router,omitempty"`

	// Prefill configuration for disaggregated serving.
	// +optional
	Prefill *WorkloadSpec `json:"prefill,omitempty"`

	// BaseRefs allows inheriting configuration from a LLMInferenceServiceConfig.
	// +optional
	BaseRefs []corev1.LocalObjectReference `json:"baseRefs,omitempty"`
}

type WorkloadSpec struct {
	// Number of replicas for the deployment.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Template for the worker pod spec. In a multi-node case, this configures the "worker" pod.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`

	// Worker configuration for multi-node deployments.
	// +optional
	Worker *corev1.PodSpec `json:"worker,omitempty"`
}

// LLMModelSpec defines the model source.
type LLMModelSpec struct {
	// URI of the model, e.g., hf://meta-llama/Llama-4-Scout-17B-16E-Instruct.
	URI apis.URL `json:"uri"`

	// Name is the name of the model as it will be set in the "model" parameter for an incoming request.
	// If omitted, it will default to `metadata.name`, for LoRA adapters, this is required.
	// +optional
	Name *string `json:"name,omitempty"`

	// Criticality defines how important it is to serve the model compared to other models.
	// +optional
	Criticality *igwapi.Criticality `json:"criticality,omitempty"`

	// LoRA adapters configurations.
	// +optional
	LoRA *LoRASpec `json:"lora,omitempty"`

	Storage *StorageSpec `json:"storage,omitempty"`
}

type LoRASpec struct {
	// Adapters is the static specification for LoRA adapters.
	// +optional
	Adapters []ModelSpec `json:"adapters,omitempty"`
}

// RouterSpec defines the routing configuration.
type RouterSpec struct {
	// Route configuration. An empty object automatically creates an HTTPRoute.
	// +optional
	Route *GatewayRoutesSpec `json:"route,omitempty"`

	// Gateway configuration. An empty object automatically creates a Gateway.
	// +optional
	Gateway *GatewaySpec `json:"gateway,omitempty"`

	// Ingress configuration. Mutually exclusive with route and gateway.
	// An empty object automatically creates an Ingress.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Scheduler configuration for Inference Gateway.
	// +optional
	Scheduler *SchedulerSpec `json:"scheduler,omitempty"`
}

// GatewayRoutesSpec defines the configuration for a route.
type GatewayRoutesSpec struct {
	// HTTP route configuration.
	// +optional
	HTTP *HTTPRouteSpec `json:"http,omitempty"`
}

// HTTPRouteSpec HTTPRoute configurations.
// Spec and Refs are mutually exclusive.
type HTTPRouteSpec struct {
	// References to custom routes.
	// +optional
	Refs []corev1.LocalObjectReference `json:"refs,omitempty"`

	// Spec custom spec of the HTTPRoute.
	// +optional
	Spec *gatewayapi.HTTPRouteSpec `json:"spec,omitempty"`
}

// GatewaySpec defines the configuration for Gateway.
// Spec and Refs are mutually exclusive.
type GatewaySpec struct {
	// References to custom gateways.
	// +optional
	Refs []UntypedObjectReference `json:"refs,omitempty"`

	// Spec custom spec of the Gateway
	// +optional
	Spec *gatewayapi.GatewaySpec `json:"spec,omitempty"`
}

// IngressSpec defines the configuration for Ingress.
// Spec and Refs are mutually exclusive.
type IngressSpec struct {
	// Reference to custom ingress(es).
	// +optional
	Refs []UntypedObjectReference `json:"refs,omitempty"`

	// Spec custom spec of the Ingress.
	// +optional
	Spec *networkingv1.IngressSpec `json:"spec,omitempty"`
}

// SchedulerSpec defines the Inference Gateway extension configuration.
type SchedulerSpec struct {
	// Pool configuration for the InferencePool.
	// +optional
	Pool *InferencePoolSpec `json:"pool,omitempty"`

	// Template is the Inference Gateway Extension pod template.
	Template *corev1.PodSpec `json:"template,omitempty"`
}

type InferencePoolSpec struct {
	Spec *igwapi.InferencePoolSpec `json:"spec,omitempty"`

	Ref *corev1.LocalObjectReference `json:"ref,omitempty"`
}

type ParallelismSpec struct {
	Tensor   *int64 `json:"tensor,omitempty"`
	Pipeline *int64 `json:"pipeline,omitempty"`
	// TODO more to be added ...
}

// StorageSpec is a copy of the v1beta1.StorageSpec as the v1beta1 package depends on the v1alpha1 and that
// creates import cycles.
type StorageSpec struct {
	// The path to the model object in the storage. It cannot co-exist
	// with the storageURI.
	// +optional
	Path *string `json:"path,omitempty"`
	// The path to the model schema file in the storage.
	// +optional
	SchemaPath *string `json:"schemaPath,omitempty"`
	// Parameters to override the default storage credentials and config.
	// +optional
	Parameters *map[string]string `json:"parameters,omitempty"`
	// The Storage Key in the secret for this model.
	// +optional
	StorageKey *string `json:"key,omitempty"`
}

type UntypedObjectReference struct {
	Name      gatewayapi.ObjectName `json:"name,omitempty"`
	Namespace gatewayapi.Namespace  `json:"namespace,omitempty"`
}

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService
type LLMInferenceServiceStatus struct {
	// URL of the service.
	// +optional
	URL *apis.URL `json:"url,omitempty"`

	duckv1.Status `json:",inline"`

	// Cluster-local URL(s)
	duckv1.AddressStatus `json:",inline,omitempty"`
}

// LLMInferenceServiceConfigStatus defines the observed state of LLMInferenceServiceConfig
type LLMInferenceServiceConfigStatus struct {
	duckv1.Status `json:",inline"`
}

// LLMInferenceServiceList is the list type for LLMInferenceService.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceService `json:"items"`
}

// LLMInferenceServiceConfigList is the list type for LLMInferenceServiceConfig.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LLMInferenceServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceServiceConfig `json:"items"`
}
