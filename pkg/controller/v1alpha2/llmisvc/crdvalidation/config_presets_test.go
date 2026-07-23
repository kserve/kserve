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

package crdvalidation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// The LLMInferenceServiceConfig CRD embeds Gateway API, GIE, and core
// Kubernetes types whose controller-gen schemas carry pattern, minLength,
// maxLength, and x-kubernetes-validations constraints. These constraints
// reject the Go template expressions used in the config presets (e.g.
// {{ .GlobalConfig.ModelBasedRoutingHeaderName }}). The Makefile strips
// them after controller-gen runs; these tests catch drift when the
// stripping goes stale after dependency bumps.
var _ = Describe("LLMInferenceServiceConfig CRD", func() {
	var ns string

	BeforeEach(func(ctx SpecContext) {
		ns = constants.KServeNamespace
		Expect(envTest.Client.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Or(Succeed(), MatchError(ContainSubstring("already exists"))))
	})

	It("should pass CRD schema validation for all shipped presets", func(ctx SpecContext) {
		presets := fixture.SharedConfigPresets(ns)
		Expect(presets).NotTo(BeEmpty(), "no config presets found in config/llmisvcconfig/")

		for _, preset := range presets {
			Expect(envTest.Client.Create(ctx, preset)).To(Succeed(),
				"CRD schema rejected preset %q - run 'make manifests' to strip embedded type validation", preset.Name)
		}
	})

	It("should accept Go templates in HTTPRoute filter header names", func(ctx SpecContext) {
		probe := &v1alpha2.LLMInferenceServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "crd-validation-probe",
				Namespace: ns,
			},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Model: v1alpha2.LLMModelSpec{URI: mustParseURL("gs://dummy")},
				Router: &v1alpha2.RouterSpec{
					Route: &v1alpha2.GatewayRoutesSpec{
						HTTP: &v1alpha2.HTTPRouteSpec{
							Spec: &gwapiv1.HTTPRouteSpec{
								Rules: []gwapiv1.HTTPRouteRule{{
									Matches: []gwapiv1.HTTPRouteMatch{{
										Path: &gwapiv1.HTTPPathMatch{
											Type:  ptrTo(gwapiv1.PathMatchPathPrefix),
											Value: ptrTo("/"),
										},
									}},
									Filters: []gwapiv1.HTTPRouteFilter{{
										Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
										RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
											Set: []gwapiv1.HTTPHeader{{
												Name:  "{{ .GlobalConfig.ModelBasedRoutingHeaderName }}",
												Value: "{{ .ObjectMeta.Name }}",
											}},
										},
									}},
								}},
							},
						},
					},
				},
			},
		}

		Expect(envTest.Client.Create(ctx, probe)).To(Succeed(),
			"CRD schema rejected Go template in requestHeaderModifier - embedded type validation not stripped")
	})
})

func mustParseURL(s string) apis.URL {
	u, err := apis.ParseURL(s)
	if err != nil {
		panic(err)
	}
	return *u
}

func ptrTo[T any](v T) *T {
	return &v
}
