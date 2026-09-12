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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

// These specs drive the controller against a Deployment created before the selector was
// narrowed, so its stored selector carries a label the reconciler no longer computes.
// spec.selector immutability and the pod template having to satisfy the selector are
// enforced by the API server, so a fake client cannot stand in here.
var _ = Describe("Deployment selector", func() {
	const queueName = "team-alpha"

	// identityLabels are the labels the main workload builder computes for a Deployment.
	identityLabels := func(svcName string) map[string]string {
		return map[string]string{
			constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
			constants.KubernetesAppNameLabelKey:   svcName,
			constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		}
	}

	// legacySelector adds the propagated queue label the pre-fix builders put in the
	// selector along with the identity labels.
	legacySelector := func(svcName string) map[string]string {
		selector := identityLabels(svcName)
		selector[LocalQueueNameLabelKey] = queueName
		return selector
	}

	// plantLegacyDeployment creates an LLMInferenceService whose Deployment already
	// exists with the legacy selector.
	//
	// The service is created with a BaseRef that does not resolve, which stops the
	// reconcile before the workload, leaving the Deployment to be created here. Clearing
	// the BaseRef then lets the controller adopt it.
	plantLegacyDeployment := func(ctx SpecContext, svcName string, testNs *TestNamespace) *v1alpha2.LLMInferenceService {
		GinkgoHelper()

		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithLabels(map[string]string{LocalQueueNameLabelKey: queueName}),
			WithBaseRefs(corev1.LocalObjectReference{Name: "does-not-exist"}),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())

		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "False"))
			llmSvc = current
		}).WithContext(ctx).Should(Succeed())

		selector := legacySelector(svcName)
		stored := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcName + "-kserve",
				Namespace: testNs.Name,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: selector},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: selector},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "placeholder", Image: "busybox"}},
					},
				},
			},
		}
		Expect(envTest.Create(ctx, stored)).To(Succeed())

		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			_, err := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
				llmSvc.Spec.BaseRefs = nil
				return nil
			})
			return err
		})).To(Succeed())

		return llmSvc
	}

	// deploymentOf reads the main Deployment of the given service.
	deploymentOf := func(ctx context.Context, g Gomega, llmSvc *v1alpha2.LLMInferenceService) *appsv1.Deployment {
		deployment := &appsv1.Deployment{}
		g.Expect(envTest.Get(ctx, types.NamespacedName{
			Name:      llmSvc.GetName() + "-kserve",
			Namespace: llmSvc.GetNamespace(),
		}, deployment)).To(Succeed())
		return deployment
	}

	It("should keep reconciling a Deployment whose stored selector it no longer computes", func(ctx SpecContext) {
		// given
		svcName := "test-llm-selector-legacy"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := plantLegacyDeployment(ctx, svcName, testNs)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		// then - the controller takes the Deployment over, replacing the planted pod spec
		Eventually(func(g Gomega, ctx context.Context) {
			deployment := deploymentOf(ctx, g, llmSvc)

			g.Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			g.Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("main"))

			g.Expect(deployment.Spec.Selector.MatchLabels).To(Equal(legacySelector(svcName)),
				"the stored selector must be carried over unchanged")
			g.Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, queueName),
				"the pod template must still satisfy the stored selector")
		}).WithContext(ctx).Should(Succeed())
	})

	It("should report that a Deployment must be recreated once its pod template stops satisfying the stored selector", func(ctx SpecContext) {
		// given
		svcName := "test-llm-selector-unsatisfied"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := plantLegacyDeployment(ctx, svcName, testNs)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(func(g Gomega, ctx context.Context) {
			deployment := deploymentOf(ctx, g, llmSvc)
			g.Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("main"))
		}).WithContext(ctx).Should(Succeed())

		// when - dropping the queue label takes it off the pod template, which the stored
		// selector still requires
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			_, err := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
				delete(llmSvc.Labels, LocalQueueNameLabelKey)
				return nil
			})
			return err
		})).To(Succeed())

		// then - the reconcile failure names the Deployment, the label and the remedy
		Eventually(func(g Gomega, ctx context.Context) {
			events := &corev1.EventList{}
			g.Expect(envTest.List(ctx, events, client.InNamespace(testNs.Name))).To(Succeed())

			messages := make([]string, 0, len(events.Items))
			for _, event := range events.Items {
				if event.InvolvedObject.Name == svcName {
					messages = append(messages, event.Message)
				}
			}
			g.Expect(messages).To(ContainElement(SatisfyAll(
				ContainSubstring(svcName+"-kserve must be recreated to reconcile"),
				ContainSubstring(LocalQueueNameLabelKey),
			)))
		}).WithContext(ctx).Should(Succeed())

		// and the stored selector is left alone
		Consistently(func(g Gomega, ctx context.Context) {
			deployment := deploymentOf(ctx, g, llmSvc)
			g.Expect(deployment.Spec.Selector.MatchLabels).To(Equal(legacySelector(svcName)))
		}).WithContext(ctx).Should(Succeed())
	})
})
