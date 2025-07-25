/*
Copyright 2023 The KServe Authors.

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

package testing

import (
	"path/filepath"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"

	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnvTest prepares k8s EnvTest with prereq
func NewEnvTest(options ...Option) *Config {
	testCRDs := WithCRDs(
		filepath.Join(ProjectRoot(), "test", "crds"),
	)
	schemes := WithScheme(
		// KServe Schemes
		v1alpha1.AddToScheme,
		v1beta1.AddToScheme,
		// Kubernetes Schemes
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		appsv1.AddToScheme,
		apiextv1.AddToScheme,
		netv1.AddToScheme,
		gatewayapiv1.Install,
		igwapi.Install,
		// Other Schemes
		knservingv1.AddToScheme,
		istioclientv1beta1.AddToScheme,
		kedav1alpha1.AddToScheme,
		otelv1beta1.AddToScheme,
	)

	return Configure(append(options, testCRDs, schemes)...)
}
