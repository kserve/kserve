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

package v1alpha2

import (
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// LLMInferenceService is the Schema for the llminferenceservices API, representing a single LLM deployment.
// It orchestrates the creation of underlying Kubernetes resources like Deployments and Services,
// and configures networking for exposing the model.
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="URLs",type="string",JSONPath=".status.addresses[*].url",priority=1
// +kubebuilder:resource:path=llminferenceservices,shortName=llmisvc
// +kubebuilder:storageversion
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
// +kubebuilder:storageversion
type LLMInferenceServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LLMInferenceServiceSpec `json:"spec,omitempty"`
}

// LLMInferenceServiceSpec defines the desired state of LLMInferenceService.
type LLMInferenceServiceSpec struct {
	// Model specification, including its URI, potential LoRA adapters, and storage details.
	// It's optional for `LLMInferenceServiceConfig` kind.
	// +optional
	Model LLMModelSpec `json:"model"`

	// StorageInitializer configuration for model artifact fetching.
	// +optional
	StorageInitializer *StorageInitializerSpec `json:"storageInitializer,omitempty"`

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
// +kubebuilder:validation:XValidation:rule="!(has(self.replicas) && has(self.scaling))",message="replicas and scaling are mutually exclusive; use scaling for autoscaled deployments or replicas for static deployments"
// +kubebuilder:validation:XValidation:rule="!(has(self.worker) && has(self.scaling))",message="autoscaling (scaling) is not supported for multi-node deployments (worker is set); remove scaling and use replicas instead to set a fixed replica count"
// TODO: remove the worker+scaling restriction above once WVA supports LeaderWorkerSet as a scaling target.
type WorkloadSpec struct {
	// Number of replicas for the deployment.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Scaling configuration for autoscaling this workload.
	// When specified, the controller creates and manages autoscaling resources
	// (VariantAutoscaling CR, ServiceMonitor, and the selected actuator — HPA or KEDA ScaledObject)
	// targeting this workload.
	// Mutually exclusive with the static 'replicas' field.
	// In a disaggregated setup, each workload (decode and prefill) can have its own independent scaling configuration,
	// resulting in separate autoscaling resources per workload.
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Parallelism configurations for the runtime, such as tensor and pipeline parallelism.
	// These values are used to configure the underlying inference runtime (e.g., vLLM).
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Labels that will be added to the component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations that will be added to the component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

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

	// LoRA (Low-Rank Adaptation) adapters configurations.
	// Allows for specifying one or more LoRA adapters to be applied to the base model.
	// +optional
	LoRA *LoRASpec `json:"lora,omitempty"`
}

// LoRASpec defines the configuration for LoRA adapters.
type LoRASpec struct {
	// Adapters is the static specification for one or more LoRA adapters.
	// Each adapter is defined by its own ModelSpec.
	// +optional
	// This type is recursive https://github.com/kubernetes-sigs/controller-tools/issues/585
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Adapters []LLMModelSpec `json:"adapters,omitempty"`
}

// StorageInitializerSpec defines the configuration for the storage initializer.
// The storage initializer is an initContainer responsible for downloading model artifacts
// from remote storage (s3://, hf://) before the main container starts.
//
// Example - Disable storage initializer:
//
//	storageInitializer:
//	  enabled: false
//
// Example - Explicitly enable (same as default):
//
//	storageInitializer:
//	  enabled: true
type StorageInitializerSpec struct {
	// Enabled controls whether the storage-initializer initContainer is created.
	// When nil or true, storage-initializer is created for applicable URIs (s3://, hf://).
	// When explicitly set to false, storage-initializer creation is skipped.
	// This is useful when models are pre-loaded via alternative mechanisms (e.g., custom init containers, modelcars).
	// Default: true (nil is treated as true for backward compatibility)
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
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
	Spec *gwapiv1.HTTPRouteSpec `json:"spec,omitempty"`
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
//
// The SchedulerSpec configures the connection from the Gateway to the model deployment leveraging the LLM optimized
// request Scheduler, also known as the Endpoint Picker (EPP) which determines the exact pod that should handle the
// request and responds back to Envoy with the target pod, Envoy will then forward the request to the chosen pod.
//
// The Scheduler is only effective when having multiple inference pod replicas.
//
// Step 1: Gateway (Envoy) &lt;-- ExtProc --&gt; EPP (select the optimal replica to handle the request)
// Step 2: Gateway (Envoy) &lt;-- forward request --&gt; Inference Pod X
type SchedulerSpec struct {
	// Pool configuration for the InferencePool, which is part of the Inference Gateway extension.
	// +optional
	Pool *InferencePoolSpec `json:"pool,omitempty"`

	// Labels that will be added to the scheduler component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations that will be added to the scheduler component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Template for the Inference Gateway Extension pod spec.
	// This configures the Endpoint Picker (EPP) Deployment.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`

	// Config is the configuration for the EndpointPicker.
	Config *SchedulerConfigSpec `json:"config,omitempty"`

	// Replicas is the number of replicas for the scheduler.
	Replicas *int32 `json:"replicas,omitempty"`
}

type SchedulerConfigSpec struct {
	// Inline EndpointPickerConfig
	Inline *runtime.RawExtension `json:"inline,omitempty"`

	// Ref is a reference to a ConfigMap key with EndpointPickerConfig.
	Ref *corev1.ConfigMapKeySelector `json:"ref,omitempty"`
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

// ScalingSpec configures autoscaling for the LLM inference deployment.
// When scaling is configured, the controller creates and manages autoscaling resources
// (VariantAutoscaling CR, ServiceMonitor, and the selected actuator — HPA or KEDA ScaledObject).
// +kubebuilder:validation:XValidation:rule="has(self.wva)",message="wva is required when scaling is configured; it provides the autoscaling mechanism"
// +kubebuilder:validation:XValidation:rule="!has(self.minReplicas) || self.minReplicas <= self.maxReplicas",message="minReplicas cannot exceed maxReplicas"
// +kubebuilder:validation:XValidation:rule="!has(self.wva) || !has(self.wva.keda) || !has(self.wva.keda.idleReplicaCount) || has(self.minReplicas)",message="minReplicas is required when idleReplicaCount is set; idleReplicaCount must be less than minReplicas"
// +kubebuilder:validation:XValidation:rule="!has(self.wva) || !has(self.wva.keda) || !has(self.wva.keda.idleReplicaCount) || !has(self.minReplicas) || self.wva.keda.idleReplicaCount < self.minReplicas",message="idleReplicaCount must be less than minReplicas; idleReplicaCount defines the replica floor when no triggers are active"
type ScalingSpec struct {
	// MinReplicas is the minimum number of replicas for the deployment during active scaling.
	// This is the scaling floor when triggers are active.
	// For idle scale-down, use KEDA's idleReplicaCount instead.
	// Defaults to 1 if not specified.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas for the deployment.
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas"`

	// WVA configures the Workload Variant Autoscaler (WVA) for scaling.
	// WVA scales based on a variety of inference metrics (KV cache utilization, queue depth, etc.)
	// rather than traditional CPU/memory metrics.
	// +optional
	WVA *WVASpec `json:"wva,omitempty"`
}

// WVASpec configures the Workload Variant Autoscaler.
type WVASpec struct {
	// VariantCost specifies the cost per replica for this variant (used in saturation analysis).
	// Must be a non-negative numeric string (e.g., "10", "10.0", "0.5").
	// Defaults to "10.0" if not specified.
	// +optional
	// +kubebuilder:validation:Pattern=`^\d+(\.\d+)?$`
	// +kubebuilder:default="10.0"
	VariantCost string `json:"variantCost,omitempty"`

	// ActuatorSpec defines the autoscaling actuator backend (HPA or KEDA).
	// Exactly one of HPA or KEDA must be specified.
	ActuatorSpec `json:",inline"`
}

// ActuatorSpec defines the autoscaling actuator backend for WVA.
// Exactly one of HPA or KEDA must be specified.
// +kubebuilder:validation:XValidation:rule="!(has(self.hpa) && has(self.keda))",message="hpa and keda are mutually exclusive; choose one actuator backend"
// +kubebuilder:validation:XValidation:rule="has(self.hpa) || has(self.keda)",message="either hpa or keda must be specified as the actuator backend"
type ActuatorSpec struct {
	// HPA configures the HorizontalPodAutoscaler as the actuator backend.
	// When specified, HPA reads the wva_desired_replicas metric via the Kubernetes external
	// metrics API (external.metrics.k8s.io) and scales the deployment accordingly.
	// Prerequisite: a Prometheus Adapter must be installed and configured in the cluster to
	// bridge wva_desired_replicas from Prometheus into the Kubernetes external metrics API.
	// Without it, the HPA will fail to read the metric and stop scaling silently.
	// Mutually exclusive with KEDA.
	// +optional
	HPA *HPAScalingSpec `json:"hpa,omitempty"`

	// KEDA configures a KEDA ScaledObject as the actuator backend.
	// When specified, KEDA queries Prometheus directly for the wva_desired_replicas metric
	// and scales the deployment accordingly. Unlike HPA, KEDA does not require a Prometheus
	// Adapter — it connects to Prometheus directly using the URL configured in the
	// autoscaling-wva-controller-config key of the inferenceservice-config ConfigMap.
	// Mutually exclusive with HPA.
	// +optional
	KEDA *KEDAScalingSpec `json:"keda,omitempty"`
}

// HPAScalingSpec configures the HorizontalPodAutoscaler behavior.
// The fields are directly from the upstream Kubernetes autoscaling/v2 API.
//
// Note: HPA-based autoscaling requires a Prometheus Adapter to be pre-installed and
// configured in the cluster. The Prometheus Adapter exposes the wva_desired_replicas
// metric published by WVA into the Kubernetes external metrics API (external.metrics.k8s.io),
// which the HPA reads to make scaling decisions. If the Prometheus Adapter is absent or
// misconfigured, the HPA will enter an Unknown state and scaling will silently stop.
// Consider using KEDA instead, which queries Prometheus directly without an adapter.
type HPAScalingSpec struct {
	// Behavior configures the scaling behavior of the target in both Up and Down directions
	// (scaleUp and scaleDown fields respectively).
	// +optional
	Behavior *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty"`
}

// KEDAScalingSpec configures the KEDA ScaledObject for autoscaling.
// The fields are directly from the upstream KEDA ScaledObject API.
// +kubebuilder:validation:XValidation:rule="!has(self.advanced) || (size(self.advanced.scalingModifiers.formula) == 0 && size(self.advanced.scalingModifiers.target) == 0 && size(self.advanced.scalingModifiers.activationTarget) == 0 && size(self.advanced.scalingModifiers.metricType) == 0)",message="scalingModifiers must not be set; WVA controls the scaling metric formula and logic"
// +kubebuilder:validation:XValidation:rule="!has(self.advanced) || !has(self.advanced.horizontalPodAutoscalerConfig) || size(self.advanced.horizontalPodAutoscalerConfig.name) == 0",message="horizontalPodAutoscalerConfig.name must not be set; the controller manages the HPA name"
type KEDAScalingSpec struct {
	// PollingInterval is the interval in seconds to check each trigger on.
	// Must be at least 1 second.
	// +optional
	// +kubebuilder:validation:Minimum=1
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// CooldownPeriod is the period in seconds to wait after the last trigger reported active
	// before scaling the resource back to its minimum replica count.
	// A value of 0 means scale down immediately with no cooldown.
	// +optional
	// +kubebuilder:validation:Minimum=0
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// InitialCooldownPeriod is the period in seconds to wait after the ScaledObject is created
	// before KEDA starts evaluating triggers. Useful for LLM deployments where the model
	// takes time to load before it can serve traffic, preventing premature scale-up decisions.
	// +optional
	// +kubebuilder:validation:Minimum=0
	InitialCooldownPeriod *int32 `json:"initialCooldownPeriod,omitempty"`

	// IdleReplicaCount is the number of replicas KEDA will scale the resource down to
	// when there are no triggers active. This must be less than minReplicas.
	// If not set, KEDA will not scale below minReplicas.
	// +optional
	// +kubebuilder:validation:Minimum=1
	IdleReplicaCount *int32 `json:"idleReplicaCount,omitempty"`

	// Fallback defines the replica count to maintain when the scaler is in a fallback state
	// (e.g., when Prometheus or WVA metrics are unavailable). This allows the deployment to
	// hold a safe replica count during metric outages rather than scaling to zero.
	// +optional
	Fallback *kedav1alpha1.Fallback `json:"fallback,omitempty"`

	// Advanced specifies the advanced KEDA configuration options.
	// This includes HPA behavior configuration and restore-to-original replica count settings.
	// +optional
	Advanced *kedav1alpha1.AdvancedConfig `json:"advanced,omitempty"`
}

// ParallelismSpec defines the parallelism parameters for distributed inference.
type ParallelismSpec struct {
	// Tensor parallelism size.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Tensor *int32 `json:"tensor,omitempty"`
	// Pipeline parallelism size.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Pipeline *int32 `json:"pipeline,omitempty"`
	// Data parallelism size.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Data *int32 `json:"data,omitempty"`
	// DataLocal data local parallelism size.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DataLocal *int32 `json:"dataLocal,omitempty"`
	// DataRPCPort is the data parallelism RPC port.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	DataRPCPort *int32 `json:"dataRPCPort,omitempty"`
	// Expert enables expert parallelism.
	// +optional
	Expert bool `json:"expert,omitempty"`
}

// UntypedObjectReference is a reference to an object without a specific Group/Version/Kind.
// It's used for referencing networking resources like Gateways and Ingresses where the exact type
// might be inferred or is not strictly required by this controller.
type UntypedObjectReference struct {
	// Name of the referenced object.
	Name gwapiv1.ObjectName `json:"name,omitempty"`
	// Namespace of the referenced object.
	Namespace gwapiv1.Namespace `json:"namespace,omitempty"`
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
