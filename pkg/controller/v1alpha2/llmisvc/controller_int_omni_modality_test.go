/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package llmisvc_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// Reconciler-level coverage for the vLLM-Omni modality path: an
// LLMInferenceService with spec.runtime kserve-llm-omni must render the base
// vLLM workload template with the omni overlay layered on top (engine image
// and startup probe swapped in, --omni appended to the args), the load-only
// EPP profile, and HTTPRoute rules carrying the audio/image endpoints to the
// InferencePool. Non-omni services must have the modality rules stripped so
// they keep the Gateway API rule and match headroom LoRA expansion needs.
func createOmniTestService(ctx SpecContext, svcName string, testNs *TestNamespace) *v1alpha2.LLMInferenceService {
	// Scheduler template at llm-d-router >= 1.0.0 so the controller injects
	// the EPPConfig preset instead of the legacy hardcoded config text.
	schedulerCfg := LLMInferenceServiceConfig("kserve-config-llm-scheduler",
		InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
		WithConfigSchedulerTemplate("1.0.0"),
	)
	Expect(envTest.Client.Create(ctx, schedulerCfg)).To(Succeed())

	llmSvc := LLMInferenceService(svcName,
		InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
		WithModelURI("hf://facebook/opt-125m"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
	)
	llmSvc.Spec.Runtime = ptr.To(llmisvc.OmniServingRuntimeName)

	// The 1.0.0 scheduler-template override above shadows the shared
	// kserve-config-llm-scheduler preset, which normally contributes the pool
	// spec. Carry an equivalent pool spec on the service (highest precedence)
	// so the managed InferencePool validates; pool content is incidental to
	// what this suite asserts (template, EPP profile, routes).
	eppPort := igwapi.PortNumber(9002)
	modelPort := igwapi.PortNumber(8000)
	llmSvc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
		Spec: &igwapi.InferencePoolSpec{
			Selector: igwapi.LabelSelector{MatchLabels: map[igwapi.LabelKey]igwapi.LabelValue{
				"app.kubernetes.io/name":    igwapi.LabelValue(svcName),
				"app.kubernetes.io/part-of": "llminferenceservice",
				"kserve.io/component":       "workload",
			}},
			TargetPorts: []igwapi.Port{{Number: modelPort}},
			EndpointPickerRef: &igwapi.EndpointPickerRef{
				Kind:        "Service",
				Name:        igwapi.ObjectName(kmeta.ChildName(svcName, "-epp-service")),
				Port:        &igwapi.Port{Number: eppPort},
				FailureMode: igwapi.EndpointPickerFailOpen,
			},
		},
	}

	// The omni overlay pins the engine image, so in production the
	// kserve-llm-omni ClusterServingRuntime is not needed for the workload to
	// render. Mirror it anyway with a sentinel image to pin down the layering:
	// the runtime is the lowest-priority merge layer, so the overlay's image
	// must win over both the runtime's and the base template's.
	runtime := &v1alpha1.ServingRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmisvc.OmniServingRuntimeName,
			Namespace: testNs.Name,
		},
		Spec: v1alpha1.ServingRuntimeSpec{
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{{Name: "vllm-omni"}},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{{
					Name:  "main",
					Image: "example.com/sentinel-omni-runtime-image:v0",
				}},
			},
		},
	}
	Expect(envTest.Client.Create(ctx, runtime)).To(Succeed())

	Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
	return llmSvc
}

