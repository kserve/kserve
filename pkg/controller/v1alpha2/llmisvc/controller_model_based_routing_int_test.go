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

package llmisvc_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

func countModelRoutingRules(rules []gwapiv1.HTTPRouteRule) int {
	count := 0
	for _, rule := range rules {
		for _, match := range rule.Matches {
			for _, h := range match.Headers {
				if string(h.Name) == "X-Gateway-Model-Name" {
					count++
					break
				}
			}
		}
	}
	return count
}

func patchIngressModelBasedRoutingMode(ctx context.Context, mode string) {
	isvcConfigMap := &corev1.ConfigMap{}
	Expect(envTest.Client.Get(ctx, types.NamespacedName{
		Name:      constants.InferenceServiceConfigMapName,
		Namespace: constants.KServeNamespace,
	}, isvcConfigMap)).To(Succeed())

	var ingressConfig map[string]interface{}
	Expect(json.Unmarshal([]byte(isvcConfigMap.Data["ingress"]), &ingressConfig)).To(Succeed())
	ingressConfig["modelBasedRoutingMode"] = mode
	updatedIngress, err := json.Marshal(ingressConfig)
	Expect(err).NotTo(HaveOccurred())

	patch := client.MergeFrom(isvcConfigMap.DeepCopy())
	isvcConfigMap.Data["ingress"] = string(updatedIngress)
	Expect(envTest.Client.Patch(ctx, isvcConfigMap, patch)).To(Succeed())
}

func restoreIngressModelBasedRoutingMode(ctx context.Context) {
	patchIngressModelBasedRoutingMode(ctx, "enabled")
}

func createModelRoutingTestService(ctx context.Context, svcName string, testNs *TestNamespace) *v1alpha2.LLMInferenceService {
	modelConfig := LLMInferenceServiceConfig("model-fb-opt-125m",
		InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
		WithConfigModelName("facebook/opt-125m"),
		WithConfigModelURI("hf://facebook/opt-125m"),
	)

	routerConfig := LLMInferenceServiceConfig("router-managed",
		InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
		WithConfigManagedRouter(),
	)

	workloadConfig := LLMInferenceServiceConfig("workload-single-cpu",
		InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
		WithConfigWorkloadTemplate(&corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "quay.io/pierdipi/vllm-cpu:latest",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("10Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				},
			},
		}),
	)

	Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())
	Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())
	Expect(envTest.Client.Create(ctx, workloadConfig)).To(Succeed())

	llmSvc := LLMInferenceService(svcName,
		InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
		WithBaseRefs(
			corev1.LocalObjectReference{Name: "model-fb-opt-125m"},
			corev1.LocalObjectReference{Name: "router-managed"},
			corev1.LocalObjectReference{Name: "workload-single-cpu"},
		),
	)

	Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
	return llmSvc
}

var _ = Describe("Model Based Routing", func() {
	const headerName = "X-Gateway-Model-Name"

	Context("Default mode (enabled)", func() {
		It("should include both path-based and model-routing rules", func(ctx SpecContext) {
			svcName := "test-mbr-default"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := createModelRoutingTestService(ctx, svcName, testNs)
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, "publishers/"+testNs.Name+"/models/facebook/opt-125m"))

				rules := routes[0].Spec.Rules
				g.Expect(countModelRoutingRules(rules)).To(BeNumerically(">=", 4),
					"should have at least 4 model-routing rules")
			}).WithContext(ctx).Should(Succeed())

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Disabled via ConfigMap", func() {
		It("should strip model-routing rules when mode is disabled", func(ctx SpecContext) {
			patchIngressModelBasedRoutingMode(ctx, "disabled")
			DeferCleanup(restoreIngressModelBasedRoutingMode)

			svcName := "test-mbr-disabled"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := createModelRoutingTestService(ctx, svcName, testNs)
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				g.Expect(&routes[0]).NotTo(HaveHeaderMatch(headerName, "publishers/"+testNs.Name+"/models/facebook/opt-125m"))

				rules := routes[0].Spec.Rules
				g.Expect(countModelRoutingRules(rules)).To(Equal(0),
					"should have no model-routing rules when disabled")
			}).WithContext(ctx).Should(Succeed())

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Forced via ConfigMap", func() {
		It("should keep model-routing rules even with Gateway opt-out annotation", func(ctx SpecContext) {
			patchIngressModelBasedRoutingMode(ctx, "forced")
			DeferCleanup(restoreIngressModelBasedRoutingMode)

			svcName := "test-mbr-forced"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			customGatewayName := "mbr-forced-gw"
			customGateway := Gateway(customGatewayName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.42"),
			)
			customGateway.Annotations = map[string]string{
				llmisvc.AnnotationModelBasedRoutingEnabled: "false",
			}
			Expect(envTest.Client.Create(ctx, customGateway)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, customGateway)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithGatewayRefs(LLMGatewayRef(customGatewayName, testNs.Name)),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, "publishers/"+testNs.Name+"/models/facebook/opt-125m"))

				rules := routes[0].Spec.Rules
				g.Expect(countModelRoutingRules(rules)).To(BeNumerically(">=", 4),
					"forced mode should keep model-routing rules despite Gateway opt-out")
			}).WithContext(ctx).Should(Succeed())

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Enabled with Gateway opt-out annotation", func() {
		It("should strip model-routing rules when Gateway opts out", func(ctx SpecContext) {
			svcName := "test-mbr-gw-optout"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			customGatewayName := "mbr-optout-gw"
			customGateway := Gateway(customGatewayName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.42"),
			)
			customGateway.Annotations = map[string]string{
				llmisvc.AnnotationModelBasedRoutingEnabled: "false",
			}
			Expect(envTest.Client.Create(ctx, customGateway)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, customGateway)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithGatewayRefs(LLMGatewayRef(customGatewayName, testNs.Name)),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				g.Expect(&routes[0]).NotTo(HaveHeaderMatch(headerName, "publishers/"+testNs.Name+"/models/facebook/opt-125m"))

				rules := routes[0].Spec.Rules
				g.Expect(countModelRoutingRules(rules)).To(Equal(0),
					"should have no model-routing rules when Gateway opts out")
			}).WithContext(ctx).Should(Succeed())

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
		})
	})
})
