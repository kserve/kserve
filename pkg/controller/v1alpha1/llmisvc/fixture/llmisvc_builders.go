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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

type LLMInferenceServiceOption ObjectOption[*v1alpha1.LLMInferenceService]

func LLMInferenceService(name string, opts ...LLMInferenceServiceOption) *v1alpha1.LLMInferenceService {
	llmSvc := &v1alpha1.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.LLMInferenceServiceSpec{},
	}

	for _, opt := range opts {
		opt(llmSvc)
	}

	return llmSvc
}

func WithModelURI(uri string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		modelURL, err := apis.ParseURL(uri)
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		llmSvc.Spec.Model.URI = *modelURL
	}
}

func WithModelName(name string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.Model.Name = &name
	}
}

func WithGatewayRefs(refs ...v1alpha1.UntypedObjectReference) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Gateway == nil {
			llmSvc.Spec.Router.Gateway = &v1alpha1.GatewaySpec{}
		}
		llmSvc.Spec.Router.Gateway.Refs = refs
	}
}

func WithHTTPRouteRefs(refs ...corev1.LocalObjectReference) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha1.GatewayRoutesSpec{}
		}
		if llmSvc.Spec.Router.Route.HTTP == nil {
			llmSvc.Spec.Router.Route.HTTP = &v1alpha1.HTTPRouteSpec{}
		}
		llmSvc.Spec.Router.Route.HTTP.Refs = refs
	}
}

func WithHTTPRouteSpec(spec *gatewayapiv1.HTTPRouteSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha1.GatewayRoutesSpec{}
		}
		if llmSvc.Spec.Router.Route.HTTP == nil {
			llmSvc.Spec.Router.Route.HTTP = &v1alpha1.HTTPRouteSpec{}
		}
		llmSvc.Spec.Router.Route.HTTP.Spec = spec
	}
}

func WithManagedGateway() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Gateway == nil {
			llmSvc.Spec.Router.Gateway = &v1alpha1.GatewaySpec{}
		}
	}
}

func WithManagedRoute() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Route == nil {
			llmSvc.Spec.Router.Route = &v1alpha1.GatewayRoutesSpec{}
		}
	}
}

func LLMGatewayRef(name, namespace string) v1alpha1.UntypedObjectReference {
	return v1alpha1.UntypedObjectReference{
		Name:      gatewayapiv1.ObjectName(name),
		Namespace: gatewayapiv1.Namespace(namespace),
	}
}

func HTTPRouteRef(name string) corev1.LocalObjectReference {
	return corev1.LocalObjectReference{
		Name: name,
	}
}

func WithParallelism(parallelism *v1alpha1.ParallelismSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.Parallelism = parallelism
	}
}

func WithPrefillParallelism(parallelism *v1alpha1.ParallelismSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha1.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Parallelism = parallelism
	}
}

func WithReplicas(replicas int32) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.Replicas = &replicas
	}
}

func WithPrefillReplicas(replicas int32) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha1.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Replicas = &replicas
	}
}

func WithTemplate(podSpec *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.Template = podSpec
	}
}

func WithWorker(worker *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.Worker = worker
	}
}

func WithPrefill(pod *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha1.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Template = pod
	}
}

func WithPrefillWorker(worker *corev1.PodSpec) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Prefill == nil {
			llmSvc.Spec.Prefill = &v1alpha1.WorkloadSpec{}
		}
		llmSvc.Spec.Prefill.Worker = worker
	}
}

func ParallelismSpec(opts ...func(*v1alpha1.ParallelismSpec)) *v1alpha1.ParallelismSpec {
	p := &v1alpha1.ParallelismSpec{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithTensorParallelism(tensor int32) func(*v1alpha1.ParallelismSpec) {
	return func(p *v1alpha1.ParallelismSpec) {
		p.Tensor = &tensor
	}
}

func WithPipelineParallelism(pipeline int32) func(*v1alpha1.ParallelismSpec) {
	return func(p *v1alpha1.ParallelismSpec) {
		p.Pipeline = &pipeline
	}
}

func WithDataParallelism(data int32) func(*v1alpha1.ParallelismSpec) {
	return func(p *v1alpha1.ParallelismSpec) {
		p.Data = &data
	}
}

func WithDataLocalParallelism(dataLocal int32) func(*v1alpha1.ParallelismSpec) {
	return func(p *v1alpha1.ParallelismSpec) {
		p.DataLocal = &dataLocal
	}
}

func WithDataRPCPort(rpcPort int32) func(*v1alpha1.ParallelismSpec) {
	return func(p *v1alpha1.ParallelismSpec) {
		p.DataRPCPort = &rpcPort
	}
}

func WithManagedScheduler() LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		llmSvc.Spec.Router.Scheduler = &v1alpha1.SchedulerSpec{}
	}
}

func WithInferencePoolRef(poolName string) LLMInferenceServiceOption {
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		if llmSvc.Spec.Router == nil {
			llmSvc.Spec.Router = &v1alpha1.RouterSpec{}
		}
		if llmSvc.Spec.Router.Scheduler == nil {
			llmSvc.Spec.Router.Scheduler = &v1alpha1.SchedulerSpec{}
		}
		llmSvc.Spec.Router.Scheduler.Pool = &v1alpha1.InferencePoolSpec{
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
	return func(llmSvc *v1alpha1.LLMInferenceService) {
		llmSvc.Spec.BaseRefs = refs
	}
}

type LLMInferenceServiceConfigOption ObjectOption[*v1alpha1.LLMInferenceServiceConfig]

func LLMInferenceServiceConfig(name string, opts ...LLMInferenceServiceConfigOption) *v1alpha1.LLMInferenceServiceConfig {
	config := &v1alpha1.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.LLMInferenceServiceSpec{},
	}

	for _, opt := range opts {
		opt(config)
	}

	return config
}

func WithConfigModelURI(uri string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha1.LLMInferenceServiceConfig) {
		modelURL, err := apis.ParseURL(uri)
		if err != nil {
			panic(err) // For test fixtures, panic is acceptable
		}
		config.Spec.Model.URI = *modelURL
	}
}

func WithConfigModelName(name string) LLMInferenceServiceConfigOption {
	return func(config *v1alpha1.LLMInferenceServiceConfig) {
		config.Spec.Model.Name = &name
	}
}

func WithConfigManagedRouter() LLMInferenceServiceConfigOption {
	return func(config *v1alpha1.LLMInferenceServiceConfig) {
		config.Spec.Router = &v1alpha1.RouterSpec{
			Gateway:   &v1alpha1.GatewaySpec{},
			Route:     &v1alpha1.GatewayRoutesSpec{},
			Scheduler: &v1alpha1.SchedulerSpec{},
		}
	}
}

func WithConfigWorkloadTemplate(podSpec *corev1.PodSpec) LLMInferenceServiceConfigOption {
	return func(config *v1alpha1.LLMInferenceServiceConfig) {
		config.Spec.Template = podSpec
	}
}
