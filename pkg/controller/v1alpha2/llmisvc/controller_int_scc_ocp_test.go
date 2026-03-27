//go:build distro

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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService OCP SCC Controller", func() {
	Context("Multi-Node SCC RoleBinding Reconciliation", func() {
		It("should create SCC RoleBinding when worker spec is set", func(ctx SpecContext) {
			// given
			svcName := "test-scc-worker"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - SCC RoleBinding should be created
			sccRB := &rbacv1.RoleBinding{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
					Namespace: testNs.Name,
				}, sccRB)
			}).WithContext(ctx).Should(Succeed())

			Expect(sccRB).To(BeOwnedBy(llmSvc))
			Expect(sccRB.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(sccRB.RoleRef.Name).To(Equal("openshift-ai-llminferenceservice-scc"))
			Expect(sccRB.RoleRef.APIGroup).To(Equal(rbacv1.GroupName))

			// Verify subjects contain both main and prefill service accounts
			Expect(sccRB.Subjects).To(HaveLen(2))
			Expect(sccRB.Subjects[0].Kind).To(Equal("ServiceAccount"))
			Expect(sccRB.Subjects[0].Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn")))
			Expect(sccRB.Subjects[1].Kind).To(Equal("ServiceAccount"))
			Expect(sccRB.Subjects[1].Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn-prefill")))
		})

		It("should create SCC RoleBinding when prefill worker spec is set", func(ctx SpecContext) {
			// given
			svcName := "test-scc-prefill"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithPrefillParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithPrefillWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - SCC RoleBinding should be created
			sccRB := &rbacv1.RoleBinding{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
					Namespace: testNs.Name,
				}, sccRB)
			}).WithContext(ctx).Should(Succeed())

			Expect(sccRB).To(BeOwnedBy(llmSvc))
			Expect(sccRB.RoleRef.Name).To(Equal("openshift-ai-llminferenceservice-scc"))
		})

		It("should create SCC RoleBinding with both worker and prefill specs", func(ctx SpecContext) {
			// given
			svcName := "test-scc-both"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(SimpleWorkerPodSpec()),
				WithPrefillParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithPrefillWorker(SimpleWorkerPodSpec()),
				WithPrefillReplicas(1),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - SCC RoleBinding should be created
			sccRB := &rbacv1.RoleBinding{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
					Namespace: testNs.Name,
				}, sccRB)
			}).WithContext(ctx).Should(Succeed())

			Expect(sccRB).To(BeOwnedBy(llmSvc))
			Expect(sccRB.Subjects).To(HaveLen(2))
			Expect(sccRB.Subjects[0].Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn")))
			Expect(sccRB.Subjects[1].Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn-prefill")))
		})

		It("should delete SCC RoleBinding when worker and prefill specs are removed", func(ctx SpecContext) {
			// given
			svcName := "test-scc-cleanup"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			sccRBName := types.NamespacedName{
				Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
				Namespace: testNs.Name,
			}

			// Verify SCC RoleBinding is created
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, sccRBName, &rbacv1.RoleBinding{})
			}).WithContext(ctx).Should(Succeed())

			// when - Remove worker spec
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Worker = nil
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - SCC RoleBinding should be deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, sccRBName, &rbacv1.RoleBinding{})
				return apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue())
		})

		It("should delete SCC RoleBinding when stop annotation is set", func(ctx SpecContext) {
			// given
			svcName := "test-scc-stop"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			sccRBName := types.NamespacedName{
				Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
				Namespace: testNs.Name,
			}

			// Verify SCC RoleBinding is created
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, sccRBName, &rbacv1.RoleBinding{})
			}).WithContext(ctx).Should(Succeed())

			// when - Set the stop annotation
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					if llmSvc.Annotations == nil {
						llmSvc.Annotations = make(map[string]string)
					}
					llmSvc.Annotations[constants.StopAnnotationKey] = "true"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - SCC RoleBinding should be deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, sccRBName, &rbacv1.RoleBinding{})
				return apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue())
		})

		It("should have correct labels on SCC RoleBinding", func(ctx SpecContext) {
			// given
			svcName := "test-scc-labels"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			sccRB := &rbacv1.RoleBinding{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-mn-scc"),
					Namespace: testNs.Name,
				}, sccRB)
			}).WithContext(ctx).Should(Succeed())

			Expect(sccRB.Labels).To(HaveKeyWithValue(constants.KubernetesAppNameLabelKey, svcName))
			Expect(sccRB.Labels).To(HaveKeyWithValue(constants.KubernetesPartOfLabelKey, constants.LLMInferenceServicePartOfValue))
		})
	})
})
