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

package fixture

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

type LLMInferenceServiceOption ObjectOption[*v1alpha2.LLMInferenceService]

func LLMInferenceService(name string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{},
	}

	for _, opt := range opts {
		opt(llmSvc)
	}

	return llmSvc
}

func WithModelURI(uri string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		modelURL, err := apis.ParseURL(uri)
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		llmSvc.Spec.Model.URI = *modelURL
	}
}

func WithModelName(name string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.Model.Name = &name
	}
}

func WithGatewayRefs(refs ...v1alpha2.UntypedObjectReference) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Gateway == nil {
			llmSvc.Spec.Router.Gateway = &v1alpha2.GatewaySpec{}
		}
		llmSvc.Spec.Router.Gateway.Refs = refs
	}
}

func WithHTTPRouteRefs(refs ...corev1.LocalObjectReference) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha2.GatewayRoutesSpec{}
		}
		if llmSvc.Spec.Router.Route.HTTP == nil {
			llmSvc.Spec.Router.Route.HTTP = &v1alpha2.HTTPRouteSpec{}
		}
		llmSvc.Spec.Router.Route.HTTP.Refs = refs
	}
}

func WithHTTPRouteSpec(spec *gwapiv1.HTTPRouteSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha2.GatewayRoutesSpec{}
		}
		if llmSvc.Spec.Router.Route.HTTP == nil {
			llmSvc.Spec.Router.Route.HTTP = &v1alpha2.HTTPRouteSpec{}
		}
		llmSvc.Spec.Router.Route.HTTP.Spec = spec
	}
}

func WithManagedGateway() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Gateway == nil {
			llmSvc.Spec.Router.Gateway = &v1alpha2.GatewaySpec{}
		}
	}
}

func WithManagedRoute() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha2.GatewayRoutesSpec{}
		}
	}
}

func WithAnnotations(annotationsToAdd map[string]string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Annotations == nil {
			llmSvc.Annotations = make(map[string]string)
		}
		maps.Copy(llmSvc.Annotations, annotationsToAdd)
	}
}

func WithLabels(labelsToAdd map[string]string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Labels == nil {
			llmSvc.Labels = make(map[string]string)
		}
		maps.Copy(llmSvc.Labels, labelsToAdd)
	}
}

func LLMGatewayRef(name, namespace string) v1alpha2.UntypedObjectReference {
	return v1alpha2.UntypedObjectReference{
		Name:      gwapiv1.ObjectName(name),
		Namespace: gwapiv1.Namespace(namespace),
	}
}

func HTTPRouteRef(name string) corev1.LocalObjectReference {
	return corev1.LocalObjectReference{
		Name: name,
	}
}

func WithParallelism(parallelism *v1alpha2.ParallelismSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.Parallelism = parallelism
	}
}

func WithPrefillParallelism(parallelism *v1alpha2.ParallelismSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Parallelism = parallelism
	}
}

func WithReplicas(replicas int32) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.Replicas = &replicas
	}
}

func WithPrefillReplicas(replicas int32) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Replicas = &replicas
	}
}

func WithTemplate(podSpec *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.Template = podSpec
	}
}

func WithWorker(worker *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.Worker = worker
	}
}

func WithPrefill(pod *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Template = pod
	}
}

func WithPrefillWorker(worker *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Worker = worker
	}
}

func ParallelismSpec(opts ...func(*v1alpha2.ParallelismSpec)) *v1alpha2.ParallelismSpec {
	p := &v1alpha2.ParallelismSpec{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithTensorParallelism(tensor int32) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.Tensor = &tensor
	}
}

func WithPipelineParallelism(pipeline int32) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.Pipeline = &pipeline
	}
}

func WithDataParallelism(data int32) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.Data = &data
	}
}

func WithDataLocalParallelism(dataLocal int32) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.DataLocal = &dataLocal
	}
}

func WithDataRPCPort(rpcPort int32) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.DataRPCPort = &rpcPort
	}
}

func WithExpert(expert bool) func(*v1alpha2.ParallelismSpec) {
	return func(p *v1alpha2.ParallelismSpec) {
		p.Expert = expert
	}
}

func WithManagedScheduler() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		llmSvc.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{}
	}
}

func WithInferencePoolRef(poolName string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if llmSvc.Spec.Router.Scheduler == nil {
			llmSvc.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{}
		}
		llmSvc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
			Ref: &corev1.LocalObjectReference{
				Name: poolName,
			},
		}
	}
}

func SimpleWorkerPodSpec() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "main",
				Image: "test-worker:latest",
			},
		},
	}
}

