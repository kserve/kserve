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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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

func WithHTTPRouteSpec(spec *gwapiv1.HTTPRouteSpec) LLMInferenceServiceOption {
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
		Name:      gwapiv1.ObjectName(name),
		Namespace: gwapiv1.Namespace(namespace),
	}
}

func HTTPRouteRef(name string) corev1.LocalObjectReference {
	return corev1.LocalObjectReference{
		Name: name,
	}
}
