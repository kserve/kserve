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

package inferenceservice

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// These objects skip the webhook on purpose. The defaulter names the predictor, but it never
// sees an InferenceService stored before it did, and it does not cover spec.canary[].predictor.
var _ = Describe("custom predictor without a kserve-container", func() {
	const noPredictorContainer = `predictor has no container named "` + constants.InferenceServiceContainerName + `"`

	unnamedContainers := []corev1.Container{
		{Name: "my-predictor", Image: "predictor:0.1.0"},
		{Name: "sidecar", Image: "sidecar:0.1.0"},
	}

	create := func(ctx SpecContext, name string, predictor []corev1.Container, canary ...v1beta1.CanarySpec) types.NamespacedName {
		GinkgoHelper()
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   "default",
				Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{PodSpec: v1beta1.PodSpec{Containers: predictor}},
				Canary:    canary,
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) error {
			return client.IgnoreNotFound(k8sClient.Delete(ctx, isvc))
		})
		return client.ObjectKeyFromObject(isvc)
	}

	BeforeEach(func(ctx SpecContext) {
		configMap := createInferenceServiceConfigMap(getRawKubeTestConfigs())
		Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) error {
			return client.IgnoreNotFound(k8sClient.Delete(ctx, configMap))
		})
	})

	It("marks a stored InferenceService InvalidSpec", func(ctx SpecContext) {
		key := create(ctx, "custom-predictor-stored", unnamedContainers)

		Eventually(func(g Gomega) {
			isvc := &v1beta1.InferenceService{}
			g.Expect(k8sClient.Get(ctx, key, isvc)).To(Succeed())
			g.Expect(isvc.Status.ModelStatus.TransitionStatus).To(Equal(v1beta1.InvalidSpec))
			g.Expect(isvc.Status.ModelStatus.LastFailureInfo).To(HaveValue(And(
				HaveField("Reason", v1beta1.InvalidPredictorSpec),
				HaveField("Message", noPredictorContainer),
			)))
		}, timeout, interval).Should(Succeed())

		expectAbsent(ctx, absent(&appsv1.Deployment{}, types.NamespacedName{
			Name:      constants.PredictorServiceName(key.Name),
			Namespace: key.Namespace,
		}))
	})

	// A canary builds on a copy of the InferenceService, so the error has no status to land on.
	It("records an InternalError event for a canary", func(ctx SpecContext) {
		key := create(ctx, "custom-predictor-canary",
			[]corev1.Container{{
				Name:  constants.InferenceServiceContainerName,
				Image: "predictor:0.1.0",
				Env:   []corev1.EnvVar{{Name: constants.CustomSpecStorageUriEnvVarKey, Value: "s3://test/model"}},
			}},
			v1beta1.CanarySpec{
				TrafficPercent: 10,
				Predictor: v1beta1.PredictorSpec{
					Name:    "candidate",
					PodSpec: v1beta1.PodSpec{Containers: unnamedContainers},
				},
			},
		)

		Eventually(func(g Gomega) {
			events := &corev1.EventList{}
			g.Expect(k8sClient.List(ctx, events, client.InNamespace(key.Namespace))).To(Succeed())
			g.Expect(events.Items).To(ContainElement(And(
				HaveField("InvolvedObject.Name", key.Name),
				HaveField("Reason", "InternalError"),
				HaveField("Message", ContainSubstring(noPredictorContainer)),
			)))
		}, timeout, interval).Should(Succeed())
	})
})
