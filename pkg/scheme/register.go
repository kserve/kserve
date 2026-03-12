/*
Copyright 2026 The KServe Authors.

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

package scheme

import (
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	wvav1alpha1 "github.com/llm-d/llm-d-workload-variant-autoscaler/api/v1alpha1"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/pkg/errors"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"

	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type addToSchemeFunc func(scheme *runtime.Scheme) error

// AddKServeAPIs registers all KServe APIs.
func AddKServeAPIs(s *runtime.Scheme) error {
	return addAll(s,
		v1alpha1.AddToScheme,
		v1alpha2.AddToScheme,
		v1beta1.AddToScheme,
	)
}

// AddCoreKubernetesAPIs registers core Kubernetes APIs used by controllers and tests.
func AddCoreKubernetesAPIs(s *runtime.Scheme) error {
	return addAll(s,
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		appsv1.AddToScheme,
		batchv1.AddToScheme,
		autoscalingv2.AddToScheme,
		apiextv1.AddToScheme,
		netv1.AddToScheme,
	)
}

// AddGatewayAPIs registers Gateway API and inference extension APIs.
func AddGatewayAPIs(s *runtime.Scheme) error {
	return addAll(s,
		gwapiv1.Install,
		igwapi.Install,
		igwapiv1alpha2.Install,
	)
}

// AddLeaderWorkerSetAPIs registers LeaderWorkerSet APIs.
func AddLeaderWorkerSetAPIs(s *runtime.Scheme) error {
	return addAll(s, lwsapi.AddToScheme)
}

// AddKnativeAPIs registers Knative Serving APIs.
func AddKnativeAPIs(s *runtime.Scheme) error {
	return addAll(s, knservingv1.AddToScheme)
}

// AddIstioAPIs registers Istio networking APIs.
func AddIstioAPIs(s *runtime.Scheme) error {
	return addAll(s, istioclientv1beta1.AddToScheme)
}

// AddKedaAPIs registers KEDA APIs.
func AddKedaAPIs(s *runtime.Scheme) error {
	return addAll(s, kedav1alpha1.AddToScheme)
}

// AddWVAAPIs registers WVA (Workload Variant Autoscaler) APIs.
func AddWVAAPIs(s *runtime.Scheme) error {
	return addAll(s, wvav1alpha1.AddToScheme)
}

// AddOpenTelemetryAPIs registers OpenTelemetry operator APIs.
func AddOpenTelemetryAPIs(s *runtime.Scheme) error {
	return addAll(s, otelv1beta1.AddToScheme)
}

// AddControllerAPIs registers the baseline controller APIs used by production and tests.
func AddControllerAPIs(s *runtime.Scheme) error {
	return addAll(s,
		AddKServeAPIs,
		AddCoreKubernetesAPIs,
	)
}

// AddLLMISVCAPIs registers API groups required by the llmisvc manager.
func AddLLMISVCAPIs(s *runtime.Scheme) error {
	return addAll(s,
		AddControllerAPIs,
		AddGatewayAPIs,
		AddLeaderWorkerSetAPIs,
		AddKedaAPIs,
		AddWVAAPIs,
	)
}

// AddAll registers all API groups supported by KServe managers and envtest suites.
func AddAll(s *runtime.Scheme) error {
	return addAll(s,
		AddControllerAPIs,
		AddGatewayAPIs,
		AddLeaderWorkerSetAPIs,
		AddKnativeAPIs,
		AddIstioAPIs,
		AddKedaAPIs,
		AddWVAAPIs,
		AddOpenTelemetryAPIs,
	)
}

func addAll(s *runtime.Scheme, fns ...addToSchemeFunc) error {
	for _, fn := range fns {
		if err := fn(s); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
