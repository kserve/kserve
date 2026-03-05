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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmeta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Monitoring", func() {
	Context("Monitoring Reconciliation", func() {
		It("should create monitoring resources when llmisvc is created", func(ctx SpecContext) {
			// given
			svcName := "test-llm-monitoring"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify ServiceAccount is created
			waitForMetricsReaderServiceAccount(ctx, testNs.Name)

			// then - verify Secret is created
			expectedSecret := waitForMetricsReaderSASecret(ctx, testNs.Name)
			Expect(expectedSecret.Annotations).To(HaveKeyWithValue("kubernetes.io/service-account.name", "kserve-metrics-reader-sa"))

			// then - verify ClusterRoleBinding is created
			expectedClusterRoleBinding := waitForMetricsReaderRoleBinding(ctx, testNs.Name)
			Expect(expectedClusterRoleBinding.Subjects).To(HaveLen(1))
			Expect(expectedClusterRoleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))
			Expect(expectedClusterRoleBinding.Subjects[0].Name).To(Equal("kserve-metrics-reader-sa"))
			Expect(expectedClusterRoleBinding.Subjects[0].Namespace).To(Equal(testNs.Name))
			Expect(expectedClusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(expectedClusterRoleBinding.RoleRef.Name).To(Equal("kserve-metrics-reader-cluster-role"))

			// then - verify PodMonitor is created
			waitForVLLMEnginePodMonitor(ctx, testNs.Name)

			// then - verify ServiceMonitor is created
			expectedServiceMonitor := waitForSchedulerServiceMonitor(ctx, testNs.Name)
			Expect(expectedServiceMonitor.Spec.Endpoints).To(HaveLen(1))
			Expect(expectedServiceMonitor.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(expectedServiceMonitor.Spec.Endpoints[0].Authorization.Credentials.Name).To(Equal("kserve-metrics-reader-sa-secret"))
		})

		It("should skip cleanup when an llmisvc is deleted but other llmisvc exist in namespace", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cleanup-skip"
			testNs := NewTestNamespace(ctx, envTest)

			// Create first LLMInferenceService
			llmSvc1 := LLMInferenceService(svcName+"-1",
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
			)

			// Create second LLMInferenceService
			llmSvc2 := LLMInferenceService(svcName+"-2",
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
			)

			// when - create both services
			Expect(envTest.Create(ctx, llmSvc1)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvc2)).To(Succeed())

			// Verify monitoring resources are created
			waitForAllMonitoringResources(ctx, testNs.Name)

			// when - delete only the first service
			Expect(envTest.Delete(ctx, llmSvc1)).To(Succeed())

			// then - monitoring resources should still exist (because second service exists)
			expectedServiceAccount := &corev1.ServiceAccount{}
			expectedSecret := &corev1.Secret{}
			expectedClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
			expectedPodMonitor := &monitoringv1.PodMonitor{}
			expectedServiceMonitor := &monitoringv1.ServiceMonitor{}

			Consistently(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-metrics-reader-sa",
					Namespace: testNs.Name,
				}, expectedServiceAccount)).To(Succeed())

				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-metrics-reader-sa-secret",
					Namespace: testNs.Name,
				}, expectedSecret)).To(Succeed())

				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name: kmeta.ChildName("kserve-metrics-reader-role-binding-", testNs.Name),
				}, expectedClusterRoleBinding)).Should(Succeed())

				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-llm-isvc-vllm-engine",
					Namespace: testNs.Name,
				}, expectedPodMonitor)).Should(Succeed())

				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-llm-isvc-scheduler",
					Namespace: testNs.Name,
				}, expectedServiceMonitor)).Should(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should perform cleanup when the last llmisvc is deleted", func(ctx SpecContext) {
			// given
			svcName := "test-llm-cleanup-last"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
			)

			// when - create service
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())

			// Verify monitoring resources are created
			waitForAllMonitoringResources(ctx, testNs.Name)

			// when - delete the last (and only) service
			Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())

			// then - all monitoring resources should be deleted
			Eventually(func(ctx context.Context) bool {
				serviceAccount := &corev1.ServiceAccount{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-metrics-reader-sa",
					Namespace: testNs.Name,
				}, serviceAccount)
				return err != nil && apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "monitoring ServiceAccount should be deleted")

			Eventually(func(ctx context.Context) bool {
				secret := &corev1.Secret{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-metrics-reader-sa-secret",
					Namespace: testNs.Name,
				}, secret)
				return err != nil && apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "monitoring Secret should be deleted")

			Eventually(func(ctx context.Context) bool {
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name: kmeta.ChildName("kserve-metrics-reader-role-binding-", testNs.Name),
				}, clusterRoleBinding)
				return err != nil && apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "monitoring ClusterRoleBinding should be deleted")

			Eventually(func(ctx context.Context) bool {
				podMonitor := &monitoringv1.PodMonitor{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-llm-isvc-vllm-engine",
					Namespace: testNs.Name,
				}, podMonitor)
				return err != nil && apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "monitoring PodMonitor should be deleted")

			Eventually(func(ctx context.Context) bool {
				serviceMonitor := &monitoringv1.ServiceMonitor{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      "kserve-llm-isvc-scheduler",
					Namespace: testNs.Name,
				}, serviceMonitor)
				return err != nil && apierrors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "monitoring ServiceMonitor should be deleted")
		})
	})
})

