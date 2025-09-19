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
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"
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

			modelConfig := LLMInferenceServiceConfig("model-fb-opt-125m",
				InNamespace[*v1alpha1.LLMInferenceServiceConfig](nsName),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)

			routerConfig := LLMInferenceServiceConfig("router-managed",
				InNamespace[*v1alpha1.LLMInferenceServiceConfig](nsName),
				WithConfigManagedRouter(),
			)

			workloadConfig := LLMInferenceServiceConfig("workload-single-cpu",
				InNamespace[*v1alpha1.LLMInferenceServiceConfig](nsName),
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
				InNamespace[*v1alpha1.LLMInferenceService](nsName),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-fb-opt-125m"},
					corev1.LocalObjectReference{Name: "router-managed"},
					corev1.LocalObjectReference{Name: "workload-single-cpu"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
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

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
			})).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Routing reconciliation ", func() {
		When("HTTP route is managed", func() {
			It("should create routes pointing to the default gateway when both are managed", func(ctx SpecContext) {
				// given
				svcName := "test-llm-create-http-route"
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
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
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
				Expect(expectedHTTPRoute).To(HaveBackendRefs(BackendRefInferencePool(svcName + "-inference-pool")))
				Expect(expectedHTTPRoute).To(Not(HaveBackendRefs(BackendRefService(svcName + "-kserve-workload-svc"))))

				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				Eventually(func(g Gomega, ctx context.Context) error {
					ip := igwapi.InferencePool{}
					return envTest.Client.Get(ctx, client.ObjectKey{Name: svcName + "-inference-pool", Namespace: llmSvc.GetNamespace()}, &ip)
				}).WithContext(ctx).Should(Succeed())

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.InferencePoolReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should use referenced external InferencePool", func(ctx SpecContext) {
				// given
				svcName := "test-llm-create-http-route-inf-pool-ref"
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

				infPoolName := kmeta.ChildName(svcName, "-my-inf-pool")

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
					WithInferencePoolRef(infPoolName),
				)

				infPool := InferencePool(infPoolName,
					InNamespace[*igwapi.InferencePool](nsName),
					WithSelector("app", "workload"),
					WithTargetPort(8000),
					WithExtensionRef("", "Service", kmeta.ChildName(svcName, "-epp-service")),
					WithInferencePoolReadyStatus(),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				Expect(envTest.Create(ctx, infPool)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
					Expect(envTest.Delete(ctx, infPool)).To(Succeed())
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
				Expect(expectedHTTPRoute).To(Not(HaveBackendRefs(BackendRefService(svcName + "-kserve-workload-svc"))))

				ensureInferencePoolReady(ctx, envTest.Client, infPool)
				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.InferencePoolReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should create routes pointing to workload service when no scheduler is configured", func(ctx SpecContext) {
				// given
				llmSvcName := "test-llm-create-http-route-no-scheduler"
				nsName := kmeta.ChildName(llmSvcName, "-test")
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: nsName,
					},
				}
				Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
				Expect(envTest.Client.Create(ctx, IstioShadowService(llmSvcName, nsName))).To(Succeed())
				defer func() {
					envTest.DeleteAll(namespace)
				}()

				llmSvc := LLMInferenceService(llmSvcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
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
					err := envTest.Client.Get(ctx, client.ObjectKey{Name: svcName, Namespace: nsName}, svc)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(svc.Spec.Selector).To(Equal(llmisvc.GetWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvc.Spec)))
					return nil
				})

				ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
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
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedGateway(),
					WithHTTPRouteSpec(customRouteSpec(ctx, envTest.Client, nsName, "my-ingress-gateway", "my-inference-service")),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
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

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should delete managed HTTPRoute when ref is defined", func(ctx SpecContext) {
				// given
				svcName := "test-llm-update-http-route"
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

				// Create the Gateway that the router-managed preset references
				gateway := Gateway("my-ingress-gateway",
					InNamespace[*gwapiv1.Gateway](nsName),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
					// Don't set the condition here initially
				)
				Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())

				// Ensure the gateway becomes ready
				ensureGatewayReady(ctx, envTest.Client, gateway)

				defer func() {
					Expect(envTest.Delete(ctx, gateway)).To(Succeed())
				}()

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(),
					WithManagedGateway(),
				)

				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
				}()

				Eventually(func(g Gomega, ctx context.Context) error {
					routes, errList := managedRoutes(ctx, llmSvc)
					g.Expect(errList).ToNot(HaveOccurred())
					g.Expect(routes).To(HaveLen(1))

					return nil
				}).WithContext(ctx).Should(Succeed())

				customHTTPRoute := HTTPRoute("my-custom-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](nsName),
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

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})

			It("should evaluate HTTPRoute readiness conditions", func(ctx SpecContext) {
				// given
				svcName := "test-llm-httproute-conditions"
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

				ingressGateway := DefaultGateway(nsName)
				Expect(envTest.Client.Create(ctx, ingressGateway)).To(Succeed())
				ensureGatewayReady(ctx, envTest.Client, ingressGateway)

				defer func() {
					Expect(envTest.Delete(ctx, ingressGateway)).To(Succeed())
				}()

				customHTTPRoute := HTTPRoute("my-custom-route", []HTTPRouteOption{
					InNamespace[*gwapiv1.HTTPRoute](nsName),
					WithParentRef(GatewayParentRef("kserve-ingress-gateway", nsName)),
					WithHTTPRouteRule(
						HTTPRouteRuleWithBackendAndTimeouts(svcName+"-inference-pool", 8000, "/", "0s", "0s"),
					),
				}...)
				Expect(envTest.Client.Create(ctx, customHTTPRoute)).To(Succeed())

				// Make the HTTPRoute ready
				ensureHTTPRouteReady(ctx, envTest.Client, customHTTPRoute)

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("my-custom-route")),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
				}()

				// then - verify HTTPRoutesReady condition is set
				Eventually(func(g Gomega, ctx context.Context) error {
					current := &v1alpha1.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

					// Check that HTTPRoutesReady condition exists and is True
					httpRoutesCondition := current.Status.GetCondition(v1alpha1.HTTPRoutesReady)
					g.Expect(httpRoutesCondition).ToNot(BeNil(), "HTTPRoutesReady condition should be set")
					g.Expect(httpRoutesCondition.IsTrue()).To(BeTrue(), "HTTPRoutesReady condition should be True")

					return nil
				}).WithContext(ctx).Should(Succeed(), "HTTPRoutesReady condition should be set to True")

				Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
					g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
				})).WithContext(ctx).Should(Succeed())
			})
		})

		When("transitioning from managed to unmanaged router", func() {
			DescribeTable("owned resources should be deleted",

				func(ctx SpecContext, testName string, initialRouterSpec *v1alpha1.RouterSpec, specMutation func(*v1alpha1.LLMInferenceService)) {
					// given
					svcName := "test-llm-" + testName
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
						InNamespace[*v1alpha1.LLMInferenceService](nsName),
						WithModelURI("hf://facebook/opt-125m"),
					)
					llmSvc.Spec.Router = initialRouterSpec

					// when - Create LLMInferenceService
					Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
					defer func() {
						Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
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

					Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha1.LLMInferenceService) {
						g.Expect(current.Status).To(HaveCondition(string(v1alpha1.HTTPRoutesReady), "True"))
					})).WithContext(ctx).Should(Succeed())
				},
				Entry("should delete HTTPRoutes when spec.Router is set to nil",
					"router-spec-nil",
					&v1alpha1.RouterSpec{
						Route: &v1alpha1.GatewayRoutesSpec{
							HTTP: &v1alpha1.HTTPRouteSpec{}, // Default empty spec
						},
						Gateway: &v1alpha1.GatewaySpec{},
					},
					func(llmSvc *v1alpha1.LLMInferenceService) {
						llmSvc.Spec.Router = nil
					},
				),
			)
		})
	})

	Context("Reference validation", func() {
		When("referenced Gateway does not exist", func() {
			It("should mark RouterReady condition as False with InvalidRefs reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-gateway-ref-not-found"
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

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("non-existent-route")),
					WithGatewayRefs(LLMGatewayRef("non-existent-gateway", nsName)),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha1.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha1.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha1.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(nsName + "/non-existent-gateway does not exist"))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("referenced parent Gateway from HTTPRoute does not exist", func() {
			It("should mark RouterReady condition as False with RefsInvalid reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-parent-gateway-ref-not-found"
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

				// Create HTTPRoute spec that references a non-existent gateway
				customRouteSpec := &HTTPRoute("temp",
					WithParentRefs(GatewayParentRef("non-existent-parent-gateway", nsName)),
					WithHTTPRule(
						WithBackendRefs(ServiceRef("some-backend", 8000, 1)),
					),
				).Spec

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteSpec(customRouteSpec),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha1.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha1.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha1.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(fmt.Sprintf("Managed HTTPRoute references non-existent Gateway %s/non-existent-parent-gateway", nsName)))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})

		When("referenced HTTPRoute does not exist", func() {
			It("should mark RouterReady condition as False with RefsInvalid reason", func(ctx SpecContext) {
				// given
				svcName := "test-llm-route-ref-not-found"
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

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha1.LLMInferenceService](nsName),
					WithModelURI("hf://facebook/opt-125m"),
					WithHTTPRouteRefs(HTTPRouteRef("non-existent-route")),
					WithGatewayRefs(LLMGatewayRef(constants.GatewayName, constants.KServeNamespace)),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
				}()

				// then
				Eventually(func(g Gomega, ctx context.Context) error {
					updatedLLMSvc := &v1alpha1.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedLLMSvc)).To(Succeed())

					g.Expect(updatedLLMSvc.Status).To(HaveCondition(string(v1alpha1.RouterReady), "False"))

					routerCondition := updatedLLMSvc.Status.GetCondition(v1alpha1.RouterReady)
					g.Expect(routerCondition).ToNot(BeNil())
					g.Expect(routerCondition.Reason).To(Equal(llmisvc.RefsInvalidReason))
					g.Expect(routerCondition.Message).To(ContainSubstring(fmt.Sprintf("HTTPRoute %s/non-existent-route does not exist", nsName)))

					return nil
				}).WithContext(ctx).Should(Succeed())
			})
		})
	})
})

func LLMInferenceServiceIsReady(llmSvc *v1alpha1.LLMInferenceService, assertFns ...func(g Gomega, current *v1alpha1.LLMInferenceService)) func(g Gomega, ctx context.Context) error {
	return func(g Gomega, ctx context.Context) error {
		current := &v1alpha1.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		g.Expect(current.Status).To(HaveCondition(string(v1alpha1.PresetsCombined), "True"))
		g.Expect(current.Status).To(HaveCondition(string(v1alpha1.RouterReady), "True"))

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

func managedRoutes(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) ([]gwapiv1.HTTPRoute, error) {
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
func ensureRouterManagedResourcesAreReady(ctx context.Context, c client.Client, llmSvc *v1alpha1.LLMInferenceService) {
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

func ensureSchedulerDeploymentReady(ctx context.Context, c client.Client, llmSvc *v1alpha1.LLMInferenceService) {
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
