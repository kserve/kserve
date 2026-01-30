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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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

	Context("Pre-migrated deployments (HTTPRoute annotation)", func() {
		It("should keep HTTPRoute pointing to v1 pool when migration annotation is already on HTTPRoute", func(ctx SpecContext) {
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

			// Create LLMInferenceService (no annotation - migration state is on HTTPRoute)
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

			// Wait for HTTPRoute to be created with v1alpha2 backendRef initially
			var managedRoute *gwapiv1.HTTPRoute
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				managedRoute = &routes[0]
				return nil
			}).WithContext(ctx).Should(Succeed())

			// Simulate existing migration annotation on HTTPRoute (as if Gateway had rejected before)
			updatedRoute := managedRoute.DeepCopy()
			if updatedRoute.Annotations == nil {
				updatedRoute.Annotations = make(map[string]string)
			}
			updatedRoute.Annotations[llmisvc.AnnotationInferencePoolMigrated] = "v1"
			Expect(envTest.Client.Update(ctx, updatedRoute)).To(Succeed())

			// Trigger reconciliation by updating the LLMInferenceService
			llmSvcUpdated := &v1alpha2.LLMInferenceService{}
			Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), llmSvcUpdated)).To(Succeed())
			if llmSvcUpdated.Annotations == nil {
				llmSvcUpdated.Annotations = make(map[string]string)
			}
			llmSvcUpdated.Annotations["trigger-reconcile"] = "true"
			Expect(envTest.Client.Update(ctx, llmSvcUpdated)).To(Succeed())

			// then - HTTPRoute should point to v1 pool (respecting annotation)
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).ToNot(BeEmpty())

				// Check that the backendRef points to v1 API group (respecting annotation)
				backendRef := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(backendRef.Group).ToNot(BeNil())
				g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName),
					"HTTPRoute backendRef should point to v1 API group when migration annotation is set")

				// Also verify annotation is still present
				g.Expect(route.Annotations).To(HaveKeyWithValue(
					llmisvc.AnnotationInferencePoolMigrated, "v1"),
					"HTTPRoute migration annotation should remain 'v1'")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Gateway rejection triggers migration to v1", func() {
		It("should swap HTTPRoute backendRef from v1alpha2 to v1 when Gateway rejects v1alpha2", func(ctx SpecContext) {
			// given
			svcName := "test-llm-gateway-rejection"
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

			// when - create the LLMInferenceService
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for HTTPRoute to be created with v1alpha2 backendRef initially
			var managedRoute *gwapiv1.HTTPRoute
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				managedRoute = &routes[0]
				return nil
			}).WithContext(ctx).Should(Succeed())

			// Simulate Gateway rejection: set ResolvedRefs=False, Reason=InvalidKind
			// This is what Envoy Gateway does when it doesn't support v1alpha2 backendRef
			updatedRoute := managedRoute.DeepCopy()
			WithHTTPRouteV1Alpha2RejectedStatus(DefaultGatewayControllerName)(updatedRoute)
			Expect(envTest.Client.Status().Update(ctx, updatedRoute)).To(Succeed())

			// then - controller should detect rejection and swap to v1 backendRef
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).ToNot(BeEmpty())

				// Check that the backendRef now points to v1 API group (after migration)
				backendRef := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(backendRef.Group).ToNot(BeNil())
				g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName),
					"HTTPRoute backendRef should point to v1 API group after Gateway rejects v1alpha2")

				// Verify migration annotation is set on HTTPRoute (one-way lock)
				g.Expect(route.Annotations).To(HaveKeyWithValue(
					llmisvc.AnnotationInferencePoolMigrated, "v1"),
					"HTTPRoute should have migration annotation after Gateway rejection")

				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Migration lock (prevents flapping)", func() {
		It("should not change migration annotation on HTTPRoute once set to v1", func(ctx SpecContext) {
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

			// Create LLMInferenceService
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for HTTPRoute to be created
			var managedRoute *gwapiv1.HTTPRoute
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				managedRoute = &routes[0]
				return nil
			}).WithContext(ctx).Should(Succeed())

			// Simulate Gateway rejection to trigger migration
			updatedRoute := managedRoute.DeepCopy()
			WithHTTPRouteV1Alpha2RejectedStatus(DefaultGatewayControllerName)(updatedRoute)
			Expect(envTest.Client.Status().Update(ctx, updatedRoute)).To(Succeed())

			// Wait for migration to complete (annotation set on HTTPRoute)
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(routes[0].Annotations).To(HaveKeyWithValue(
					llmisvc.AnnotationInferencePoolMigrated, "v1"))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// Verify migration annotation remains "v1" on HTTPRoute (not changed) across multiple reconciles
			Consistently(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				route := &routes[0]
				g.Expect(route.Annotations).To(HaveKeyWithValue(
					llmisvc.AnnotationInferencePoolMigrated, "v1"),
					"HTTPRoute migration annotation should remain 'v1' (one-way lock)")

				// Verify backendRef still points to v1
				g.Expect(route.Spec.Rules).ToNot(BeEmpty())
				g.Expect(route.Spec.Rules[0].BackendRefs).ToNot(BeEmpty())
				backendRef := route.Spec.Rules[0].BackendRefs[0]
				g.Expect(backendRef.Group).ToNot(BeNil())
				g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName))

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