func waitForMetricsReaderServiceAccount(ctx context.Context, nsName string) {
	expectedServiceAccount := &corev1.ServiceAccount{}
	Eventually(func(_ Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{
			Name:      "kserve-metrics-reader-sa",
			Namespace: nsName,
		}, expectedServiceAccount)
	}).WithContext(ctx).Should(Succeed())
}

func waitForMetricsReaderSASecret(ctx context.Context, nsName string) *corev1.Secret {
	expectedSecret := &corev1.Secret{}
	Eventually(func(_ Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{
			Name:      "kserve-metrics-reader-sa-secret",
			Namespace: nsName,
		}, expectedSecret)
	}).WithContext(ctx).Should(Succeed())

	return expectedSecret
}

func waitForMetricsReaderRoleBinding(ctx context.Context, nsName string) *rbacv1.ClusterRoleBinding {
	expectedClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	Eventually(func(_ Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{
			Name: kmeta.ChildName("kserve-metrics-reader-role-binding-", nsName),
		}, expectedClusterRoleBinding)
	}).WithContext(ctx).Should(Succeed())

	return expectedClusterRoleBinding
}

func waitForVLLMEnginePodMonitor(ctx context.Context, nsName string) {
	expectedPodMonitor := &monitoringv1.PodMonitor{}
	Eventually(func(_ Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{
			Name:      "kserve-llm-isvc-vllm-engine",
			Namespace: nsName,
		}, expectedPodMonitor)
	}).WithContext(ctx).Should(Succeed())
}

func waitForSchedulerServiceMonitor(ctx context.Context, nsName string) *monitoringv1.ServiceMonitor {
	expectedServiceMonitor := &monitoringv1.ServiceMonitor{}
	Eventually(func(_ Gomega, ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{
			Name:      "kserve-llm-isvc-scheduler",
			Namespace: nsName,
		}, expectedServiceMonitor)
	}).WithContext(ctx).Should(Succeed())

	return expectedServiceMonitor
}

func waitForAllMonitoringResources(ctx context.Context, nsName string) {
	waitForMetricsReaderServiceAccount(ctx, nsName)
	waitForMetricsReaderSASecret(ctx, nsName)
	waitForMetricsReaderRoleBinding(ctx, nsName)
	waitForVLLMEnginePodMonitor(ctx, nsName)
	waitForSchedulerServiceMonitor(ctx, nsName)
}
