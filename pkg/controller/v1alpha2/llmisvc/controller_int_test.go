/*
Copyright 2023 The KServe Authors.

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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

const (
	DefaultGatewayControllerName = "gateway.networking.k8s.io/gateway-controller"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("Basic Reconciliation", func() {
		It("should create a basic single node deployment with just base refs", func(ctx SpecContext) {
			// given
			svcName := "test-llm"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			modelConfig := LLMInferenceServiceConfig("model-fb-opt-125m",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)

			routerConfig := LLMInferenceServiceConfig("router-managed",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigManagedRouter(),
			)

			workloadConfig := LLMInferenceServiceConfig("workload-single-cpu",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "quay.io/pierdipi/vllm-cpu:latest",
							Env: []corev1.EnvVar{
								{
									Name:  "VLLM_LOGGING_LEVEL",
									Value: "DEBUG",
								},
							},
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    5,
								InitialDelaySeconds: 30,
								PeriodSeconds:       30,
								TimeoutSeconds:      30,
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("10Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				}),
			)

			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, workloadConfig)).To(Succeed())

			// Create LLMInferenceService using baseRefs only
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-fb-opt-125m"},
					corev1.LocalObjectReference{Name: "router-managed"},
					corev1.LocalObjectReference{Name: "workload-single-cpu"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedDeployment.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			Expect(expectedDeployment).To(HaveContainerImage("quay.io/pierdipi/vllm-cpu:latest")) // Coming from preset
			Expect(expectedDeployment).To(BeOwnedBy(llmSvc))

			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(llmisvc.IsHTTPRouteReady(&routes[0])).To(BeTrue())
				return nil
			}).WithContext(ctx).Should(Succeed())

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
			})).WithContext(ctx).Should(Succeed())
		})

		It("should propagate kueue labels and annotations to the deployment", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kueue"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			localQueueName := "test-local-q"
			preemptPriority := "0"
			testValue := "test"

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				// Add a kueue label and annotation to ensure value propagation to the deployment
				// the kueue functionality itself will not be tested here
				WithAnnotations(map[string]string{
					PreemptionReclaimAnnotationKey: preemptPriority,
					testValue:                      testValue, // dummy value, should not be propagated
				}),
				WithLabels(map[string]string{
					LocalQueueNameLabelKey: localQueueName,
					testValue:              testValue, // dummy value, should not be propagated
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedDeployment.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			Expect(expectedDeployment).To(BeOwnedBy(llmSvc))

			By("checking the Deployment's top-level metadata")
			// Check that the kueue label/annotation was propagated
			Expect(expectedDeployment.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, localQueueName))
			Expect(expectedDeployment.Annotations).To(gomega.HaveKeyWithValue(PreemptionReclaimAnnotationKey, preemptPriority))
			// Check that the test label/annotation was not propagated as it is not in the approved prefixes for propagation
			Expect(expectedDeployment.Labels).ToNot(HaveKeyWithValue(testValue, testValue))
			Expect(expectedDeployment.Annotations).ToNot(HaveKeyWithValue(testValue, testValue))

			By("checking the Deployment's pod template metadata")
			// Check that the kueue label/annotation was propagated
			Expect(expectedDeployment.Spec.Template.Labels).To(HaveKeyWithValue(LocalQueueNameLabelKey, localQueueName))
			Expect(expectedDeployment.Spec.Template.Annotations).To(gomega.HaveKeyWithValue(PreemptionReclaimAnnotationKey, preemptPriority))
			// Check that the test label/annotation was not propagated as it is not in the approved prefixes for propagation
			Expect(expectedDeployment.Spec.Template.Labels).ToNot(HaveKeyWithValue(testValue, testValue))
			Expect(expectedDeployment.Spec.Template.Annotations).ToNot(HaveKeyWithValue(testValue, testValue))
		})

		It("should preserve externally set replicas when owner does not specify replicas", func(ctx SpecContext) {
			// given
			svcName := "test-llm-preserve-replicas"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))
			deploymentName := types.NamespacedName{Name: svcName + "-kserve", Namespace: testNs.Name}

			// Create LLMInferenceService WITHOUT specifying replicas
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				// Note: Not using WithReplicas() - replicas should be nil
			)

			// Verify replicas is nil in the spec
			Expect(llmSvc.Spec.Replicas).To(BeNil())

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - Deployment should be created with default replicas (1)
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, deploymentName, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedDeployment.Spec.Replicas).To(Equal(ptr.To[int32](1)))

			// Simulate external scaling (e.g., HPA scaling to 3 replicas)
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				deployment := &appsv1.Deployment{}
				if err := envTest.Get(ctx, deploymentName, deployment); err != nil {
					return err
				}
				deployment.Spec.Replicas = ptr.To[int32](3)
				return envTest.Client.Update(ctx, deployment)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Trigger a reconciliation by updating a spec field in the LLMInferenceService.
			// Using a spec change (Model.Name) rather than an annotation ensures
			// compatibility with GenerationChangedPredicate if it's added in the future.
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Model.Name = ptr.To(*llmSvc.Spec.Model.Name + "-updated")
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Verify the externally set replicas are preserved after reconciliation
			Consistently(func(g Gomega, ctx context.Context) {
				deployment := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, deploymentName, deployment)).To(Succeed())

				g.Expect(deployment.Spec.Replicas).To(Equal(ptr.To[int32](3)),
					"Externally set replicas should be preserved when owner doesn't specify replicas")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should override externally set replicas when owner specifies replicas", func(ctx SpecContext) {
			// given
			svcName := "test-llm-override-replicas"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))
			deploymentName := types.NamespacedName{Name: svcName + "-kserve", Namespace: testNs.Name}

			// Create LLMInferenceService WITH explicit replicas
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithReplicas(2), // Owner explicitly sets replicas
			)

			// Verify replicas is set in the spec
			Expect(llmSvc.Spec.Replicas).To(Equal(ptr.To[int32](2)))

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - Deployment should be created with owner-specified replicas (2)
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, deploymentName, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedDeployment.Spec.Replicas).To(Equal(ptr.To[int32](2)))

			// Simulate external scaling attempt (e.g., someone tries to manually scale to 5)
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				deployment := &appsv1.Deployment{}
				if err := envTest.Get(ctx, deploymentName, deployment); err != nil {
					return err
				}
				deployment.Spec.Replicas = ptr.To[int32](5)
				return envTest.Client.Update(ctx, deployment)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Trigger a reconciliation by updating a spec field in the LLMInferenceService.
			// Using a spec change (Model.Name) rather than an annotation ensures
			// compatibility with GenerationChangedPredicate if it's added in the future.
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Model.Name = ptr.To(*llmSvc.Spec.Model.Name + "-updated")
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Verify the owner-specified replicas override the external change
			Eventually(func(g Gomega, ctx context.Context) {
				deployment := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, deploymentName, deployment)).To(Succeed())

				g.Expect(deployment.Spec.Replicas).To(Equal(ptr.To[int32](2)),
					"Owner-specified replicas should override externally set replicas")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Routing reconciliation ", func() {
		When("HTTP route is managed", func() {
			It("should create routes pointing to the default gateway when both are managed", func(ctx SpecContext) {
				// given
				svcName := "test-llm-create-http-route"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
					WithManagedScheduler(),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then
				expectedHTTPRoute := &gwapiv1.HTTPRoute{}
				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))
					expectedHTTPRoute = &routes[0]

					return nil
				}).WithContext(ctx).Should(Succeed())

				Expect(expectedHTTPRoute).To(BeControlledBy(llmSvc))
				Expect(expectedHTTPRoute).To(HaveGatewayRefs(gwapiv1.ParentReference{Name: "kserve-ingress-gateway"}))
				// With completions-only routing, the catch-all rule uses a Service backend
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefService(svcName + "-kserve-workload-svc")))

				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				// HTTPRoute uses v1alpha2 backendRef for InferencePool rules when both CRDs are available
				Eventually(func(g Gomega, ctx context.Context) {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))
					g.Expect(&routes[0]).To(HaveBackendRefs(BackendRefInferencePoolV1Alpha2(svcName + "-inference-pool")))
				}).WithContext(ctx).Should(Succeed())

				Eventually(func(g Gomega, ctx context.Context) error {
					ip := igwapi.InferencePool{}
					return envTest.Client.Get(ctx, client.ObjectKey{Name: svcName + "-inference-pool", Namespace: llmSvc.GetNamespace()}, &ip)
				}).WithContext(ctx).Should(Succeed())

				// Verify the scheduler service (EPP service) has the expected ports including zmq
				Eventually(func(g Gomega, ctx context.Context) error {
					eppSvc := &corev1.Service{}
					g.Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: svcName + "-epp-service", Namespace: llmSvc.GetNamespace()}, eppSvc)).To(Succeed())

					// Verify all expected ports are present (grpc, grpc-health, metrics, zmq)
					portNames := make(map[string]int32)
					for _, port := range eppSvc.Spec.Ports {
						portNames[port.Name] = port.Port
					}

					g.Expect(portNames).To(HaveKeyWithValue("grpc", int32(9002)))
					g.Expect(portNames).To(HaveKeyWithValue("grpc-health", int32(9003)))
					g.Expect(portNames).To(HaveKeyWithValue("metrics", int32(9090)))
					g.Expect(portNames).To(HaveKeyWithValue("zmq", int32(5557)))
					g.Expect(eppSvc.Spec.Ports).To(HaveLen(4))

					return nil
				}).WithContext(ctx).Should(Succeed())

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should use referenced external InferencePool", func(ctx SpecContext) {
				// given
				svcName := "test-llm-create-http-route-inf-pool-ref"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				infPoolName := kmeta.ChildName(svcName, "-my-inf-pool")

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
					WithInferencePoolRef(infPoolName),
				)

				infPool := InferencePool(infPoolName,
					InNamespace[*igwapi.InferencePool](testNs.Name),
					WithSelector("app", "workload"),
					WithTargetPort(8000),
					WithExtensionRef("", "Service", kmeta.ChildName(svcName, "-epp-service"), 9002),
					WithInferencePoolReadyStatus(),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				Expect(envTest.Create(ctx, infPool)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
					testNs.DeleteAndWait(ctx, infPool)
				}()

				// then
				expectedHTTPRoute := &gwapiv1.HTTPRoute{}
				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))
					expectedHTTPRoute = &routes[0]

					return nil
				}).WithContext(ctx).Should(Succeed())

				Expect(expectedHTTPRoute).To(BeControlledBy(llmSvc))
				Expect(expectedHTTPRoute).To(HaveGatewayRefs(gwapiv1.ParentReference{Name: "kserve-ingress-gateway"}))
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefInferencePool(infPoolName)))
				// With completions-only routing, the catch-all rule uses a Service backend
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefService(svcName + "-kserve-workload-svc")))

				ensureInferencePoolReady(ctx, envTest.Client, infPool)
				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should create routes pointing to workload service when no scheduler is configured", func(ctx SpecContext) {
				// given
				llmSvcName := "test-llm-create-http-route-no-scheduler"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(llmSvcName))

				llmSvc := LLMInferenceService(llmSvcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then
				expectedHTTPRoute := &gwapiv1.HTTPRoute{}
				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))
					expectedHTTPRoute = &routes[0]

					return nil
				}).WithContext(ctx).Should(Succeed())

				svcName := kmeta.ChildName(llmSvcName, "-kserve-workload-svc")

				Expect(expectedHTTPRoute).To(BeControlledBy(llmSvc))
				Expect(expectedHTTPRoute).To(HaveGatewayRefs(gwapiv1.ParentReference{Name: "kserve-ingress-gateway"}))
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefService(svcName)))
				Expect(expectedHTTPRoute).To(Not(HaveBackendRefs(BackendRefInferencePool(kmeta.ChildName(llmSvcName, "-inference-pool")))))

				Eventually(func(g Gomega, ctx context.Context) error {
					svc := &corev1.Service{}
					err := envTest.Client.Get(ctx, client.ObjectKey{Name: svcName, Namespace: testNs.Name}, svc)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(svc.Spec.Selector).To(Equal(llmisvc.GetWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvc.Spec)))
					return nil
				})

				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())

				Consistently(func(g Gomega, ctx context.Context) error {
					ip := igwapi.InferencePool{}
					return envTest.Client.Get(ctx, client.ObjectKey{Name: llmSvcName + "-inference-pool", Namespace: llmSvc.GetNamespace()}, &ip)
				}).WithContext(ctx).
					Within(2 * time.Second).
					WithPolling(300 * time.Millisecond).
					Should(HaveOccurred())
			})

			It("should create HTTPRoute with defined spec", func(ctx SpecContext) {
				// given
				svcName := "test-llm-defined-http-route"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedGateway(),
					WithHTTPRouteSpec(customRouteSpec(ctx, envTest.Client, testNs.Name, "my-ingress-gateway", "my-inference-service")),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				expectedHTTPRoute := &gwapiv1.HTTPRoute{}

				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))
					expectedHTTPRoute = &routes[0]
					return nil
				}).WithContext(ctx).Should(Not(HaveOccurred()), "HTTPRoute should be created")

				Expect(expectedHTTPRoute).To(BeControlledBy(llmSvc))
				Expect(expectedHTTPRoute).To(HaveGatewayRefs(gwapiv1.ParentReference{Name: "my-ingress-gateway"}))
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefService("my-inference-service")))
				Expect(expectedHTTPRoute).To(Not(HaveBackendRefs(BackendRefInferencePool(kmeta.ChildName(svcName, "-inference-pool")))))

				// Advanced fixture pattern: Update the HTTPRoute status using fixture functions
				updatedRoute := expectedHTTPRoute.DeepCopy()
				WithHTTPRouteReadyStatus(DefaultGatewayControllerName)(updatedRoute)
				Expect(envTest.Client.Status().Update(ctx, updatedRoute)).To(Succeed())

				ensureSchedulerDeploymentReady(ctx, envTest.Client, llmSvc)

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should delete managed HTTPRoute when ref is defined", func(ctx SpecContext) {
				// given
				svcName := "test-llm-update-http-route"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				// Create the Gateway that the router-managed preset references
				gateway := Gateway("my-ingress-gateway",
					InNamespace[*gwapiv1.Gateway](testNs.Name),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
					// Don't set the condition here initially
				)
				Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())

				// Ensure the gateway becomes ready
				ensureGatewayReady(ctx, envTest.Client, gateway)

				defer func() {
					testNs.DeleteAndWait(ctx, gateway)
				}()

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
				)

				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))

					return nil
				}).WithContext(ctx).Should(Succeed())

				customHTTPRoute := HTTPRoute("my-custom-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
					WithParentRef(GatewayParentRef(gateway.Name, gateway.Namespace)),
					WithHTTPRouteRule(
						HTTPRouteRuleWithBackendAndTimeouts(svcName+"-inference-pool", 8000, "/", "0s", "0s"),
					),
				}...)
				Expect(envTest.Client.Create(ctx, customHTTPRoute)).To(Succeed())

				// Make the HTTPRoute ready
				ensureHTTPRouteReady(ctx, envTest.Client, customHTTPRoute)

				// when - Update the HTTPRoute spec
				errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
						WithHTTPRouteRefs(HTTPRouteRef("my-custom-route"))(llmSvc)
						WithGatewayRefs(LLMGatewayRef(gateway.Name, gateway.Namespace))(llmSvc)
						return nil
					})
					return errUpdate
				})
				Expect(errRetry).ToNot(HaveOccurred())

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(BeEmpty())

					return nil
				}).WithContext(ctx).Should(Succeed())

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should evaluate HTTPRoute readiness conditions", func(ctx SpecContext) {
				// given
				svcName := "test-llm-httproute-conditions"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				ingressGateway := DefaultGateway(testNs.Name)
				Expect(envTest.Client.Create(ctx, ingressGateway)).To(Succeed())
				ensureGatewayReady(ctx, envTest.Client, ingressGateway)

				defer func() {
					testNs.DeleteAndWait(ctx, ingressGateway)
				}()

				customHTTPRoute := HTTPRoute("my-custom-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
					WithParentRef(GatewayParentRef("kserve-ingress-gateway", testNs.Name)),
					WithHTTPRouteRule(
						HTTPRouteRuleWithBackendAndTimeouts(svcName+"-inference-pool", 8000, "/", "0s", "0s"),
					),
				}...)
				Expect(envTest.Client.Create(ctx, customHTTPRoute)).To(Succeed())

				// Make the HTTPRoute ready
				ensureHTTPRouteReady(ctx, envTest.Client, customHTTPRoute)

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("my-custom-route")),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then - verify HTTPRoutesReady condition is set
				Eventually(func(g Gomega, ctx context.Context) error {
					current := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

					// Check that HTTPRoutesReady condition exists and is True
					httpRoutesCondition := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
					g.Expect(httpRoutesCondition).ToNot(BeNil(), "HTTPRoutesReady condition should be set")
					g.Expect(httpRoutesCondition.IsTrue()).To(BeTrue(), "HTTPRoutesReady condition should be True")

					return nil
				}).WithContext(ctx).Should(Succeed(), "HTTPRoutesReady condition should be set to True")

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})
		})

		When("transitioning from managed to unmanaged router", func() {
			DescribeTable("owned resources should be deleted",

				func(ctx SpecContext, testName string, initialRouterSpec *v1alpha2.RouterSpec, specMutation func(*v1alpha2.LLMInferenceService)) {
					// given
					svcName := "test-llm-" + testName
					testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

					llmSvc := LLMInferenceService(svcName,
						InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
						WithModelURI("hf://facebook/opt-125m"),
					)
					llmSvc.Spec.Router = initialRouterSpec

					// when - Create LLMInferenceService
					Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
					defer func() {
						testNs.DeleteAndWait(ctx, llmSvc)
					}()

					// then - HTTPRoute should be created with router labels
					Eventually(func(g Gomega, ctx context.Context) error {
						routes, errList := managedRoutes(ctx, llmSvc)
						g.Expect(errList).ToNot(HaveOccurred())
						g.Expect(routes).To(HaveLen(1))

						return nil
					}).WithContext(ctx).Should(Succeed(), "Should have managed HTTPRoute")

					// when - Update LLMInferenceService using the provided update function
					errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
						_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
							specMutation(llmSvc)
							return nil
						})
						return errUpdate
					})

					Expect(errRetry).ToNot(HaveOccurred())

					// then - HTTPRoute with router labels should be deleted
					Eventually(func(g Gomega, ctx context.Context) error {
						routes, errList := managedRoutes(ctx, llmSvc)
						g.Expect(errList).ToNot(HaveOccurred())
						g.Expect(routes).To(BeEmpty())

						return nil
					}).WithContext(ctx).Should(Succeed(), "Should have no managed HTTPRoutes with router when ")

					Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
				},
				Entry("should delete HTTPRoutes when spec.Router is set to nil",
					"router-spec-nil",
					&v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{}, // Default empty spec
						},
						Gateway: &v1alpha2.GatewaySpec{},
					},
					func(llmSvc *v1alpha2.LLMInferenceService) {
						llmSvc.Spec.Router = nil
					},
				),
			)
		})
	})

	Context("Custom Gateway Reference", func() {
		It("should not require default gateway when custom gateway is specified", func(ctx SpecContext) {
			// given - remove the default gateway
			defaultGateway := &gwapiv1.Gateway{}
			Expect(envTest.Client.Get(ctx, types.NamespacedName{
				Name:      constants.GatewayName,
				Namespace: constants.KServeNamespace,
			}, defaultGateway)).To(Succeed())

			gatewayToRestore := defaultGateway.DeepCopy()
			gatewayToRestore.ResourceVersion = "" // Clear for re-creation

			DeferCleanup(func(ctx context.Context) {
				// Restore the default gateway after the test (pass or fail)
				existing := &gwapiv1.Gateway{}
				err := envTest.Client.Get(ctx, types.NamespacedName{
					Name:      constants.GatewayName,
					Namespace: constants.KServeNamespace,
				}, existing)
				if err == nil {
					// Gateway already exists, no restoration needed.
					return
				}
				// Whether NotFound or transient error, attempt to restore.
				Expect(client.IgnoreAlreadyExists(envTest.Client.Create(ctx, gatewayToRestore))).To(Succeed())
			})

			Expect(envTest.Client.Delete(ctx, defaultGateway)).To(Succeed())

			// Ensure the default gateway is actually deleted
			Eventually(func(g Gomega, ctx context.Context) bool {
				err := envTest.Client.Get(ctx, types.NamespacedName{
					Name:      constants.GatewayName,
					Namespace: constants.KServeNamespace,
				}, &gwapiv1.Gateway{})
				return errors.IsNotFound(err)
			}).WithContext(ctx).Should(BeTrue(), "default gateway should be deleted")

			// Create test namespace
			svcName := "test-llm-custom-gateway"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(ctx, namespace)
			}()

			customGatewayName := "my-custom-gateway"
			customGateway := Gateway(customGatewayName,
				InNamespace[*gwapiv1.Gateway](nsName),
				WithListener(gwapiv1.HTTPProtocolType),
				WithAddresses("203.0.113.42"),
			)
			Expect(envTest.Client.Create(ctx, customGateway)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, customGateway)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithGatewayRefs(LLMGatewayRef(customGatewayName, nsName)),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for HTTPRoute to be created and verify it references the custom gateway
			var createdRoute *gwapiv1.HTTPRoute
			Eventually(func(g Gomega, ctx context.Context) error {
				routes, errList := managedRoutes(ctx, llmSvc)
				g.Expect(errList).ToNot(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))

				g.Expect(&routes[0]).To(HaveGatewayRefs(gwapiv1.ParentReference{
					Name:      gwapiv1.ObjectName(customGatewayName),
					Namespace: ptr.To(gwapiv1.Namespace(nsName)),
				}))

				createdRoute = &routes[0]
				return nil
			}).WithContext(ctx).Should(Succeed(), "HTTPRoute should reference the custom gateway")

			// Simulate gateway controller setting HTTPRoute status
			ensureHTTPRouteReady(ctx, envTest.Client, createdRoute)

			// then - inspect status
			Eventually(func(g Gomega, ctx context.Context) error {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				// Check that RouterReady condition exists and is True
				routerCondition := current.Status.GetCondition(v1alpha2.RouterReady)
				g.Expect(routerCondition).ToNot(BeNil(), "RouterReady condition should be set")
				g.Expect(routerCondition.IsTrue()).To(BeTrue(), "RouterReady condition should be True")

				// Check that HTTPRoutesReady condition exists and is True
				httpRoutesCondition := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
				g.Expect(httpRoutesCondition).ToNot(BeNil(), "HTTPRoutesReady condition should be set")
				g.Expect(httpRoutesCondition.IsTrue()).To(BeTrue(), "HTTPRoutesReady condition should be True")

				return nil
			}).WithContext(ctx).Should(Succeed(), "LLMInferenceService should become ready with custom gateway")
		})
	})

	Context("Reference validation", func() {
		When("referenced Gateway does not exist", func() {
			It("should mark RouterReady condition as False with InvalidRefs reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-gateway-ref-not-found"
				testNs := NewTestNamespace(ctx, envTest)

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("non-existent-route")),
					WithGatewayRefs(LLMGatewayRef("non-existent-gateway", testNs.Name)),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha2.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(testNs.Name + "/non-existent-gateway does not exist"))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("referenced parent Gateway from HTTPRoute does not exist", func() {
			It("should mark RouterReady condition as False with RefsInvalid reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-parent-gateway-ref-not-found"
				testNs := NewTestNamespace(ctx, envTest)

				// Create HTTPRoute spec that references a non-existent gateway
				customRouteSpec := &HTTPRoute("temp",
					WithParentRefs(GatewayParentRef("non-existent-parent-gateway", testNs.Name)),
					WithHTTPRule(
						WithBackendRefs(ServiceRef("some-backend", 8000, 1)),
					),
				).Spec

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteSpec(customRouteSpec),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha2.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(fmt.Sprintf("Managed HTTPRoute references non-existent Gateway %s/non-existent-parent-gateway", testNs.Name)))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("referenced HTTPRoute does not exist", func() {
			It("should mark RouterReady condition as False with RefsInvalid reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-route-ref-not-found"
				testNs := NewTestNamespace(ctx, envTest)

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("non-existent-route")),
					WithGatewayRefs(LLMGatewayRef(constants.GatewayName, constants.KServeNamespace)),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha2.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(fmt.Sprintf("HTTPRoute %s/non-existent-route does not exist", testNs.Name)))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})
	})

	Context("Stale condition cleanup", func() {
		When("Gateway is created after LLMInferenceService references it", func() {
			It("should clear stale GatewaysReady condition when Gateway is created", func(ctx SpecContext) {
				// given
				svcName := "test-llm-stale-gateway-condition"
				testNs := NewTestNamespace(ctx, envTest)

				// Create HTTPRoute first so HTTPRoute validation passes (focus on Gateway stale condition)
				httpRoute := HTTPRoute("my-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
				}...)
				Expect(envTest.Client.Create(ctx, httpRoute)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, httpRoute)
				}()

				// Create LLMInferenceService referencing a Gateway that doesn't exist yet
				// Must use WithHTTPRouteRefs (custom gateway requires custom routes, not managed routes)
				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("my-route")),
					WithGatewayRefs(LLMGatewayRef("my-gateway", testNs.Name)),
				)

				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// Verify GatewaysReady is False with RefsInvalid
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					gatewaysCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.GatewaysReady)
					g.Expect(gatewaysCondition).ToNot(BeNil())
					g.Expect(gatewaysCondition.IsFalse()).To(BeTrue())
					g.Expect(gatewaysCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(gatewaysCondition.Message).To(ContainSubstring("my-gateway does not exist"))

					return nil
				}).WithContext(ctx).Should(Succeed())

				// when - Create the Gateway
				gateway := Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway](testNs.Name),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				)
				Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())
				ensureGatewayReady(ctx, envTest.Client, gateway)
				defer func() {
					testNs.DeleteAndWait(ctx, gateway)
				}()

				// then - GatewaysReady stale condition should be cleared (no longer RefsInvalid)
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					gatewaysCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.GatewaysReady)
					// Condition should either be nil (cleared) or True (gateway is ready)
					if gatewaysCondition != nil {
						g.Expect(gatewaysCondition.Reason).ToNot(Equal(llmisvc.RefsInvalidReason),
							"Stale GatewaysReady condition with RefsInvalid should be cleared")
					}

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("HTTPRoute is created after LLMInferenceService references it", func() {
			It("should clear stale HTTPRoutesReady condition when HTTPRoute is created", func(ctx SpecContext) {
				// given
				svcName := "test-llm-stale-httproute-condition"
				testNs := NewTestNamespace(ctx, envTest)

				// Create the Gateway first so gateway validation passes
				gateway := Gateway("my-gateway",
					InNamespace[*gwapiv1.Gateway](testNs.Name),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
				)
				Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())
				ensureGatewayReady(ctx, envTest.Client, gateway)
				defer func() {
					testNs.DeleteAndWait(ctx, gateway)
				}()

				// Create LLMInferenceService referencing an HTTPRoute that doesn't exist yet
				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("my-route")),
					WithGatewayRefs(LLMGatewayRef("my-gateway", testNs.Name)),
				)

				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// Verify HTTPRoutesReady is False with RefsInvalid
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					httpRoutesCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.HTTPRoutesReady)
					g.Expect(httpRoutesCondition).ToNot(BeNil())
					g.Expect(httpRoutesCondition.IsFalse()).To(BeTrue())
					g.Expect(httpRoutesCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(httpRoutesCondition.Message).To(ContainSubstring("my-route does not exist"))

					// GatewaysReady should NOT have stale RefsInvalid (gateway exists now)
					gatewaysCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.GatewaysReady)
					if gatewaysCondition != nil {
						g.Expect(gatewaysCondition.Reason).ToNot(Equal(llmisvc.RefsInvalidReason),
							"GatewaysReady should not have stale RefsInvalid when gateway exists")
					}

					return nil
				}).WithContext(ctx).Should(Succeed())

				// when - Create the HTTPRoute
				httpRoute := HTTPRoute("my-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
					WithParentRef(GatewayParentRef("my-gateway", testNs.Name)),
					WithHTTPRouteRule(
						HTTPRouteRuleWithBackendAndTimeouts(svcName+"-kserve-workload-svc", 8000, "/", "0s", "0s"),
					),
				}...)
				Expect(envTest.Client.Create(ctx, httpRoute)).To(Succeed())
				ensureHTTPRouteReady(ctx, envTest.Client, httpRoute)
				defer func() {
					testNs.DeleteAndWait(ctx, httpRoute)
				}()

				// then - HTTPRoutesReady stale condition should be cleared
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					httpRoutesCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.HTTPRoutesReady)
					// Condition should either be nil (cleared) or True (route is ready)
					if httpRoutesCondition != nil {
						g.Expect(httpRoutesCondition.Reason).ToNot(Equal(llmisvc.RefsInvalidReason),
							"Stale HTTPRoutesReady condition with RefsInvalid should be cleared")
					}

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("scheduler configuration is removed", func() {
			It("should clear SchedulerWorkloadReady condition when scheduler is no longer configured", func(ctx SpecContext) {
				// given
				svcName := "test-llm-stale-scheduler-condition"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				// Create LLMInferenceService with scheduler
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

				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				// Verify SchedulerWorkloadReady condition exists
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					schedulerCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.SchedulerWorkloadReady)
					g.Expect(schedulerCondition).ToNot(BeNil(), "SchedulerWorkloadReady condition should exist when scheduler is configured")

					return nil
				}).WithContext(ctx).Should(Succeed())

				// when - Remove scheduler configuration
				errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
						llmSvc.Spec.Router.Scheduler = nil
						return nil
					})
					return errUpdate
				})
				Expect(errRetry).ToNot(HaveOccurred())

				// then - SchedulerWorkloadReady condition should be cleared
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					schedulerCondition := updatedLLMSvc.Status.GetCondition(v1alpha2.SchedulerWorkloadReady)
					g.Expect(schedulerCondition).To(BeNil(),
						"SchedulerWorkloadReady condition should be cleared when scheduler is not configured")

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})
	})
})

func LLMInferenceServiceIsReady(llmSvc *v1alpha2.LLMInferenceService, assertFns ...func(g Gomega, current *v1alpha2.LLMInferenceService)) func(g Gomega, ctx context.Context) error {
	return func(g Gomega, ctx context.Context) error {
		current := &v1alpha2.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
		g.Expect(current.Status).To(HaveCondition(string(v1alpha2.RouterReady), "True"))

		// Overall condition depends on owned resources such as Deployment.
		// When running on EnvTest certain controllers are not built-in, and that
		// includes deployment controllers, ReplicaSet controllers, etc.
		// Therefore, we can only observe a successful reconcile when testing against the actual cluster
		if envTest.UsingExistingCluster() {
			g.Expect(current.Status).To(HaveCondition("Ready", "True"))
		}

		for _, assertFn := range assertFns {
			assertFn(g, current)
		}

		return nil
	}
}

func managedRoutes(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]gwapiv1.HTTPRoute, error) {
	httpRoutes := &gwapiv1.HTTPRouteList{}
	listOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.RouterLabels(llmSvc)),
	}
	err := envTest.List(ctx, httpRoutes, listOpts)
	return httpRoutes.Items, ignoreNoMatch(err)
}

func ignoreNoMatch(err error) error {
	if meta.IsNoMatchError(err) {
		return nil
	}

	return err
}

// ensureGatewayReady sets up Gateway status conditions to simulate a ready Gateway
// Only runs in non-cluster mode
func ensureGatewayReady(ctx context.Context, c client.Client, gateway *gwapiv1.Gateway) {
	if envTest.UsingExistingCluster() {
		return
	}

	// Get the current gateway
	createdGateway := &gwapiv1.Gateway{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(gateway), createdGateway)).To(Succeed())

	// Set the status conditions to simulate the Gateway controller making it ready
	createdGateway.Status.Conditions = []metav1.Condition{
		{
			Type:               string(gwapiv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             "Accepted",
			Message:            "Gateway accepted",
			LastTransitionTime: metav1.Now(),
		},
		{
			Type:               string(gwapiv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            "Gateway is ready",
			LastTransitionTime: metav1.Now(),
		},
	}

	// Update the status
	Expect(c.Status().Update(ctx, createdGateway)).To(Succeed())

	// Verify the gateway is now ready
	Eventually(func(g Gomega, ctx context.Context) bool {
		updatedGateway := &gwapiv1.Gateway{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(gateway), updatedGateway)).To(Succeed())
		return llmisvc.IsGatewayReady(updatedGateway)
	}).WithContext(ctx).Should(BeTrue())
}

// ensureHTTPRouteReady sets up HTTPRoute status conditions to simulate a ready HTTPRoute
// Only runs in non-cluster mode
func ensureHTTPRouteReady(ctx context.Context, c client.Client, route *gwapiv1.HTTPRoute) {
	if envTest.UsingExistingCluster() {
		return
	}

	// Get the current HTTPRoute
	createdRoute := &gwapiv1.HTTPRoute{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(route), createdRoute)).To(Succeed())

	// Set the status conditions to simulate the Gateway controller making the HTTPRoute ready
	// HTTPRoute readiness is determined by parent status conditions
	if len(createdRoute.Spec.ParentRefs) > 0 {
		createdRoute.Status.RouteStatus.Parents = make([]gwapiv1.RouteParentStatus, len(createdRoute.Spec.ParentRefs))
		for i, parentRef := range createdRoute.Spec.ParentRefs {
			createdRoute.Status.RouteStatus.Parents[i] = gwapiv1.RouteParentStatus{
				ParentRef:      parentRef,
				ControllerName: "gateway.networking.k8s.io/gateway-controller",
				Conditions: []metav1.Condition{
					{
						Type:               string(gwapiv1.RouteConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "Accepted",
						Message:            "HTTPRoute accepted",
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               string(gwapiv1.RouteConditionResolvedRefs),
						Status:             metav1.ConditionTrue,
						Reason:             "ResolvedRefs",
						Message:            "HTTPRoute references resolved",
						LastTransitionTime: metav1.Now(),
					},
				},
			}
		}
	}

	// Update the status
	Expect(c.Status().Update(ctx, createdRoute)).To(Succeed())

	// Verify the HTTPRoute is now ready
	Eventually(func(g Gomega, ctx context.Context) bool {
		updatedRoute := &gwapiv1.HTTPRoute{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(route), updatedRoute)).To(Succeed())
		return llmisvc.IsHTTPRouteReady(updatedRoute)
	}).WithContext(ctx).Should(BeTrue())
}

// ensureInferencePoolReady sets up InferencePool status conditions to simulate a ready InferencePool
func ensureInferencePoolReady(ctx context.Context, c client.Client, pool *igwapi.InferencePool) {
	if envTest.UsingExistingCluster() {
		return
	}

	createdPool := &igwapi.InferencePool{}
	Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), createdPool)).To(Succeed())
	WithInferencePoolReadyStatus()(createdPool)
	Expect(c.Status().Update(ctx, createdPool)).To(Succeed())

	// Verify the InferencePool is now ready
	updatedPool := &igwapi.InferencePool{}
	Eventually(func(g Gomega, ctx context.Context) bool {
		updatedPool = &igwapi.InferencePool{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(pool), updatedPool)).To(Succeed())
		return llmisvc.IsInferencePoolReady(updatedPool)
	}).WithContext(ctx).Should(BeTrue(), fmt.Sprintf("Expected InferencePool to be ready, got: %#v", updatedPool.Status))
}

// Only runs in non-cluster mode
func ensureRouterManagedResourcesAreReady(ctx context.Context, c client.Client, llmSvc *v1alpha2.LLMInferenceService) {
	if envTest.UsingExistingCluster() {
		return
	}

	gomega.Eventually(func(g gomega.Gomega, ctx context.Context) {
		// Get managed gateways and make them ready
		gateways := &gwapiv1.GatewayList{}
		listOpts := &client.ListOptions{
			Namespace:     llmSvc.Namespace,
			LabelSelector: labels.SelectorFromSet(llmisvc.RouterLabels(llmSvc)),
		}
		err := c.List(ctx, gateways, listOpts)
		if err != nil && !errors.IsNotFound(err) {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}

		logf.FromContext(ctx).Info("Marking Gateway resources ready", "gateways", gateways)
		for _, gateway := range gateways.Items {
			// Update gateway status to ready
			updatedGateway := gateway.DeepCopy()
			WithGatewayReadyStatus()(updatedGateway)
			g.Expect(c.Status().Update(ctx, updatedGateway)).To(gomega.Succeed())
		}

		// Get managed HTTPRoutes and make them ready
		httpRoutes := &gwapiv1.HTTPRouteList{}
		err = c.List(ctx, httpRoutes, listOpts)
		if err != nil && !errors.IsNotFound(err) {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}

		logf.FromContext(ctx).Info("Marking HTTPRoute resources ready", "routes", httpRoutes)
		for _, route := range httpRoutes.Items {
			// Update HTTPRoute status to ready
			updatedRoute := route.DeepCopy()
			WithHTTPRouteReadyStatus(DefaultGatewayControllerName)(updatedRoute)
			g.Expect(c.Status().Update(ctx, updatedRoute)).To(gomega.Succeed())
		}

		// Ensure at least one HTTPRoute was found and made ready
		g.Expect(httpRoutes.Items).To(gomega.HaveLen(1), "Expected exactly one managed HTTPRoute")

		infPoolsListOpts := &client.ListOptions{
			Namespace:     llmSvc.Namespace,
			LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
		}

		infPools := &igwapi.InferencePoolList{}
		err = c.List(ctx, infPools, infPoolsListOpts)
		if err != nil && !errors.IsNotFound(err) {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}
		logf.FromContext(ctx).Info("Marking InferencePool resources ready", "inferencepools", infPools)
		for _, pool := range infPools.Items {
			updatedPool := pool.DeepCopy()
			WithInferencePoolReadyStatus()(updatedPool)
			g.Expect(c.Status().Update(ctx, updatedPool)).To(gomega.Succeed())
		}

		ensureSchedulerDeploymentReady(ctx, c, llmSvc)
	}).WithContext(ctx).Should(gomega.Succeed())
}

func ensureSchedulerDeploymentReady(ctx context.Context, c client.Client, llmSvc *v1alpha2.LLMInferenceService) {
	if envTest.UsingExistingCluster() {
		return
	}

	schedulerListOpts := &client.ListOptions{
		Namespace:     llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
	}
	deployments := &appsv1.DeploymentList{}
	err := c.List(ctx, deployments, schedulerListOpts)
	if err != nil && !errors.IsNotFound(err) {
		Expect(err).NotTo(gomega.HaveOccurred())
	}

	logf.FromContext(ctx).Info("Marking scheduler ready (if any)", "deployments", deployments)
	for _, d := range deployments.Items {
		dep := d.DeepCopy()
		dep.Status.Conditions = append(dep.Status.Conditions, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionTrue,
		})
		Expect(c.Status().Update(ctx, dep)).To(gomega.Succeed())
	}
}

func customRouteSpec(ctx context.Context, c client.Client, nsName, gatewayRefName, backendRefName string) *gwapiv1.HTTPRouteSpec {
	customGateway := Gateway(gatewayRefName,
		InNamespace[*gwapiv1.Gateway](nsName),
		WithClassName("istio"),
		WithListeners(gwapiv1.Listener{
			Name:     "http",
			Port:     9991,
			Protocol: gwapiv1.HTTPProtocolType,
			AllowedRoutes: &gwapiv1.AllowedRoutes{
				Namespaces: &gwapiv1.RouteNamespaces{
					From: ptr.To(gwapiv1.NamespacesFromAll),
				},
			},
		}),
		WithGatewayReadyStatus(),
	)

	Expect(c.Create(ctx, customGateway)).To(Succeed())
	Expect(c.Status().Update(ctx, customGateway)).To(Succeed())

	route := HTTPRoute("custom-route", []HTTPRouteOption{
		InNamespace[*gwapiv1.HTTPRoute](nsName),
		WithParentRef(GatewayParentRef(gatewayRefName, nsName)),
		WithHTTPRouteRule(
			HTTPRouteRuleWithBackendAndTimeouts(backendRefName, 8000, "/", "0s", "0s"),
		),
	}...)

	// Create the HTTPRoute so we can make it ready
	Expect(c.Create(ctx, route)).To(Succeed())

	// Ensure the HTTPRoute becomes ready
	ensureHTTPRouteReady(ctx, c, route)

	httpRouteSpec := &route.Spec

	return httpRouteSpec
}
