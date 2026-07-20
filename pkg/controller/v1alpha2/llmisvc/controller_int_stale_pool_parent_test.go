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

package llmisvc_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("Stale InferencePool parent handling", func() {
		It("should recover readiness after switching from a custom gateway to the default", func(ctx SpecContext) {
			svcName := "test-stale-pool-parent"
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

			poolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Client.Get(ctx, client.ObjectKey{
					Name: poolName, Namespace: testNs.Name,
				}, &igwapi.InferencePool{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
			})).WithContext(ctx).Should(Succeed())

			// Simulate what happens when a user previously referenced a different gateway
			// (e.g. data-science-gateway) and then switched to the default. The old gateway
			// controller leaves a rejected parent entry in the pool status.
			injectStalePoolParent(ctx, envTest.Client,
				client.ObjectKey{Name: poolName, Namespace: testNs.Name},
				igwapi.ParentReference{
					Group:     ptr.To(igwapi.Group("networking.istio.io")),
					Kind:      igwapi.Kind("Gateway"),
					Name:      "data-science-gateway",
					Namespace: "openshift-ingress",
				},
			)

			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"),
					"stale rejected parent from old gateway should not block readiness")
			}).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(time.Second).Should(Succeed())
		})
	})
})

// injectStalePoolParent appends a rejected parent entry to an InferencePool's status,
// simulating a gateway controller that hasn't cleaned up after the HTTPRoute stopped
// referencing its gateway. Only runs in envtest (no-op against a real cluster where
// the gateway controller manages pool status).
func injectStalePoolParent(ctx context.Context, c client.Client, poolKey client.ObjectKey, parentRef igwapi.ParentReference) {
	if envTest.UsingExistingCluster() {
		return
	}

	pool := &igwapi.InferencePool{}
	Expect(c.Get(ctx, poolKey, pool)).To(Succeed())

	pool.Status.Parents = append(pool.Status.Parents, igwapi.ParentStatus{
		ParentRef: parentRef,
		Conditions: []metav1.Condition{{
			Type:               string(igwapi.InferencePoolConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             "HTTPRouteNotAccepted",
			Message:            "namespace not allowed by the parent",
			LastTransitionTime: metav1.Now(),
		}},
	})

	Expect(c.Status().Update(ctx, pool)).To(Succeed())
}
