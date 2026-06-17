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
	"k8s.io/apimachinery/pkg/api/resource"
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
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type LLMInferenceServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec         `json:"spec,omitempty"`
	Status LLMInferenceServiceConfigStatus `json:"status,omitempty"`
}

// LLMInferenceServiceConfigStatus defines the observed state of LLMInferenceServiceConfig.
type LLMInferenceServiceConfigStatus struct {
	// Conditions of the resource.
	duckv1.Status `json:",inline"`

	// ReferencedBy lists the LLMInferenceService instances that reference this config
	// via spec.baseRefs, status.annotations, or implicitly as a well-known default.
	// +optional
	// +listType=atomic
	ReferencedBy []UntypedObjectReference `json:"referencedBy,omitempty"`
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

	// Tracing configuration for distributed tracing across all managed components.
	// When present (even as `{}`), distributed tracing is enabled with defaults.
	// When omitted, no tracing instrumentation is injected.
	// The controller propagates this configuration to every managed component (inference server and scheduler),
	// automatically adjusting the service name per component (e.g. "-decode"/"-prefill" suffix for inference servers,
	// "inference-scheduler" for the scheduler).
	// +optional
	Tracing *TracingSpec `json:"tracing,omitempty"`

	// Speculator configures speculative decoding for the model server.
	// When specified with a model URI, the controller creates a second storage-initializer
	// init container to download the speculator (or draft) model and mounts it into the
	// inference container. The config map is passed directly to vLLM's --speculative-config.
	// +optional
	Speculator *SpeculatorSpec `json:"speculator,omitempty"`

	// BaseRefs allows inheriting and overriding configurations from one or more LLMInferenceServiceConfig instances.
	// The controller merges these base configurations, with the current LLMInferenceService spec taking the highest precedence.
	// When multiple baseRefs are provided, the last one in the list overrides previous ones.
	// +optional
	BaseRefs []corev1.LocalObjectReference `json:"baseRefs,omitempty"`
}

