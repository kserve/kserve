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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmeta"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	leaderworkerset "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Stop Feature", func() {
	Context("When service is stopped", func() {
		It("should delete workload resources when stop annotation is set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-workload"
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
			)

			// when - Create LLMInferenceService without stop annotation
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify deployment is created
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// verify workload service is created
			workloadService := &corev1.Service{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-workload-svc",
					Namespace: nsName,
				}, workloadService)
			}).WithContext(ctx).Should(Succeed())

			// verify TLS secret is created
			tlsSecret := &corev1.Secret{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-self-signed-certs",
					Namespace: nsName,
				}, tlsSecret)
			}).WithContext(ctx).Should(Succeed())

			// when - Update LLMInferenceService with stop annotation
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

			// then - verify the service is marked as stopped
			Eventually(func(g Gomega, ctx context.Context) error {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName,
					Namespace: nsName,
				}, llmSvc)
				g.Expect(err).ToNot(HaveOccurred())

				mainWorkloadCondition := llmSvc.Status.GetCondition(v1alpha2.MainWorkloadReady)
				g.Expect(mainWorkloadCondition).ToNot(BeNil())
				g.Expect(mainWorkloadCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(mainWorkloadCondition.Reason).To(Equal("Stopped"))
				return nil
			}).WithContext(ctx).Should(Succeed(), "service should be marked as stopped")

			// verify deployment is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedDeployment)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "deployment should be deleted when service is stopped")

			// verify workload service is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-workload-svc",
					Namespace: nsName,
				}, workloadService)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "workload service should be deleted when service is stopped")

			// verify TLS secret is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-self-signed-certs",
					Namespace: nsName,
				}, tlsSecret)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "TLS secret should be deleted when service is stopped")
		})

		It("should delete router resources when stop annotation is set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-router"
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
			)

			// when - Create LLMInferenceService without stop annotation
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify HTTPRoute is created
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// when - Update LLMInferenceService with stop annotation
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

			// then - verify HTTPRoute is deleted
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(BeEmpty())
				return nil
			}).WithContext(ctx).Should(Succeed(), "HTTPRoute should be deleted when service is stopped")
		})

		It("should delete scheduler resources when stop annotation is set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-scheduler"
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

			// when - Create LLMInferenceService without stop annotation
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Ensure router and scheduler resources are ready (required for envTest)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// then - verify scheduler deployment is created
			schedulerDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, schedulerDeployment)
			}).WithContext(ctx).Should(Succeed())

			// verify scheduler service is created
			schedulerService := &corev1.Service{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-service",
					Namespace: nsName,
				}, schedulerService)
			}).WithContext(ctx).Should(Succeed())

			// verify InferencePool is created
			inferencePool := &igwapi.InferencePool{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-inference-pool",
					Namespace: nsName,
				}, inferencePool)
			}).WithContext(ctx).Should(Succeed())

			// verify InferenceModel is created
			// inferenceModel := &igwapi.InferenceObjective{}
			// Eventually(func(g Gomega, ctx context.Context) error {
			// 	return envTest.Get(ctx, types.NamespacedName{
			// 		Name:      svcName + "-inference-model",
			// 		Namespace: nsName,
			// 	}, inferenceModel)
			// }).WithContext(ctx).Should(Succeed())

			// verify scheduler ServiceAccount is created
			schedulerSA := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: nsName,
				}, schedulerSA)
			}).WithContext(ctx).Should(Succeed())

			// when - Update LLMInferenceService with stop annotation
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

			// then - verify scheduler deployment is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp",
					Namespace: nsName,
				}, schedulerDeployment)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "scheduler deployment should be deleted when service is stopped")

			// verify scheduler service is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-service",
					Namespace: nsName,
				}, schedulerService)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "scheduler service should be deleted when service is stopped")

			// verify InferencePool is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-inference-pool",
					Namespace: nsName,
				}, inferencePool)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "InferencePool should be deleted when service is stopped")

			// // verify InferenceModel is deleted
			// Eventually(func(g Gomega, ctx context.Context) bool {
			// 	err := envTest.Get(ctx, types.NamespacedName{
			// 		Name:      svcName + "-inference-model",
			// 		Namespace: nsName,
			// 	}, inferenceModel)
			// 	return err != nil && errors.IsNotFound(err)
			// }).WithContext(ctx).Should(BeTrue(), "InferenceModel should be deleted when service is stopped")

			// verify scheduler ServiceAccount is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: nsName,
				}, schedulerSA)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "scheduler ServiceAccount should be deleted when service is stopped")
		})

		It("should recreate resources when stop annotation is removed", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-restart"
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
			)

			// when - Create LLMInferenceService without stop annotation
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify deployment is created
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// when - Update LLMInferenceService with stop annotation
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

			// then - verify deployment is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedDeployment)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "deployment should be deleted when service is stopped")

			// when - Remove stop annotation to restart the service
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					delete(llmSvc.Annotations, constants.StopAnnotationKey)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - verify deployment is recreated
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed(), "deployment should be recreated when stop annotation is removed")
		})

		It("should handle multiple services with mixed stop states", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-multi"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}

			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			// Create first service
			llmSvc1 := LLMInferenceService(svcName+"-1",
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
			)
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName+"-1", nsName))).To(Succeed())
			Expect(envTest.Create(ctx, llmSvc1)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc1)).To(Succeed())
			}()

			// Create second service
			llmSvc2 := LLMInferenceService(svcName+"-2",
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
			)
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName+"-2", nsName))).To(Succeed())
			Expect(envTest.Create(ctx, llmSvc2)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc2)).To(Succeed())
			}()

			// Verify both deployments are created
			deployment1 := &appsv1.Deployment{}
			deployment2 := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-1-kserve",
					Namespace: nsName,
				}, deployment1)).To(Succeed())
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-2-kserve",
					Namespace: nsName,
				}, deployment2)).To(Succeed())
				return nil
			}).WithContext(ctx).Should(Succeed())

			// when - Stop only the first service
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc1, func() error {
					if llmSvc1.Annotations == nil {
						llmSvc1.Annotations = make(map[string]string)
					}
					llmSvc1.Annotations[constants.StopAnnotationKey] = "true"
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - first deployment should be deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-1-kserve",
					Namespace: nsName,
				}, deployment1)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "first deployment should be deleted when stopped")

			// but second deployment should still exist
			Consistently(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-2-kserve",
					Namespace: nsName,
				}, deployment2)).To(Succeed(), "second deployment should still exist")
			}).WithContext(ctx).
				WithTimeout(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(Succeed())
		})

		It("should delete multi-node LeaderWorkerSet resources when stop annotation is set", func(ctx SpecContext) {
			// given
			svcName := "test-llm-stop-multinode"
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
				WithReplicas(1),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
					WithTensorParallelism(1),
				)),
				WithWorker(SimpleWorkerPodSpec()),
			)

			// when - Create LLMInferenceService with worker (multi-node)
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify LeaderWorkerSet is created
			expectedLWS := &leaderworkerset.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedLWS)
			}).WithContext(ctx).Should(Succeed())

			// when - Update LLMInferenceService with stop annotation
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

			// then - verify the service is marked as stopped
			Eventually(func(g Gomega, ctx context.Context) error {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName,
					Namespace: nsName,
				}, llmSvc)
				g.Expect(err).ToNot(HaveOccurred())

				mainWorkloadCondition := llmSvc.Status.GetCondition(v1alpha2.MainWorkloadReady)
				g.Expect(mainWorkloadCondition).ToNot(BeNil())
				g.Expect(mainWorkloadCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(mainWorkloadCondition.Reason).To(Equal("Stopped"))
				return nil
			}).WithContext(ctx).Should(Succeed(), "service should be marked as stopped")

			// verify LeaderWorkerSet is deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedLWS)
				return err != nil && errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "LeaderWorkerSet should be deleted when service is stopped")
		})
	})
})
