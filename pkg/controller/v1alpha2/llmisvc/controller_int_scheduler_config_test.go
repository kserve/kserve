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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Scheduler Config", func() {
	Context("Inline scheduler config", func() {
		It("should use inline scheduler config in the scheduler deployment", func(ctx SpecContext) {
			// given
			svcName := "test-llm-inline-scheduler-config"
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

			customSchedulerConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: custom-plugin
  parameters:
    customParam: customValue
schedulingProfiles:
- name: custom
  plugins:
  - pluginRef: custom-plugin
    weight: 5
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(customSchedulerConfig),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment is created and uses the inline config
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses the custom inline config
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find --config-text with inline config in scheduler deployment")
			Expect(configText).To(ContainSubstring("custom-plugin"))
			Expect(configText).To(ContainSubstring("customParam"))
		})

		It("should not override config when args already contain --config-text or --configFile", func(ctx SpecContext) {
			// given
			svcName := "test-llm-config-already-set"
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

			// Create scheduler template with existing --config-text
			modelConfig := LLMInferenceServiceConfig("model-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)

			routerConfig := LLMInferenceServiceConfig("router-with-custom-args",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
			)
			routerConfig.Spec.Router = &v1alpha2.RouterSpec{
				Gateway: &v1alpha2.GatewaySpec{},
				Route:   &v1alpha2.GatewayRoutesSpec{},
				Scheduler: &v1alpha2.SchedulerSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "main",
								Image: "ghcr.io/llm-d/llm-d-inference-scheduler:v0.2.0",
								Args: []string{
									"--config-text",
									"existing-config-from-template",
									"--poolName",
									"test-pool",
								},
								Ports: []corev1.ContainerPort{
									{Name: "grpc", ContainerPort: 9002, Protocol: corev1.ProtocolTCP},
									{Name: "grpc-health", ContainerPort: 9003, Protocol: corev1.ProtocolTCP},
									{Name: "metrics", ContainerPort: 9090, Protocol: corev1.ProtocolTCP},
								},
							},
						},
					},
					Pool: &v1alpha2.InferencePoolSpec{},
				},
			}

			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-config"},
					corev1.LocalObjectReference{Name: "router-with-custom-args"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment preserves the existing --config-text from template
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify that the existing --config-text is preserved (not overwritten or duplicated)
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
			Expect(configText).To(Equal("existing-config-from-template"))
			Expect(countConfigTextArgs(expectedDeployment)).To(Equal(1), "Expected exactly one --config-text argument")
		})
	})

	Context("ConfigMap ref scheduler config", func() {
		It("should resolve scheduler config from ConfigMap ref and use it in deployment", func(ctx SpecContext) {
			// given
			svcName := "test-llm-configmap-ref-scheduler"
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

			configMapData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: configmap-plugin
schedulingProfiles:
- name: configmap-profile
  plugins:
  - pluginRef: configmap-plugin
    weight: 10
`
			configMap := SchedulerConfigMap("scheduler-config-custom", nsName, configMapData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-custom", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses the config from ConfigMap
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses config from ConfigMap
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find --config-text with ConfigMap config in scheduler deployment")
			Expect(configText).To(ContainSubstring("configmap-plugin"))
			Expect(configText).To(ContainSubstring("configmap-profile"))
		})

		It("should use custom key from ConfigMap ref", func(ctx SpecContext) {
			// given
			svcName := "test-llm-configmap-custom-key"
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

			configMapData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: custom-key-plugin
schedulingProfiles:
- name: custom-key-profile
  plugins:
  - pluginRef: custom-key-plugin
`
			configMap := SchedulerConfigMapWithKey("scheduler-config-custom-key", nsName, "my-custom-key", configMapData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-custom-key", "my-custom-key"),
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
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses config from the custom key
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find --config-text with custom key config")
			Expect(configText).To(ContainSubstring("custom-key-plugin"))
		})
	})

	Context("ConfigMap update triggers reconciliation", func() {
		It("should update scheduler deployment when referenced ConfigMap is updated", func(ctx SpecContext) {
			// given
			svcName := "test-llm-configmap-update"
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

			initialConfigData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: initial-plugin
schedulingProfiles:
- name: initial-profile
  plugins:
  - pluginRef: initial-plugin
`
			configMap := SchedulerConfigMap("scheduler-config-update", nsName, initialConfigData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-update", ""),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Wait for initial deployment
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify initial config
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected initial config in scheduler deployment")
			Expect(configText).To(ContainSubstring("initial-plugin"))

			// when - update ConfigMap
			updatedConfigData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: updated-plugin
schedulingProfiles:
- name: updated-profile
  plugins:
  - pluginRef: updated-plugin
`
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedConfigMap := &corev1.ConfigMap{}
				if err := envTest.Client.Get(ctx, client.ObjectKeyFromObject(configMap), updatedConfigMap); err != nil {
					return err
				}
				updatedConfigMap.Data["epp"] = updatedConfigData
				return envTest.Client.Update(ctx, updatedConfigMap)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - verify the deployment is updated with new config
			updatedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedDeployment = &appsv1.Deployment{}
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, updatedDeployment); err != nil {
					return err
				}

				// Check for updated config using helper function
				configText, found := getSchedulerConfigText(updatedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in updated deployment")
				g.Expect(configText).To(ContainSubstring("updated-plugin"))
				return nil
			}).WithContext(ctx).Should(Succeed(), fmt.Sprintf("Expected to find updated scheduler config in updated deployment %#v", updatedDeployment))
		})
	})

	Context("Default scheduler config", func() {
		It("should use default scheduler config when no config is specified (non-prefill)", func(ctx SpecContext) {
			// given
			svcName := "test-llm-default-scheduler-config"
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
			// No Config specified - should use default

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses default config (for non-prefill mode)
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify default config for non-prefill mode (should contain queue-scorer, kv-cache-utilization-scorer, etc.)
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected default config in scheduler deployment")
			// Default non-prefill config should contain these plugins
			Expect(configText).To(ContainSubstring("queue-scorer"))
			Expect(configText).To(ContainSubstring("prefix-cache-scorer"))
			Expect(configText).To(ContainSubstring("max-score-picker"))
			Expect(configText).To(ContainSubstring("name: default"))
		})

		It("should use prefill/decode scheduler config when prefill is configured", func(ctx SpecContext) {
			// given
			svcName := "test-llm-prefill-scheduler-config"
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
				WithPrefill(&corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "quay.io/pierdipi/vllm-cpu:latest",
						},
					},
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses P/D config
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify P/D config (should contain prefill-filter, decode-filter, pd-profile-handler, etc.)
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected P/D config in scheduler deployment")
			// P/D config should contain these plugins
			Expect(configText).To(ContainSubstring("prefill-header-handler"))
			Expect(configText).To(ContainSubstring("prefill-filter"))
			Expect(configText).To(ContainSubstring("decode-filter"))
			Expect(configText).To(ContainSubstring("pd-profile-handler"))
			Expect(configText).To(ContainSubstring("name: prefill"))
			Expect(configText).To(ContainSubstring("name: decode"))
		})
	})

	Context("System namespace ConfigMap fallback", func() {
		It("should resolve ConfigMap from system namespace when not found in service namespace", func(ctx SpecContext) {
			// given
			svcName := "test-llm-system-ns-configmap"
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

			systemConfigData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: system-namespace-plugin
schedulingProfiles:
- name: system-profile
  plugins:
  - pluginRef: system-namespace-plugin
`
			systemConfigMap := SchedulerConfigMap("config-scheduler-system", constants.KServeNamespace, systemConfigData)
			Expect(envTest.Client.Create(ctx, systemConfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, systemConfigMap)).To(Succeed())
			}()

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("config-scheduler-system", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses the config from system namespace
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses config from system namespace
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find config from system namespace in scheduler deployment")
			Expect(configText).To(ContainSubstring("system-namespace-plugin"))
		})
	})

	Context("Scheduler config error handling", func() {
		It("should report error when referenced ConfigMap does not exist", func(ctx SpecContext) {
			// given
			svcName := "test-llm-missing-configmap"
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
				WithSchedulerConfigRef("non-existent-configmap", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - the scheduler deployment should not be created and status should indicate error
			Consistently(func(g Gomega, ctx context.Context) error {
				deployment := &appsv1.Deployment{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, deployment)
			}).WithContext(ctx).
				Within(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(HaveOccurred())
		})

		It("should report error when ConfigMap is missing the referenced key", func(ctx SpecContext) {
			// given
			svcName := "test-llm-missing-key"
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

			// Create ConfigMap without the expected key
			configMap := SchedulerConfigMapWithKey("scheduler-config-wrong-key", nsName, "wrong-key", "some-config")
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-wrong-key", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - the scheduler deployment should not be created
			Consistently(func(g Gomega, ctx context.Context) error {
				deployment := &appsv1.Deployment{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, deployment)
			}).WithContext(ctx).
				Within(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(HaveOccurred())
		})
	})

	Context("Scheduler config via baseRefs", func() {
		It("should inherit scheduler config from baseRef LLMInferenceServiceConfig", func(ctx SpecContext) {
			// given
			svcName := "test-llm-baseref-scheduler-config"
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

			schedulerConfigData := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: baseref-plugin
schedulingProfiles:
- name: baseref-profile
  plugins:
  - pluginRef: baseref-plugin
`
			configMap := SchedulerConfigMap("scheduler-config-baseref", nsName, schedulerConfigData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			// Create LLMInferenceServiceConfig with scheduler config ref
			modelConfig := LLMInferenceServiceConfig("model-with-scheduler",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigSchedulerConfigRef("scheduler-config-baseref", ""),
			)
			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())

			routerConfig := LLMInferenceServiceConfig("router-managed-baseref",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigManagedRouter(),
			)
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-with-scheduler"},
					corev1.LocalObjectReference{Name: "router-managed-baseref"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses the config from baseRef
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses config inherited from baseRef
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find config from baseRef in scheduler deployment")
			Expect(configText).To(ContainSubstring("baseref-plugin"))
		})

		It("should inherit inline scheduler config from baseRef LLMInferenceServiceConfig", func(ctx SpecContext) {
			// given
			svcName := "test-llm-baseref-inline-config"
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

			inlineSchedulerConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: inline-baseref-plugin
schedulingProfiles:
- name: inline-baseref-profile
  plugins:
  - pluginRef: inline-baseref-plugin
`

			// Create LLMInferenceServiceConfig with inline scheduler config
			modelConfig := LLMInferenceServiceConfig("model-with-inline-scheduler",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigSchedulerConfigInline(inlineSchedulerConfig),
			)
			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())

			routerConfig := LLMInferenceServiceConfig("router-managed-inline",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigManagedRouter(),
			)
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-with-inline-scheduler"},
					corev1.LocalObjectReference{Name: "router-managed-inline"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then - verify the scheduler deployment uses the inline config from baseRef
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses inline config inherited from baseRef
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find inline config from baseRef in scheduler deployment")
			Expect(configText).To(ContainSubstring("inline-baseref-plugin"))
			Expect(configText).To(ContainSubstring("inline-baseref-profile"))
		})
	})
})

// schedulerContainerName is the expected name of the main container in the scheduler deployment
const schedulerContainerName = "main"

// getSchedulerConfigText extracts the --config-text argument value from a scheduler deployment.
// Returns the config text and a boolean indicating whether it was found.
func getSchedulerConfigText(deployment *appsv1.Deployment) (configText string, found bool) {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == schedulerContainerName {
			for i, arg := range container.Args {
				if arg == "--config-text" && i+1 < len(container.Args) {
					return container.Args[i+1], true
				}
			}
		}
	}
	return "", false
}

// countConfigTextArgs counts how many --config-text arguments exist in the scheduler deployment.
// Used to verify that config is not duplicated.
func countConfigTextArgs(deployment *appsv1.Deployment) int {
	count := 0
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == schedulerContainerName {
			for _, arg := range container.Args {
				if arg == "--config-text" {
					count++
				}
			}
		}
	}
	return count
}
