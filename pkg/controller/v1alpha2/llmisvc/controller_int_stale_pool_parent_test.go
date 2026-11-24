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
	"k8s.io/client-go/util/retry"
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

			// A real gateway controller owns the pool status on an existing cluster.
			if envTest.UsingExistingCluster() {
				return
			}

			pool := &igwapi.InferencePool{}
			Expect(envTest.Get(ctx, client.ObjectKey{Name: poolName, Namespace: testNs.Name}, pool)).To(Succeed())
			Expect(pool.Status.Parents).To(HaveLen(1))
			defaultGateway := pool.Status.Parents[0].ParentRef
			// The gateway the service used before switching to the default. Its controller
			// leaves a rejected parent entry in the pool status.
			oldGateway := igwapi.ParentReference{
				Group:     ptr.To(igwapi.Group("networking.istio.io")),
				Kind:      igwapi.Kind("Gateway"),
				Name:      "data-science-gateway",
				Namespace: "openshift-ingress",
			}

			// when - the default gateway has not accepted the pool yet
			setPoolParents(ctx, pool, poolParent(defaultGateway, metav1.ConditionFalse), poolParent(oldGateway, metav1.ConditionFalse))
			Eventually(llmSvcStatus).WithContext(ctx).WithArguments(llmSvc).
				Should(HaveCondition(string(v1alpha2.InferencePoolReady), "False"))

			// and - then accepts it, while the old gateway's rejection stays
			setPoolParents(ctx, pool, poolParent(defaultGateway, metav1.ConditionTrue), poolParent(oldGateway, metav1.ConditionFalse))

			// then - True after False takes a pass that read the stale parent
			poolReady := HaveCondition(string(v1alpha2.InferencePoolReady), "True")
			Eventually(llmSvcStatus).WithContext(ctx).WithArguments(llmSvc).
				Should(poolReady, "stale rejected parent from old gateway should not block readiness")

			// and - so does a pass that starts from True. The hold is there because a block on
			// that pass alone flips the condition back and forth, and one read can land on True.
			forceObservedPass(ctx, llmSvc)
			Consistently(llmSvcStatus).WithContext(ctx).WithArguments(llmSvc).
				WithTimeout(500 * time.Millisecond).Should(poolReady)
		})
	})
})

// setPoolParents replaces an InferencePool's status parents, standing in for the
// gateway controllers that envtest does not run.
func setPoolParents(ctx context.Context, pool *igwapi.InferencePool, parents ...igwapi.ParentStatus) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := envTest.Get(ctx, client.ObjectKeyFromObject(pool), pool); err != nil {
			return err
		}
		pool.Status.Parents = parents
		return envTest.Status().Update(ctx, pool)
	})).To(Succeed())
}

// poolParent is a gateway's verdict on the pool.
func poolParent(gateway igwapi.ParentReference, accepted metav1.ConditionStatus) igwapi.ParentStatus {
	reason := string(igwapi.InferencePoolReasonAccepted)
	if accepted != metav1.ConditionTrue {
		reason = "HTTPRouteNotAccepted"
	}
	return igwapi.ParentStatus{
		ParentRef: gateway,
		Conditions: []metav1.Condition{{
			Type:               string(igwapi.InferencePoolConditionAccepted),
			Status:             accepted,
			Reason:             reason,
			LastTransitionTime: metav1.Now(),
		}},
	}
}
