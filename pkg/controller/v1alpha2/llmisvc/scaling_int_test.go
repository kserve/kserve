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
	"path/filepath"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller - Scaling", func() {
	Context("Legacy WVA upgrade cleanup", func() {
		It("should delete legacy autoscalers and preserve workloads with admission enabled", func(ctx SpecContext) {
			// Use the production admission chain, but temporarily disable service
			// validation to seed pre-upgrade resources. Keep the webhook server alive
			// through namespace cleanup after the spec context ends.
			legacyEnv := NewEnvTest(WithWebhookManifests(
				filepath.Join(ProjectRoot(), "config", "webhook", "llmisvc", "manifests.yaml"))).
				WithWebhooks(SetupWebhooks).Start(context.Background())
			RequiredResources(ctx, legacyEnv.Client, constants.KServeNamespace)
			testNs := NewTestNamespace(ctx, legacyEnv)
			clientset, err := kubernetes.NewForConfig(legacyEnv.Config)
			Expect(err).NotTo(HaveOccurred())
			validators := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()
			validatorConfig, err := validators.Get(ctx, "llminferenceservice.serving.kserve.io", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(validators.Delete(ctx, validatorConfig.Name, metav1.DeleteOptions{})).To(Succeed())
			llmSvc := LLMInferenceService("legacy-wva",
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(&v1alpha2.ScalingSpec{
					MinReplicas: ptr.To(int32(1)), MaxReplicas: 5,
					WVA: &v1alpha2.WVASpec{VariantCost: "10.0", ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}},
				}),
				WithPrefillScaling(&v1alpha2.ScalingSpec{
					MinReplicas: ptr.To(int32(1)), MaxReplicas: 5,
					WVA: &v1alpha2.WVASpec{VariantCost: "10.0", ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
				}),
			)
			Eventually(func() error {
				return legacyEnv.Create(ctx, llmSvc)
			}).WithContext(ctx).Should(Succeed())
			originalSpec := llmSvc.Spec.DeepCopy()
			r := &llmisvc.LLMISVCReconciler{
				Client: legacyEnv.Client, Config: legacyEnv.Config,
				Clientset: clientset, EventRecorder: record.NewFakeRecorder(100),
			}
			request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(llmSvc)}
			_, err = r.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			workloads := make([]*appsv1.Deployment, 0, 2)
			legacyAutoscalers := make([]client.Object, 0, 4)
			for _, suffix := range []string{"-kserve", "-kserve-prefill"} {
				name := kmeta.ChildName(llmSvc.Name, suffix)
				deployment := &appsv1.Deployment{}
				Expect(legacyEnv.Get(ctx, types.NamespacedName{Name: name, Namespace: testNs.Name}, deployment)).To(Succeed())
				deployment.Spec.Replicas = ptr.To(int32(3))
				Expect(legacyEnv.Update(ctx, deployment)).To(Succeed())
				// envtest has no Deployment controller; emulate an already-running workload.
				deployment.Status.ObservedGeneration = deployment.Generation
				deployment.Status.Replicas = 3
				deployment.Status.ReadyReplicas = 3
				deployment.Status.UpdatedReplicas = 3
				deployment.Status.AvailableReplicas = 3
				deployment.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}
				Expect(legacyEnv.Status().Update(ctx, deployment)).To(Succeed())
				workloads = append(workloads, deployment.DeepCopy())

				metadata := metav1.ObjectMeta{
					Namespace:       testNs.Name,
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK)},
					Annotations:     map[string]string{"llm-d.ai/managed": "true", "llm-d.ai/model-id": "meta-llama/Llama-3.1-8B"},
				}
				hpa := &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: *metadata.DeepCopy(),
					Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
						ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: name},
						MinReplicas:    ptr.To(int32(1)), MaxReplicas: 5,
					},
				}
				hpa.Name = kmeta.ChildName(llmSvc.Name, suffix+"-hpa")
				so := &kedav1alpha1.ScaledObject{
					ObjectMeta: *metadata.DeepCopy(),
					Spec: kedav1alpha1.ScaledObjectSpec{
						ScaleTargetRef:  &kedav1alpha1.ScaleTarget{APIVersion: "apps/v1", Kind: "Deployment", Name: name},
						MinReplicaCount: ptr.To(int32(1)), MaxReplicaCount: ptr.To(int32(5)),
						Triggers: []kedav1alpha1.ScaleTriggers{{Type: "prometheus", Metadata: map[string]string{
							"serverAddress": "http://prometheus:9090", "query": "wva_desired_replicas", "threshold": "1",
						}}},
					},
				}
				so.Name = kmeta.ChildName(llmSvc.Name, suffix+"-keda")
				Expect(legacyEnv.Create(ctx, hpa)).To(Succeed())
				Expect(legacyEnv.Create(ctx, so)).To(Succeed())
				legacyAutoscalers = append(legacyAutoscalers, hpa, so)
			}

			validatorConfig.ResourceVersion = ""
			_, err = validators.Create(ctx, validatorConfig, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			// Confirm admission has observed re-enablement before testing reconciliation.
			newService := llmSvc.DeepCopy()
			newService.Name = "new-wva"
			newService.ResourceVersion = ""
			newService.UID = ""
			Eventually(func() error {
				return legacyEnv.Create(ctx, newService, client.DryRunAll)
			}).WithContext(ctx).Should(MatchError(ContainSubstring("WVA autoscaling is no longer supported")))
			for _, prefill := range []bool{false, true} {
				current := &v1alpha2.LLMInferenceService{}
				Expect(legacyEnv.Get(ctx, request.NamespacedName, current)).To(Succeed())
				if prefill {
					current.Spec.Prefill.Scaling.WVA.VariantCost = "20.0"
				} else {
					current.Spec.Scaling.WVA.VariantCost = "20.0"
				}
				Expect(legacyEnv.Update(ctx, current)).To(MatchError(ContainSubstring("existing WVA configuration must remain unchanged or be removed")))
			}
			current := &v1alpha2.LLMInferenceService{}
			Expect(legacyEnv.Get(ctx, request.NamespacedName, current)).To(Succeed())
			current.Labels = map[string]string{"upgrade": "retained-wva"}
			Expect(legacyEnv.Update(ctx, current)).To(Succeed(), "unrelated updates must allow unchanged WVA")

			// Verify cleanup is idempotent and cannot recreate WVA autoscalers.
			for range 2 {
				_, err = r.Reconcile(ctx, request)
				Expect(err).NotTo(HaveOccurred())
				for _, autoscaler := range legacyAutoscalers {
					Expect(apierrors.IsNotFound(legacyEnv.Get(ctx, client.ObjectKeyFromObject(autoscaler), autoscaler))).To(BeTrue())
				}
				for _, original := range workloads {
					current := &appsv1.Deployment{}
					Expect(legacyEnv.Get(ctx, client.ObjectKeyFromObject(original), current)).To(Succeed())
					Expect(current.UID).To(Equal(original.UID))
					Expect(current.Spec).To(Equal(original.Spec))
				}
				current := &v1alpha2.LLMInferenceService{}
				Expect(legacyEnv.Get(ctx, request.NamespacedName, current)).To(Succeed())
				Expect(current.Spec).To(Equal(*originalSpec))
				for _, conditionType := range []apis.ConditionType{v1alpha2.ScalingReady, v1alpha2.PrefillScalingReady} {
					condition := current.Status.GetCondition(conditionType)
					Expect(condition).NotTo(BeNil())
					Expect(condition.IsFalse()).To(BeTrue())
					Expect(condition.Reason).To(Equal(v1alpha2.WVAUnsupportedReason))
				}
				condition := current.Status.GetCondition(v1alpha2.WorkloadReady)
				Expect(condition).NotTo(BeNil())
				Expect(condition.IsTrue()).To(BeTrue(), "WVAUnsupported must not block workload readiness")
			}
		})
	})

	Context("Direct KEDA scaling", func() {
		It("should create ScaledObject with user triggers and without legacy annotations", func(ctx SpecContext) {
			svcName := "test-direct-keda-scaling"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(so.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
				g.Expect(so).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should create ScaledObject targeting LeaderWorkerSet with user triggers and without legacy annotations when worker is set", func(ctx SpecContext) {
			svcName := "test-direct-keda-lws"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{
					Containers: []corev1.Container{{Name: "worker", Image: "vllm:latest"}},
				}),
				WithScaling(DirectKEDAScaling(1, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.APIVersion).To(Equal(lwsapi.GroupVersion.String()))
				g.Expect(so.Spec.ScaleTargetRef.Kind).To(Equal("LeaderWorkerSet"))
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
				g.Expect(so).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should accept idleReplicaCount=0 for scale-to-zero", func(ctx SpecContext) {
			svcName := "test-direct-keda-scale-to-zero"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScalingWithIdleReplicaCount(1, 8, 0,
					kedav1alpha1.ScaleTriggers{
						Type:     "cpu",
						Metadata: map[string]string{"value": "80"},
					},
				)),
			)

			// Regression test: the CRD schema previously rejected idleReplicaCount=0
			// (minimum:1), which made true scale-to-zero unreachable.
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.IdleReplicaCount).To(Equal(ptr.To(int32(0))))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should reject create when direct KEDA scaling has no triggers", func(ctx SpecContext) {
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService("test-direct-keda-no-triggers",
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithScaling(&v1alpha2.ScalingSpec{
					MaxReplicas: 5,
					KEDA: &v1alpha2.DirectKEDAScalingSpec{
						Triggers: []kedav1alpha1.ScaleTriggers{},
					},
				}),
			)

			errValidation := envTest.Create(ctx, llmSvc)

			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail when direct KEDA triggers are empty")
			Expect(errValidation.Error()).To(ContainSubstring("at least one trigger is required when using direct KEDA scaling"))
		})

		It("should update direct KEDA triggers in place without adding legacy annotations", func(ctx SpecContext) {
			svcName := "test-direct-keda-trigger-update"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = DirectKEDAScaling(2, 10,
						kedav1alpha1.ScaleTriggers{
							Type: "cpu",
							Metadata: map[string]string{
								"value": "60",
							},
						},
						kedav1alpha1.ScaleTriggers{
							Type: "memory",
							Metadata: map[string]string{
								"value": "70",
							},
						},
					)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(10))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("60"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete ScaledObject when direct KEDA scaling is removed", func(ctx SpecContext) {
			svcName := "test-direct-keda-cleanup"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = nil
					llmSvc.Spec.Replicas = ptr.To(int32(3))
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Prefill scaling", func() {
		It("should create separate direct KEDA scaling resources for decode and prefill workloads", func(ctx SpecContext) {
			svcName := "test-prefill-direct-keda"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
				)),
				WithPrefillScaling(DirectKEDAScaling(2, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			decodeSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			prefillSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, decodeSOKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, prefillSOKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-prefill")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("No scaling configured", func() {
		It("should not create any scaling resources when scaling is nil", func(ctx SpecContext) {
			svcName := "test-no-scaling"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())

				err = envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Scaling with stop annotation", func() {
		It("should delete KEDA scaling resources when stop annotation is set", func(ctx SpecContext) {
			svcName := "test-keda-stop"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					WithAnnotations(map[string]string{
						constants.StopAnnotationKey: "true",
					})(llmSvc)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted when stopped")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("ScalingReady condition propagation", func() {
		It("should not have ScalingReady when no scaling is configured", func(ctx SpecContext) {
			svcName := "test-no-scaling-cond"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the controller to reconcile (WorkloadsReady should appear)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.WorkloadReady)).ToNot(BeNil())
			}).WithContext(ctx).Should(Succeed())

			// Verify ScalingReady is absent
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.ScalingReady)).To(BeNil(),
					"ScalingReady should be absent when no scaling is configured")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set ScalingReady=False when KEDA ScaledObject is not yet ready", func(ctx SpecContext) {
			svcName := "test-keda-cond-prog"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should exist")
				g.Expect(scalingCond.IsFalse()).To(BeTrue(), "ScalingReady should be False when ScaledObject has no conditions")
				g.Expect(scalingCond.Reason).To(Equal("ScaledObjectProgressing"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set ScalingReady=True when KEDA ScaledObject reports Ready", func(ctx SpecContext) {
			svcName := "test-keda-cond-ok"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Simulate KEDA setting ready conditions
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "True", Reason: "ScaledObjectReady"},
					{Type: kedav1alpha1.ConditionActive, Status: "True"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should exist")
				g.Expect(scalingCond.IsTrue()).To(BeTrue(), "ScalingReady should be True when ScaledObject is ready")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should transition ScalingReady from True to False when KEDA ScaledObject degrades", func(ctx SpecContext) {
			svcName := "test-keda-true-false"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Phase 1: set ScaledObject to ready
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "True", Reason: "ScaledObjectReady"},
					{Type: kedav1alpha1.ConditionActive, Status: "True"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil())
				g.Expect(scalingCond.IsTrue()).To(BeTrue(), "ScalingReady should be True initially")
			}).WithContext(ctx).Should(Succeed())

			// Phase 2: degrade ScaledObject (trigger error)
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "False", Reason: "TriggerError", Message: "prometheus query failed"},
					{Type: kedav1alpha1.ConditionActive, Status: "False"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should still exist")
				g.Expect(scalingCond.IsFalse()).To(BeTrue(), "ScalingReady should flip to False after ScaledObject degrades")
				g.Expect(scalingCond.Reason).To(Equal("TriggerError"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set PrefillScalingReady when direct KEDA prefill scaling is configured", func(ctx SpecContext) {
			svcName := "test-prefill-scale"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
				WithPrefillScaling(DirectKEDAScaling(1, 3)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady should exist for decode workload")

				prefillScalingCond := current.Status.GetCondition(v1alpha2.PrefillScalingReady)
				g.Expect(prefillScalingCond).ToNot(BeNil(), "PrefillScalingReady should exist for prefill workload")
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
