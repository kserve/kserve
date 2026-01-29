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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("InferencePool Migration", func() {
	Context("Dual-pool creation (both v1 and v1alpha2 CRDs available)", func() {
		It("should create both v1 and v1alpha2 InferencePools when both CRDs are available", func(ctx SpecContext) {
			// given
			svcName := "test-llm-dual-pool"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify v1 InferencePool is created
			expectedPoolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) error {
				v1Pool := &igwapi.InferencePool{}
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)
			}).WithContext(ctx).Should(Succeed(), "v1 InferencePool should be created")

			// then - verify v1alpha2 InferencePool is created (same name, different API group)
			Eventually(func(g Gomega, ctx context.Context) error {
				v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
				return envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2Pool)
			}).WithContext(ctx).Should(Succeed(), "v1alpha2 InferencePool should be created")

			// Verify both pools have correct owner reference
			v1Pool := &igwapi.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1Pool)).To(Succeed())
			Expect(v1Pool).To(BeOwnedBy(llmSvc))

			v1alpha2Pool := &igwapiv1alpha2.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: expectedPoolName, Namespace: nsName}, v1alpha2Pool)).To(Succeed())
			Expect(v1alpha2Pool).To(BeOwnedBy(llmSvc))

			// Note: In envtest, the v1 pool is considered ready immediately (no status conditions = valid pool = ready),
			// so migration may happen before we can check. The key assertion is that both pools are created.
			// The migration annotation and HTTPRoute swap behavior are tested in separate test cases.
		})

		It("should point HTTPRoute backendRef to v1alpha2 pool initially (before migration)", func(ctx SpecContext) {
			// given
			svcName := "test-llm-initial-backend"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify HTTPRoute points to v1alpha2 pool (API group)
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).ToNot(BeEmpty())

				// Check that the backendRef points to v1alpha2 API group
				backendRef := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(backendRef.Group).ToNot(BeNil())
				g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1Alpha2APIGroupName),
					"HTTPRoute backendRef should initially point to v1alpha2 API group")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Pre-migrated deployments", func() {
		It("should point HTTPRoute directly to v1 pool when migration annotation is already set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-pre-migrated"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			// Create LLMInferenceService with migration annotation already set
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithAnnotations(map[string]string{
					constants.InferencePoolMigratedAnnotationKey: "v1",
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - HTTPRoute should point directly to v1 pool
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).ToNot(BeEmpty())

				// Check that the backendRef points to v1 API group (already migrated)
				backendRef := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(backendRef.Group).ToNot(BeNil())
				g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName),
					"HTTPRoute backendRef should point to v1 API group for pre-migrated deployment")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Migration lock (prevents flapping)", func() {
		It("should not change migration annotation once set to v1", func(ctx SpecContext) {
			// given
			svcName := "test-llm-migration-lock"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			// Create with migration annotation already set (simulating existing migrated deployment)
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithAnnotations(map[string]string{
					constants.InferencePoolMigratedAnnotationKey: "v1",
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Make managed resources ready
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Verify migration annotation remains "v1" (not changed)
			Consistently(func(g Gomega, ctx context.Context) error {
				updatedLLMSvc := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())
				g.Expect(updatedLLMSvc.Annotations).To(HaveKeyWithValue(
					constants.InferencePoolMigratedAnnotationKey, "v1"),
					"Migration annotation should remain 'v1' (one-way lock)")
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

// Helper to list managed InferencePools
func managedInferencePools(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*igwapi.InferencePoolList, error) {
	pools := &igwapi.InferencePoolList{}
	listOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
	}
	err := envTest.List(ctx, pools, listOpts)
	return pools, err
}

// Helper to list managed v1alpha2 InferencePools
func managedInferencePoolsV1Alpha2(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*igwapiv1alpha2.InferencePoolList, error) {
	pools := &igwapiv1alpha2.InferencePoolList{}
	listOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
	}
	err := envTest.List(ctx, pools, listOpts)
	return pools, err
}