var _ = Describe("Omni Modality (TTS/TTI)", func() {
	Context("omni runtime scheduler", func() {
		It("should use the load-only modality EPP profile, not the text prefix chain", func(ctx SpecContext) {
			// given
			svcName := "test-llm-omni-scheduler"
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := createOmniTestService(ctx, svcName, testNs)
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - scheduler deployment carries the modality preset
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, expectedDeployment)).To(Succeed())

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected preset config in scheduler deployment")
				g.Expect(configText).To(MatchRegexp(`(?m)^apiVersion: llm-d\.ai/v1$`))

				pluginTypes, err := pluginTypesFromConfig(configText)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pluginTypes).To(ContainElements(
					"active-request-scorer",
					"max-score-picker",
				))
				g.Expect(pluginTypes).NotTo(ContainElement("prefix-cache-affinity-filter"),
					"text prefix chain has no TokenizedRequest for audio/image inputs")
				g.Expect(schedulerProfileNames(configText)).To(ConsistOf("default"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("omni runtime workload", func() {
		It("should layer the omni overlay on the base vLLM template", func(ctx SpecContext) {
			// given
			svcName := "test-llm-omni-workload"
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := createOmniTestService(ctx, svcName, testNs)
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - the workload reuses the base template command (with the
			// model-based routing served name) and swaps only the overlay's
			// diff: image, startup probe, and the --omni arg.
			workload := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve"),
					Namespace: testNs.Name,
				}, workload)).To(Succeed())

				g.Expect(workload.Spec.Template.Spec.Containers).NotTo(BeEmpty())
				main := workload.Spec.Template.Spec.Containers[0]

				var joined strings.Builder
				for _, c := range main.Command {
					joined.WriteString(c + "\n")
				}
				command := joined.String()
				g.Expect(command).To(ContainSubstring("vllm serve"),
					"command must come from the base vLLM template")
				g.Expect(command).To(ContainSubstring("--served-model-name"),
					"served model names must come from the base vLLM template")
				g.Expect(command).To(ContainSubstring(
					fmt.Sprintf("publishers/%s/models/%s", testNs.Name, svcName)),
					"the model-based routing model name must be served for header-based routing")
				g.Expect(command).NotTo(ContainSubstring("--omni"),
					"--omni is passed as a container arg, not a command fork")

				g.Expect(main.Args).To(ContainElement("--omni"))
				g.Expect(main.Image).To(Equal("docker.io/vllm/vllm-omni:v0.30.0"),
					"engine image must come from the omni overlay, above the runtime and base template layers")
				g.Expect(main.StartupProbe).NotTo(BeNil())
				g.Expect(main.StartupProbe.FailureThreshold).To(Equal(int32(120)),
					"startup probe must come from the omni overlay")
				g.Expect(main.SecurityContext.RunAsNonRoot).To(HaveValue(BeFalse()),
					"the vllm-omni image ships no non-root user")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("omni runtime routes", func() {
		It("should expose modality endpoints to the InferencePool", func(ctx SpecContext) {
			// given
			svcName := "test-llm-omni-routes"
			testNs := NewTestNamespace(ctx, envTest)
			llmSvc := createOmniTestService(ctx, svcName, testNs)
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - path rules rewrite modality prefixes to the pool
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).ToNot(BeEmpty())

				var got []string
				var sawAudioPrefix, sawImagesPrefix, sawSpeechExact, sawAudioPublisher bool
				for _, r := range routes {
					for _, rule := range r.Spec.Rules {
						isPool := false
						for _, b := range rule.BackendRefs {
							if b.Kind != nil && *b.Kind == "InferencePool" {
								isPool = true
							}
						}
						if !isPool {
							continue
						}
						for _, m := range rule.Matches {
							if m.Path == nil || m.Path.Type == nil {
								continue
							}
							typ := *m.Path.Type
							v := ptr.Deref(m.Path.Value, "")
							got = append(got, fmt.Sprintf("%s %s", typ, v))
							if typ == gwapiv1.PathMatchPathPrefix && strings.Contains(v, "v1/audio") {
								sawAudioPrefix = true
							}
							if typ == gwapiv1.PathMatchPathPrefix && strings.Contains(v, "v1/images") {
								sawImagesPrefix = true
							}
							if typ == gwapiv1.PathMatchExact && v == "/v1/audio/speech" && len(m.Headers) > 0 {
								sawSpeechExact = true
							}
							if typ == gwapiv1.PathMatchPathPrefix && strings.Contains(v, "/publishers/") && strings.Contains(v, "v1/audio") {
								sawAudioPublisher = true
							}
						}
					}
				}
				g.Expect(sawAudioPrefix).To(BeTrue(), "expected PathPrefix audio rule to InferencePool, got %v", got)
				g.Expect(sawImagesPrefix).To(BeTrue(), "expected PathPrefix images rule to InferencePool, got %v", got)
				g.Expect(sawSpeechExact).To(BeTrue(), "expected Exact /v1/audio/speech model-routing match, got %v", got)
				g.Expect(sawAudioPublisher).To(BeTrue(), "expected publisher-qualified audio rule to InferencePool, got %v", got)
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("non-omni runtime routes", func() {
		It("should strip the modality rules the runtime cannot serve", func(ctx SpecContext) {
			// given - a plain vLLM service: same route preset, no omni runtime
			svcName := "test-llm-text-routes"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - no modality rule or match survives, so the text endpoints
			// keep the Gateway API rule and match budget LoRA expansion needs
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).ToNot(BeEmpty())

				for _, r := range routes {
					for _, rule := range r.Spec.Rules {
						for _, m := range rule.Matches {
							if m.Path == nil {
								continue
							}
							v := ptr.Deref(m.Path.Value, "")
							g.Expect(v).NotTo(SatisfyAny(
								ContainSubstring("v1/audio"),
								ContainSubstring("v1/images"),
							), "modality path %q must be stripped for non-omni runtimes", v)
						}
					}

					// The preset's 16 rules drop to 12: the four modality
					// prefix rules (namespaced + publisher) are removed and
					// the model-routing rule keeps only its text matches.
					ruleNames := make([]string, 0, len(r.Spec.Rules))
					var modelRoutingMatches int
					for _, rule := range r.Spec.Rules {
						ruleNames = append(ruleNames, string(ptr.Deref(rule.Name, "")))
						if ptr.Deref(rule.Name, "") == "v1-model-routing" {
							modelRoutingMatches = len(rule.Matches)
						}
					}
					g.Expect(ruleNames).To(ConsistOf(
						"v1-completions-path",
						"v1-chat-completions-path",
						"v1-responses-path",
						"v1-messages-path",
						"v1-model-routing",
						"v1-completions-publisher-path",
						"v1-chat-completions-publisher-path",
						"v1-responses-publisher-path",
						"v1-messages-publisher-path",
						"v1-publisher-path-catch-all",
						"v1-catch-all-path",
						"v1-catch-all-model-routing",
					))
					g.Expect(modelRoutingMatches).To(Equal(8),
						"text model-routing matches: completions, chat/completions, responses, messages, with and without trailing slash")
				}
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
