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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
	"github.com/kserve/kserve/pkg/utils"
)

var _ = Describe("InferencePool v1", func() {
	DescribeTable("selects standalone rank configs through baseRefs with asymmetric active ports", func(ctx SpecContext, useDisaggregatedSet bool) {
		PatchLLMISVCFeatureGates(ctx, envTest.Client, map[string]bool{"disaggregatedSet": useDisaggregatedSet})
		DeferCleanup(func(ctx context.Context) {
			PatchLLMISVCFeatureGates(ctx, envTest.Client, nil)
		})
		testNs := NewTestNamespace(ctx, envTest)
		llmSvc := LLMInferenceService("test-rank-configs",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
			WithBaseRefs(
				corev1.LocalObjectReference{Name: "kserve-config-llm-prefill-worker-data-parallel-multi-port"},
				corev1.LocalObjectReference{Name: "kserve-config-llm-decode-worker-data-parallel-multi-port"},
			),
		)
		llmSvc.Spec.Parallelism = &v1alpha2.ParallelismSpec{Data: ptr.To[int32](8), DataLocal: ptr.To[int32](4), Tensor: ptr.To[int32](1), Expert: true}
		llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2), DataLocal: ptr.To[int32](1), Tensor: ptr.To[int32](8), Expert: true}}
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(func(g Gomega, ctx context.Context) {
			pools, err := managedInferencePools(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pools.Items).To(HaveLen(1))
			g.Expect(pools.Items[0].Spec.TargetPorts).To(Equal([]igwapi.Port{{Number: 8000}, {Number: 8001}, {Number: 8002}, {Number: 8003}}))
			groups := &lwsapi.LeaderWorkerSetList{}
			g.Expect(envTest.List(ctx, groups, client.InNamespace(testNs.Name), client.MatchingLabels{constants.KubernetesAppNameLabelKey: llmSvc.Name})).To(Succeed())
			sets := &disaggregatedsetv1.DisaggregatedSetList{}
			g.Expect(envTest.List(ctx, sets, client.InNamespace(testNs.Name))).To(Succeed())
			var templates []lwsapi.LeaderWorkerTemplate
			if useDisaggregatedSet {
				g.Expect(groups.Items).To(BeEmpty(), "envtest does not run the DisaggregatedSet controller that creates child LWSs")
				g.Expect(sets.Items).To(HaveLen(1))
				g.Expect(&sets.Items[0]).To(BeOwnedBy(llmSvc))
				g.Expect(sets.Items[0].Spec.Roles).To(HaveLen(2))
				for _, role := range sets.Items[0].Spec.Roles {
					templates = append(templates, role.Spec.LeaderWorkerTemplate)
				}
			} else {
				g.Expect(sets.Items).To(BeEmpty())
				g.Expect(groups.Items).To(HaveLen(2))
				for _, group := range groups.Items {
					templates = append(templates, group.Spec.LeaderWorkerTemplate)
				}
			}
			for _, template := range templates {
				g.Expect(template.Size).To(Equal(ptr.To[int32](2)))
				g.Expect(template.LeaderTemplate).NotTo(BeNil())
				for _, pod := range []*corev1.PodTemplateSpec{template.LeaderTemplate, &template.WorkerTemplate} {
					g.Expect(pod.Labels).To(HaveKeyWithValue(constants.KServeComponentLabelKey, constants.KServeComponentWorkload))
					main := utils.GetContainerWithName(&pod.Spec, "main")
					g.Expect(main).NotTo(BeNil())
					probePort := "dp-supervisor"
					readinessPath := "/readyz"
					if pod.Labels[constants.LLMDRoleLabelKey] == constants.LLMDRolePrefill {
						g.Expect(pod.Annotations).To(HaveKeyWithValue("llm-d.ai/active-ports", "8000"))
						g.Expect(pod.Spec.InitContainers).NotTo(ContainElement(HaveField("Name", constants.LLMISVCRoutingSidecarContainerName)))
						probePort = "dp-rank-0"
						readinessPath = "/health"
					} else {
						g.Expect(pod.Labels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, constants.LLMDRoleDecode))
						g.Expect(pod.Annotations).To(HaveKeyWithValue("llm-d.ai/active-ports", "8000,8001,8002,8003"))
						g.Expect(pod.Spec.InitContainers).To(ContainElement(HaveField("Name", constants.LLMISVCRoutingSidecarContainerName)))
						for _, sidecar := range pod.Spec.InitContainers {
							if sidecar.Name != constants.LLMISVCRoutingSidecarContainerName {
								continue
							}
							g.Expect(sidecar.Command).To(ContainElements("--data-parallel-size=4", "--model-server-port=8200"))
							g.Expect(sidecar.LivenessProbe.HTTPGet.Port.StrVal).To(Equal("routing-sidecar"))
							g.Expect(sidecar.ReadinessProbe.HTTPGet.Port.StrVal).To(Equal("routing-sidecar"))
						}
					}
					for _, probe := range []*corev1.Probe{main.StartupProbe, main.LivenessProbe, main.ReadinessProbe} {
						g.Expect(probe).NotTo(BeNil())
						g.Expect(probe.HTTPGet).NotTo(BeNil())
						g.Expect(probe.HTTPGet.Port.StrVal).To(Equal(probePort))
					}
					g.Expect(main.ReadinessProbe.HTTPGet.Path).To(Equal(readinessPath))
					g.Expect(main.Ports).To(ContainElement(HaveField("Name", probePort)))
				}
			}
		}).WithContext(ctx).Should(Succeed())
	},
		Entry("on LeaderWorkerSets", false),
		Entry("on a DisaggregatedSet", true),
	)

	DescribeTable("creates a multi-port v1 pool without restricting unrelated workload ports", func(ctx SpecContext, ports []igwapi.Port) {
		testNs := NewTestNamespace(ctx, envTest)
		llmSvc := LLMInferenceService("test-llm-v1-pool",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
		)
		llmSvc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{
			Selector: igwapi.LabelSelector{MatchLabels: map[igwapi.LabelKey]igwapi.LabelValue{
				"app.kubernetes.io/name": igwapi.LabelValue(llmSvc.Name),
			}},
			TargetPorts: ports,
		}}
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(func(g Gomega, ctx context.Context) {
			pools, err := managedInferencePools(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pools.Items).To(HaveLen(1))
			g.Expect(&pools.Items[0]).To(BeOwnedBy(llmSvc))
			g.Expect(pools.Items[0].Spec.TargetPorts).To(Equal(ports))
			legacyPool := &igwapiv1alpha2.InferencePool{}
			g.Expect(apierrors.IsNotFound(envTest.Get(ctx, client.ObjectKeyFromObject(&pools.Items[0]), legacyPool))).To(BeTrue(), "multi-port pools must not create a truncated legacy copy")
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(&routes[0]).To(HaveBackendRefs(BackendRefInferencePool(llmSvc.Name + "-inference-pool")))
			workload := &appsv1.Deployment{}
			g.Expect(envTest.Get(ctx, client.ObjectKey{Namespace: testNs.Name, Name: llmSvc.Name + "-kserve"}, workload)).To(Succeed())
			g.Expect(workload.Spec.Template.Annotations).NotTo(HaveKey("llm-d.ai/active-ports"))
		}).WithContext(ctx).Should(Succeed())
	},
		Entry("eight standard ports", []igwapi.Port{{Number: 8000}, {Number: 8001}, {Number: 8002}, {Number: 8003}, {Number: 8004}, {Number: 8005}, {Number: 8006}, {Number: 8007}}),
		Entry("two standard ports", []igwapi.Port{{Number: 8000}, {Number: 8001}}),
		Entry("custom ports", []igwapi.Port{{Number: 9000}, {Number: 9001}}),
	)
})
