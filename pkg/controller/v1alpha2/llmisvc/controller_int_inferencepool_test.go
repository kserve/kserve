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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("InferencePool v1", func() {
	It("creates a multi-port v1 pool and routes to it before Gateway acceptance", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		llmSvc := LLMInferenceService("test-llm-v1-pool",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
		)
		ports := []igwapi.Port{{Number: 8000}, {Number: 8001}, {Number: 8002}, {Number: 8003}, {Number: 8004}, {Number: 8005}, {Number: 8006}, {Number: 8007}}
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
		}).WithContext(ctx).Should(Succeed())
	})
})