func WithBaseRefs(refs ...corev1.LocalObjectReference) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		llmSvc.Spec.BaseRefs = refs
	}
}

type LLMInferenceServiceConfigOption ObjectOption[*v1alpha2.LLMInferenceServiceConfig]

func LLMInferenceServiceConfig(name string, opts ...LLMInferenceServiceConfigOption) *v1alpha2.LLMInferenceServiceConfig {
	config := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{},
	}

	for _, opt := range opts {
		opt(config)
	}

	return config
}

func WithConfigModelURI(uri string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		modelURL, err := apis.ParseURL(uri)
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		config.Spec.Model.URI = *modelURL
	}
}

func WithConfigModelName(name string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		config.Spec.Model.Name = &name
	}
}

func WithConfigManagedRouter() LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		config.Spec.Router = &v1alpha2.RouterSpec{
			Gateway:   &v1alpha2.GatewaySpec{},
			Route:     &v1alpha2.GatewayRoutesSpec{},
			Scheduler: &v1alpha2.SchedulerSpec{},
		}
	}
}

func WithConfigManagedScheduler() LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		if config.Spec.Router == nil {
			config.Spec.Router = &v1alpha2.RouterSpec{}
		}
		config.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{}
	}
}

func WithConfigWorkloadTemplate(podSpec *corev1.PodSpec) LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		config.Spec.Template = podSpec
	}
}

// WithConfigSchedulerConfigInline sets an inline scheduler config on the LLMInferenceServiceConfig.
// The configYAML is converted to JSON since RawExtension.Raw expects JSON bytes.
func WithConfigSchedulerConfigInline(configYAML string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		ensureSchedulerSpec(&config.Spec)
		jsonBytes, err := yaml.YAMLToJSON([]byte(configYAML))
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		config.Spec.Router.Scheduler.Config = &v1alpha2.SchedulerConfigSpec{
			Inline: &runtime.RawExtension{Raw: jsonBytes},
		}
	}
}

// WithConfigSchedulerConfigRef sets a ConfigMap reference for scheduler config on LLMInferenceServiceConfig.
// If key is empty, it defaults to "epp" at runtime.
func WithConfigSchedulerConfigRef(configMapName, key string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha2.LLMInferenceServiceConfig) {
		ensureSchedulerSpec(&config.Spec)
		ref := &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
		}
		if key != "" {
			ref.Key = key
		}
		config.Spec.Router.Scheduler.Config = &v1alpha2.SchedulerConfigSpec{
			Ref: ref,
		}
	}
}

// WithSchedulerConfigInline sets an inline scheduler config on the LLMInferenceService.
// The configYAML is converted to JSON since RawExtension.Raw expects JSON bytes.
func WithSchedulerConfigInline(configYAML string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		ensureSchedulerSpec(&llmSvc.Spec)
		jsonBytes, err := yaml.YAMLToJSON([]byte(configYAML))
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		llmSvc.Spec.Router.Scheduler.Config = &v1alpha2.SchedulerConfigSpec{
			Inline: &runtime.RawExtension{Raw: jsonBytes},
		}
	}
}

// WithSchedulerConfigRef sets a ConfigMap reference for scheduler config.
// If key is empty, it defaults to "epp" at runtime.
func WithSchedulerConfigRef(configMapName, key string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha2.LLMInferenceService) {
		ensureSchedulerSpec(&llmSvc.Spec)
		ref := &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
		}
		if key != "" {
			ref.Key = key
		}
		llmSvc.Spec.Router.Scheduler.Config = &v1alpha2.SchedulerConfigSpec{
			Ref: ref,
		}
	}
}

// ensureSchedulerSpec ensures the router and scheduler spec are initialized
func ensureSchedulerSpec(spec *v1alpha2.LLMInferenceServiceSpec) {
	if spec.Router == nil {
		spec.Router = &v1alpha2.RouterSpec{}
	}
	if spec.Router.Scheduler == nil {
		spec.Router.Scheduler = &v1alpha2.SchedulerSpec{}
	}
}

// SchedulerConfigMap creates a ConfigMap with scheduler config data using the default "epp" key
func SchedulerConfigMap(name, namespace, configData string) *corev1.ConfigMap {
	return SchedulerConfigMapWithKey(name, namespace, "epp", configData)
}

// SchedulerConfigMapWithKey creates a ConfigMap with scheduler config data using a custom key
func SchedulerConfigMapWithKey(name, namespace, key, configData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			key: configData,
		},
	}
}