// SpeculatorSpec configures speculative decoding for the inference server.
// Speculative decoding uses a fast draft mechanism (either a small draft model or a purpose-trained
// speculator head like Eagle3) to propose candidate tokens that the target model verifies in parallel,
// reducing the number of sequential decode steps and improving token generation throughput.
//
// When configured with a model, the controller:
//  1. Creates a second storage-initializer init container to download the speculator/draft model
//  2. Mounts the downloaded model into the inference container at /mnt/speculator/model
//  3. Injects the appropriate --speculative-config arguments into the vLLM command line
//
// The Config map keys correspond directly to vLLM's --speculative-config JSON schema
// (see https://docs.vllm.ai/en/latest/features/speculative_decoding/#-speculative-config-schema).
//
// Example - Eagle3 speculator head:
//
//	speculator:
//	  model:
//	    uri: hf://RedHatAI/Qwen3-32B-speculator.eagle3
//	  config:
//	    method: eagle3
//	    num_speculative_tokens: "3"
//
// Example - Draft-target model pair:
//
//	speculator:
//	  model:
//	    uri: hf://meta-llama/Llama-3.2-1B-Instruct
//	  config:
//	    method: draft_model
//	    num_speculative_tokens: "5"
//	    max_model_len: "8192"
//	    draft_tensor_parallel_size: "1"
//
// Example - N-gram (no model needed):
//
//	speculator:
//	  config:
//	    method: ngram
//	    num_speculative_tokens: "4"
//	    prompt_lookup_max: "5"
//
// +kubebuilder:validation:XValidation:rule="!has(self.model) || !has(self.model.lora)",message="speculator.model.lora is not supported; LoRA adapters apply only to the base model"
// +kubebuilder:validation:XValidation:rule="!has(self.config) || (size(self.config) == 0) || ('method' in self.config)",message="speculator.config.method is required; it specifies the speculative decoding strategy (e.g. eagle3, draft_model, ngram, mtp)"
type SpeculatorSpec struct {
	// Model specification for the speculator or draft model.
	// The URI specifies the location of the model to download (e.g., hf://RedHatAI/Qwen3-32B-speculator.eagle3).
	// The controller creates a dedicated storage-initializer init container to fetch this model.
	// Not required for methods that don't use a separate model (e.g., ngram, mtp).
	// +optional
	Model *LLMModelSpec `json:"model,omitempty"`

	// Config provides the speculative decoding parameters passed directly to the
	// vLLM --speculative-config JSON. Keys correspond to vLLM's speculative config schema.
	// Common keys: method, num_speculative_tokens, max_model_len, draft_tensor_parallel_size.
	// See https://docs.vllm.ai/en/latest/features/speculative_decoding/#-speculative-config-schema
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// WorkloadSpec defines the configuration for a deployment workload, such as replicas and pod specifications.
// +kubebuilder:validation:XValidation:rule="!(has(self.replicas) && has(self.scaling))",message="replicas and scaling are mutually exclusive; use scaling for autoscaled deployments or replicas for static deployments"
type WorkloadSpec struct {
	// Number of replicas for the deployment.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Scaling configuration for autoscaling this workload.
	// When specified, the controller creates and manages autoscaling resources
	// (ServiceMonitor and the selected actuator — HPA or KEDA ScaledObject, annotated for WVA discovery)
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
	//
	// For the storage-initializer init container (named "storage-initializer"), users may
	// customize fields such as env, resources, volumeMounts, and image. The container's
	// name, args, and command are controller-managed and any user-provided values for
	// these fields will be overridden. To disable the storage-initializer entirely, use
	// spec.storageInitializer.enabled=false.
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`

	// Worker configuration for multi-node deployments.
	// The presence of this field triggers the creation of a multi-node (distributed) setup.
	// This spec defines the configuration for the worker pods, while the main 'Template' field defines the head pod.
	// The controller is responsible for enabling discovery between head and worker pods.
	// +optional
	Worker *corev1.PodSpec `json:"worker,omitempty"`

	// KVCacheOffloading configures multi-tier KV cache CPU offloading for this workload.
	// The controller translates this into --kv-transfer-config for the vLLM serve command.
	// +optional
	KVCacheOffloading *KVCacheOffloadingSpec `json:"kvCacheOffloading,omitempty"`
}

// KVCacheOffloadingSpec configures KV cache offloading via vLLM's OffloadingConnector.
type KVCacheOffloadingSpec struct {
	// CPU is the amount of CPU RAM to allocate as the primary KV cache tier
	// (maps to vLLM kv_connector_extra_config.cpu_bytes_to_use). Accepts standard
	// Kubernetes quantity notation, e.g. "10Gi".
	CPU resource.Quantity `json:"cpu"`

	// EvictionPolicy for the primary CPU KV cache tier. Defaults to "lru".
	// +optional
	// +kubebuilder:validation:Enum=lru;arc
	// +kubebuilder:default=lru
	EvictionPolicy string `json:"evictionPolicy,omitempty"`

	// Secondary is an ordered list of secondary KV cache tiers. vLLM cascades
	// through tiers in the order listed. Currently only fileSystem tiers are
	// supported; the array shape is designed to accommodate object store tiers
	// in a future follow-up without an API break.
	// +optional
	Secondary []SecondaryTierSpec `json:"secondary,omitempty"`
}

// SecondaryTierSpec defines one secondary KV cache tier.
// Exactly one field must be set.
type SecondaryTierSpec struct {
	// FileSystem configures a POSIX disk tier backed by a Kubernetes volume.
	// +optional
	FileSystem *FileSystemTierSpec `json:"fileSystem,omitempty"`
}

// FileSystemTierSpec configures a POSIX disk secondary tier.
// Exactly one of EmptyDir or PVC must be set.
type FileSystemTierSpec struct {
	// EmptyDir uses a node-local ephemeral emptyDir volume. No StorageClass required.
	// Each pod gets its own independent, node-local disk cache. The controller also
	// adds an ephemeral-storage resource request to the main container equal to the
	// size so the scheduler accounts for the disk space when placing the pod.
	// +optional
	EmptyDir *EmptyDirTierSpec `json:"emptyDir,omitempty"`

	// PVC configures a PVC-backed disk cache tier. Either Spec (for a controller-managed
	// ephemeral PVC per pod) or Ref (for a pre-existing user-managed PVC) must be set,
	// but not both.
	// +optional
	PVC *PVCTierSpec `json:"pvc,omitempty"`
}

// EmptyDirTierSpec configures a node-local ephemeral emptyDir volume as a KV cache tier.
type EmptyDirTierSpec struct {
	// Size is the maximum storage capacity for this tier (maps to emptyDir.sizeLimit).
	// The controller also requests this amount as ephemeral-storage on the main container.
	Size resource.Quantity `json:"size"`
}

// PVCTierSpec configures a PVC-backed KV cache tier.
// Exactly one of Spec or Ref must be set.
type PVCTierSpec struct {
	// Spec creates one ephemeral PVC per pod automatically. The PVC is owned by the pod
	// and deleted when the pod is deleted — it does not survive pod restarts.
	// +optional
	Spec *corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`

	// Ref mounts a pre-existing user-managed PVC as the disk cache. The PVC must
	// already exist. For a cache shared across replicas, provision an RWX PVC;
	// for per-pod persistent cache, provision a separate PVC per pod and reference
	// it by name.
	// +optional
	Ref *PVCRefTierSpec `json:"ref,omitempty"`
}

// PVCRefTierSpec mounts a pre-existing user-managed PVC as a KV cache tier.
type PVCRefTierSpec struct {
	// Name of the pre-existing PersistentVolumeClaim.
	Name string `json:"name"`

	// Path is a subdirectory within the PVC used as VolumeMount.subPath.
	// +optional
	Path string `json:"path,omitempty"`
}

// ConfidentialSpec enables confidential model serving with encrypted model artifacts.
// When enabled, the storage initializer will decrypt model files using keys obtained
// from a Key Broker Service (KBS) via TEE attestation.
type ConfidentialSpec struct {
	// Enabled controls whether confidential model serving is active.
	// When true, the confidential storage initializer image is used and
	// encrypted model artifacts are decrypted after download.
	Enabled bool `json:"enabled"`
	// ResourceId is the KBS resource identifier for the decryption key,
	// in the format kbs:///<repo>/<type>/<tag>.
	// +optional
	ResourceId *string `json:"resourceId,omitempty"`
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

	// Confidential enables confidential model serving with encrypted model artifacts.
	// When enabled, the storage initializer decrypts model files using keys obtained
	// from a Key Broker Service (KBS) via TEE attestation.
	// +optional
	Confidential *ConfidentialSpec `json:"confidential,omitempty"`
}

// LoRASpec defines the configuration for LoRA adapters.
type LoRASpec struct {
	// Adapters is a list of LoRA (Low-Rank Adaptation) adapters to attach to the base model.
	// Each adapter is specified by name and URI (supports hf://, s3://, and pvc:// schemes).
	// The controller automatically downloads adapters and configures the runtime to use them.
	// +optional
	// This type is recursive https://github.com/kubernetes-sigs/controller-tools/issues/585
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Adapters []LLMModelSpec `json:"adapters,omitempty"`

	// MaxRank is the maximum LoRA rank supported by the runtime (maps to vLLM --max-lora-rank).
	// Higher values allow adapters with higher rank but increase memory usage.
	// If not set, vLLM's default applies (16).
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxRank *int32 `json:"maxRank,omitempty"`

	// MaxAdapters is the maximum number of LoRA adapters that can be loaded simultaneously
	// (maps to vLLM --max-loras). Defaults to the number of configured adapters.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxAdapters *int32 `json:"maxAdapters,omitempty"`

	// MaxCpuAdapters is the maximum number of LoRA adapters stored in CPU memory
	// (maps to vLLM --max-cpu-loras). Defaults to the number of configured adapters.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxCpuAdapters *int32 `json:"maxCpuAdapters,omitempty"`
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

	// Group identifies the routing group this LLMISVC belongs to.
	// All members with the same group share weighted traffic distribution.
	// Must be explicit - not inferred from spec.model.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Group *string `json:"group,omitempty"`

	// Weight is the proportional traffic share for this member.
	// Follows Gateway API backendRef weight semantics (proportional, not percentage).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000000
	Weight *int32 `json:"weight,omitempty"`
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
	Refs []GatewayObjectReference `json:"refs,omitempty"`
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
// request Scheduler, also known as the Endpoint Picker (EPP). The EPP determines the exact pod that should handle the
// request and responds back to the Gateway with the target pod. The Gateway will then forward the request to the chosen pod.
//
// The Scheduler is only effective when having multiple inference pod replicas.
//
// Step 1 (endpoint selection): Gateway <-- ExtProc --> EPP (select the optimal replica to handle the request)
// Step 2 (endpoint routing): Gateway <-- forward request/response --> Inference Pod X
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

	// Tokenizer provides optional operational overrides for the standalone
	// tokenizer deployment that serves the vLLM render endpoint
	// (/v1/completions/render) over HTTP. The tokenizer is auto-deployed when
	// token-producer or precise-prefix-cache-scorer plugins are detected in the
	// scheduler config.inline. Use this field to customize the tokenizer pod
	// template (resources, image overrides, etc.).
	// +optional
	Tokenizer *TokenizerSpec `json:"tokenizer,omitempty"`
}

// TokenizerSpec configures a standalone tokenizer deployment.
type TokenizerSpec struct {
	// Template for the tokenizer pod spec. This is merged on top of the
	// well-known tokenizer config defaults (vLLM render container).
	// +optional
	Template *corev1.PodSpec `json:"template,omitempty"`
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
// (ServiceMonitor and the selected actuator — HPA or KEDA ScaledObject, annotated for WVA discovery).
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

// TracingSpec defines the distributed tracing configuration.
// When present (even as an empty object `{}`), tracing is enabled with sensible defaults.
// When omitted, no tracing instrumentation is injected.
//
// Example - Enable tracing with defaults:
//
//	tracing: {}
//
// Example - Custom configuration:
//
//	tracing:
//	  exporterEndpoint: "http://my-collector:4317"
//	  sampler: "parentbased_traceidratio"
//	  samplerArg: "0.1"
//	  exporter: "otlp"
type TracingSpec struct {
	// ExporterEndpoint is the OTLP exporter endpoint.
	// Maps to the OTEL_EXPORTER_OTLP_ENDPOINT environment variable.
	// Default: "http://otel-collector:4317"
	// +optional
	ExporterEndpoint *string `json:"exporterEndpoint,omitempty"`

	// Sampler specifies the sampler to use for traces.
	// Maps to the OTEL_TRACES_SAMPLER environment variable.
	// Default: "parentbased_traceidratio"
	// +optional
	Sampler *string `json:"sampler,omitempty"`

	// SamplerArg is an argument passed to the traces sampler (e.g. the sampling ratio).
	// Maps to the OTEL_TRACES_SAMPLER_ARG environment variable.
	// Default: "0.05" (5% sampling rate)
	// +optional
	SamplerArg *string `json:"samplerArg,omitempty"`

	// Exporter specifies which exporter is used for traces.
	// Maps to the OTEL_TRACES_EXPORTER environment variable.
	// Default: "otlp"
	// +optional
	Exporter *string `json:"exporter,omitempty"`
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
// It's used for referencing networking resources like Ingresses where the exact type
// might be inferred or is not strictly required by this controller.
type UntypedObjectReference struct {
	// Name of the referenced object.
	Name gwapiv1.ObjectName `json:"name,omitempty"`
	// Namespace of the referenced object.
	Namespace gwapiv1.Namespace `json:"namespace,omitempty"`
}

// GatewayObjectReference is a reference to a Gateway resource.
// It extends UntypedObjectReference with Gateway-specific fields.
type GatewayObjectReference struct {
	UntypedObjectReference `json:",inline"`
	// SectionName is the name of a section within the target resource. When
	// set on a Gateway reference, it targets a specific listener by name.
	// When unset, the route is attached to all listeners on the referenced
	// Gateway that support the route type.
	// +optional
	SectionName *gwapiv1.SectionName `json:"sectionName,omitempty"`
}

// ObservedGateway is a Gateway reference with the listeners and HTTPRoutes
// bound to this service through it. Used in status to record observed routing topology.
type ObservedGateway struct {
	// Embedded ObjectReference carries group, kind, name, namespace of the Gateway.
	gwapiv1.ObjectReference `json:",inline"`

	// Listeners lists the SectionNames of the Gateway listeners that accepted
	// routes from this service. Nil means the route targets all listeners
	// (no SectionName was specified on the parentRef).
	// +optional
	// +listType=atomic
	Listeners []gwapiv1.SectionName `json:"listeners,omitempty"`

	// HTTPRoutes lists the HTTPRoutes bound to this service through this Gateway.
	// +optional
	// +listType=atomic
	HTTPRoutes []gwapiv1.ObjectReference `json:"httpRoutes,omitempty"`
}

// ObservedSchedulerStatus records the scheduler-related resources observed
// during the last successful routing reconciliation.
type ObservedSchedulerStatus struct {
	// InferencePool is the InferencePool observed as active for this service.
	// +optional
	InferencePool *gwapiv1.ObjectReference `json:"inferencePool,omitempty"`

	// Service is the EPP Service observed for this service.
	// +optional
	Service *gwapiv1.ObjectReference `json:"service,omitempty"`
}

// RouterStatus records the networking resources observed during the last
// successful routing reconciliation. Nil when routing is not configured or
// the service is stopped.
type RouterStatus struct {
	// Gateways lists the Gateway resources observed as attached to this service,
	// each with the listeners and HTTPRoutes bound through them.
	// +optional
	// +listType=atomic
	Gateways []ObservedGateway `json:"gateways,omitempty"`

	// Scheduler records the observed scheduler topology.
	// Nil when the scheduler is not configured.
	// +optional
	Scheduler *ObservedSchedulerStatus `json:"scheduler,omitempty"`

	// Group reports the observed routing group topology.
	// Nil when this LLMISVC is not part of a routing group.
	// +optional
	Group *GroupStatus `json:"group,omitempty"`
}

// GroupStatus reports the observed state of the routing group this LLMISVC belongs to.
type GroupStatus struct {
	// Name of the routing group.
	Name string `json:"name"`

	// Members lists all observed members of the group with their weights
	// and resolved backend references.
	// +optional
	// +listType=map
	// +listMapKey=name
	Members []GroupMemberStatus `json:"members,omitempty"`
}

// GroupMemberStatus reports the observed state of a single group member.
type GroupMemberStatus struct {
	// Name of the group member (LLMInferenceService name).
	Name string `json:"name"`

	// Weight is the effective traffic weight for this member.
	Weight int32 `json:"weight"`

	// Stopped is true when the member has the force-stop annotation set.
	// A stopped member remains in the group but receives no traffic (weight
	// is overridden to 0 regardless of the spec value).
	// +optional
	Stopped bool `json:"stopped,omitempty"`

	// BackendRef is the resolved backend for this member
	// (InferencePool or Service).
	// +optional
	BackendRef *gwapiv1.BackendObjectReference `json:"backendRef,omitempty"`
}

// ObservedWorkloadStatus identifies a workload resource and its observed replica state.
type ObservedWorkloadStatus struct {
	corev1.TypedLocalObjectReference `json:",inline"`

	// ReadyReplicas is the number of pods available to serve traffic.
	// Copied from the workload's status on each reconcile.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`
}

// WorkloadStatus records the workload resources observed during the last
// successful reconciliation. Nil when no workload resources have been
// created yet, or when the service is stopped.
// +optional
type WorkloadStatus struct {
	// Primary is the main inference workload (Deployment or LeaderWorkerSet).
	// When disaggregated serving is configured, this workload handles
	// the decode phase; otherwise it handles both prefill and decode.
	// +optional
	Primary *ObservedWorkloadStatus `json:"primary,omitempty"`

	// Prefill is the prefill workload in disaggregated serving mode.
	// Nil when disaggregated serving is not configured.
	// +optional
	Prefill *ObservedWorkloadStatus `json:"prefill,omitempty"`

	// Service is the Kubernetes Service fronting the primary inference workload.
	// +optional
	Service *corev1.TypedLocalObjectReference `json:"service,omitempty"`

	// Scheduler is the EPP scheduler Deployment.
	// Nil when the scheduler is not configured.
	// +optional
	Scheduler *ObservedWorkloadStatus `json:"scheduler,omitempty"`
}

// SourcedAddress extends Addressable with the networking resource that
// produced this address, enabling consumers to select endpoints by origin.
type SourcedAddress struct {
	duckv1.Addressable `json:",inline"`

	// Origin identifies the networking resource (e.g., Gateway) that
	// produced this address. Nil when the origin is unknown or when
	// the address was converted from an older API version.
	// +optional
	Origin *gwapiv1.ObjectReference `json:"origin,omitempty"`

	Models []ModelSourcedAddressStatus `json:"models,omitempty"`
}

// AppliedConfigSource identifies how a configuration was selected for merging.
// +kubebuilder:validation:Enum=Preset;UserRef
type AppliedConfigSource string

const (
	// AppliedConfigSourcePreset indicates the config was automatically injected
	// by the controller based on the deployment pattern (single-node, multi-node,
	// disaggregated, scheduler, router).
	AppliedConfigSourcePreset AppliedConfigSource = "Preset"
	// AppliedConfigSourceUserRef indicates the config was explicitly referenced
	// by the user via spec.baseRefs.
	AppliedConfigSourceUserRef AppliedConfigSource = "UserRef"
)

// AppliedConfigRef identifies an LLMInferenceServiceConfig resource that contributed
// to the final merged configuration during reconciliation.
type AppliedConfigRef struct {
	// Name of the LLMInferenceServiceConfig resource that was applied.
	// +required
	Name gwapiv1.ObjectName `json:"name"`
	// Namespace where the LLMInferenceServiceConfig was resolved from.
	// +required
	Namespace gwapiv1.Namespace `json:"namespace"`
	// Source indicates how this config was selected - either automatically injected
	// as a well-known default based on the deployment pattern, or explicitly
	// referenced via spec.baseRefs.
	// +required
	Source AppliedConfigSource `json:"source"`
}

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService.
type LLMInferenceServiceStatus struct {
	// URL is the primary address for accessing the service.
	// It is set to an external (public) address when available, otherwise
	// it is promoted from the first discovered address (which may be
	// cluster-local or private) for easy discovery.
	// +optional
	URL *apis.URL `json:"url,omitempty"`

	// Conditions of the resource.
	duckv1.Status `json:",inline"`

	// Deprecated: Address is retained for CRD schema compatibility.
	// It is never populated; use Addresses instead.
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`

	// Addresses lists the network endpoints where the service is reachable.
	// Each address may include an origin reference identifying the networking
	// resource that produced it.
	// +optional
	// +listType=atomic
	Addresses []SourcedAddress `json:"addresses,omitempty"`

	// Router records the observed networking topology for this service.
	// Nil when routing is not configured or the service is stopped.
	// +optional
	Router *RouterStatus `json:"router,omitempty"`

	// Workloads records the observed workload resources for this service.
	// +optional
	Workloads *WorkloadStatus `json:"workloads,omitempty"`

	// AppliedConfigRefs records which LLMInferenceServiceConfig resources were applied
	// during the last successful reconciliation, in merge precedence order.
	// Well-known configs (determined by the deployment pattern) appear first with
	// lower precedence, followed by explicitly referenced baseRefs with higher
	// precedence. The service's own spec always takes the highest precedence but
	// is not listed here.
	// +optional
	// Atomic because the controller always writes the full list and ordering encodes merge precedence.
	// +listType=atomic
	AppliedConfigRefs []AppliedConfigRef `json:"appliedConfigs,omitempty"`
}

type ModelSourcedAddressStatus struct {
	Name string `json:"name"`
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
