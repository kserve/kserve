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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Scheduler Config", func() {
	Context("Inline scheduler config", func() {
		It("should use inline scheduler config in the scheduler deployment", func(ctx SpecContext) {
			// given
			svcName := "test-llm-inline-scheduler-config"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(customSchedulerConfig),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment is created and uses the inline config
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			// Create scheduler template with existing --config-text
			modelConfig := LLMInferenceServiceConfig("model-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)

			routerConfig := LLMInferenceServiceConfig("router-with-custom-args",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
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
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-config"},
					corev1.LocalObjectReference{Name: "router-with-custom-args"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment preserves the existing --config-text from template
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
			configMap := SchedulerConfigMap("scheduler-config-custom", testNs.Name, configMapData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-custom", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses the config from ConfigMap
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
			configMap := SchedulerConfigMapWithKey("scheduler-config-custom-key", testNs.Name, "my-custom-key", configMapData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-custom-key", "my-custom-key"),
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
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
			configMap := SchedulerConfigMap("scheduler-config-update", testNs.Name, initialConfigData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-update", ""),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for initial deployment
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(configMap), updatedConfigMap); err != nil {
					return err
				}
				updatedConfigMap.Data["epp"] = updatedConfigData
				return envTest.Update(ctx, updatedConfigMap)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - verify the deployment is updated with new config
			updatedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedDeployment = &appsv1.Deployment{}
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)
			// No Config specified - should use default

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses default config (for non-prefill mode)
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
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
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses P/D config
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("config-scheduler-system", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses the config from system namespace
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("non-existent-configmap", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - the scheduler deployment should not be created and status should indicate error
			Consistently(func(g Gomega, ctx context.Context) error {
				deployment := &appsv1.Deployment{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).
				Within(2 * time.Second).
				WithPolling(300 * time.Millisecond).
				Should(HaveOccurred())
		})

		It("should report error when ConfigMap is missing the referenced key", func(ctx SpecContext) {
			// given
			svcName := "test-llm-missing-key"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			// Create ConfigMap without the expected key
			configMap := SchedulerConfigMapWithKey("scheduler-config-wrong-key", testNs.Name, "wrong-key", "some-config")
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigRef("scheduler-config-wrong-key", ""),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - the scheduler deployment should not be created
			Consistently(func(g Gomega, ctx context.Context) error {
				deployment := &appsv1.Deployment{}
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
			configMap := SchedulerConfigMap("scheduler-config-baseref", testNs.Name, schedulerConfigData)
			Expect(envTest.Client.Create(ctx, configMap)).To(Succeed())

			// Create LLMInferenceServiceConfig with scheduler config ref
			modelConfig := LLMInferenceServiceConfig("model-with-scheduler",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigSchedulerConfigRef("scheduler-config-baseref", ""),
			)
			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())

			routerConfig := LLMInferenceServiceConfig("router-managed-baseref",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigManagedRouter(),
			)
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-with-scheduler"},
					corev1.LocalObjectReference{Name: "router-managed-baseref"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses the config from baseRef
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
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
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigSchedulerConfigInline(inlineSchedulerConfig),
			)
			Expect(envTest.Client.Create(ctx, modelConfig)).To(Succeed())

			routerConfig := LLMInferenceServiceConfig("router-managed-inline",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigManagedRouter(),
			)
			Expect(envTest.Client.Create(ctx, routerConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "model-with-inline-scheduler"},
					corev1.LocalObjectReference{Name: "router-managed-inline"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment uses the inline config from baseRef
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Verify the deployment uses inline config inherited from baseRef
			configText, found := getSchedulerConfigText(expectedDeployment)
			Expect(found).To(BeTrue(), "Expected to find inline config from baseRef in scheduler deployment")
			Expect(configText).To(ContainSubstring("inline-baseref-plugin"))
			Expect(configText).To(ContainSubstring("inline-baseref-profile"))
		})
	})

	Context("Leader election flag injection", func() {
		It("should inject --ha-enable-leader-election when replicas > 1", func(ctx SpecContext) {
			// given
			svcName := "test-llm-ha-replicas-gt-1"
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

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerReplicas(2),
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

			Expect(hasLeaderElectionFlag(expectedDeployment)).To(BeTrue(),
				"Expected --ha-enable-leader-election flag when replicas > 1")
		})

		It("should NOT inject --ha-enable-leader-election when replicas = 1", func(ctx SpecContext) {
			// given
			svcName := "test-llm-ha-replicas-eq-1"
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

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerReplicas(1),
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

			Expect(hasLeaderElectionFlag(expectedDeployment)).To(BeFalse(),
				"Expected NO --ha-enable-leader-election flag when replicas = 1")
		})

		It("should NOT inject --ha-enable-leader-election when replicas is not specified", func(ctx SpecContext) {
			// given
			svcName := "test-llm-ha-replicas-nil"
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

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](nsName),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				// No scheduler replicas specified
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

			Expect(hasLeaderElectionFlag(expectedDeployment)).To(BeFalse(),
				"Expected NO --ha-enable-leader-election flag when replicas is not specified")
		})

		It("should NOT duplicate --ha-enable-leader-election when flag already exists", func(ctx SpecContext) {
			// given
			svcName := "test-llm-ha-flag-exists"
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

			// Create scheduler template with existing --ha-enable-leader-election
			modelConfig := LLMInferenceServiceConfig("model-ha-config",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
			)

			routerConfig := LLMInferenceServiceConfig("router-with-ha-flag",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](nsName),
			)
			routerConfig.Spec.Router = &v1alpha2.RouterSpec{
				Gateway: &v1alpha2.GatewaySpec{},
				Route:   &v1alpha2.GatewayRoutesSpec{},
				Scheduler: &v1alpha2.SchedulerSpec{
					Replicas: ptr.To[int32](3),
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "main",
								Image: "ghcr.io/llm-d/llm-d-inference-scheduler:v0.2.0",
								Args: []string{
									"--ha-enable-leader-election",
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
					corev1.LocalObjectReference{Name: "model-ha-config"},
					corev1.LocalObjectReference{Name: "router-with-ha-flag"},
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
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: nsName,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(countLeaderElectionFlags(expectedDeployment)).To(Equal(1),
				"Expected exactly one --ha-enable-leader-election flag (not duplicated)")
		})
	})

	Context("Certificate hash annotation", func() {
		It("should set cert-hash annotation on the scheduler pod template", func(ctx SpecContext) {
			// given
			svcName := "test-llm-scheduler-cert-hash"
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

			// then - verify the scheduler deployment has the cert-hash annotation
			Eventually(func(g Gomega, ctx context.Context) error {
				schedulerDeployment := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, schedulerDeployment)).To(Succeed())

				g.Expect(schedulerDeployment.Spec.Template.Annotations).To(
					HaveKey(llmisvc.DefaultRestartAnnotation),
					"Scheduler pod template should have cert-hash annotation to trigger restart on cert renewal",
				)
				g.Expect(schedulerDeployment.Spec.Template.Annotations[llmisvc.DefaultRestartAnnotation]).To(
					MatchRegexp("^[0-9a-f]{64}$"), "cert-hash should be a SHA-256 hex string",
				)

				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		DescribeTable("should skip cert-hash annotation when scheduler supports cert reload",
			func(ctx SpecContext, certReloadArg string) {
				// given
				svcName := "test-llm-cert-reload-skip"
				testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

				modelConfig := LLMInferenceServiceConfig("model-cert-reload",
					InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
					WithConfigModelName("facebook/opt-125m"),
					WithConfigModelURI("hf://facebook/opt-125m"),
				)

				routerConfig := LLMInferenceServiceConfig("router-cert-reload",
					InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				)
				routerConfig.Spec.Router = &v1alpha2.RouterSpec{
					Gateway: &v1alpha2.GatewaySpec{},
					Route:   &v1alpha2.GatewayRoutesSpec{},
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "main",
									Image: "ghcr.io/llm-d/llm-d-inference-scheduler:v0.6.0",
									Args: []string{
										certReloadArg,
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
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithBaseRefs(
						corev1.LocalObjectReference{Name: "model-cert-reload"},
						corev1.LocalObjectReference{Name: "router-cert-reload"},
					),
				)

				// when
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				defer func() {
					testNs.DeleteAndWait(ctx, llmSvc)
				}()

				// then - the scheduler deployment must NOT have cert-hash annotation
				schedulerDeployment := &appsv1.Deployment{}
				Eventually(func(g Gomega, ctx context.Context) error {
					return envTest.Get(ctx, types.NamespacedName{
						Name:      svcName + "-kserve-router-scheduler",
						Namespace: testNs.Name,
					}, schedulerDeployment)
				}).WithContext(ctx).Should(Succeed())

				Expect(schedulerDeployment.Spec.Template.Annotations).NotTo(
					HaveKey(llmisvc.DefaultRestartAnnotation),
					"Scheduler with cert reload enabled should not have cert-hash annotation",
				)
			},
			Entry("bare flag", "--enable-cert-reload"),
			Entry("flag with =true", "--enable-cert-reload=true"),
		)
	})

	Context("Scheduler RBAC", func() {
		It("should create scheduler role with leases permission for leader election", func(ctx SpecContext) {
			// given
			svcName := "test-llm-scheduler-rbac"
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

			// then - verify the scheduler role is created with leases permission
			expectedRole := &rbacv1.Role{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-role",
					Namespace: nsName,
				}, expectedRole)
			}).WithContext(ctx).Should(Succeed())

			// Verify leases permission exists
			hasLeasesPermission := false
			for _, rule := range expectedRole.Rules {
				for _, apiGroup := range rule.APIGroups {
					if apiGroup == "coordination.k8s.io" {
						for _, resource := range rule.Resources {
							if resource == "leases" {
								hasLeasesPermission = true
								// Verify all necessary verbs for leader election
								Expect(rule.Verbs).To(ContainElements("get", "list", "watch", "create", "update", "patch", "delete"))
							}
						}
					}
				}
			}
			Expect(hasLeasesPermission).To(BeTrue(), "Expected scheduler role to have leases permission for leader election")
		})
	})

	Context("UDS tokenizer config injection", func() {
		It("should inject tokenizersPoolConfig when precise-prefix-cache-scorer has indexerConfig", func(ctx SpecContext) {
			// given
			svcName := "test-llm-uds-tokenizer-inject"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			precisePrefixConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 16
        hashSeed: "42"
- type: queue-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(precisePrefixConfig),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the scheduler deployment has injected tokenizersPoolConfig
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
				g.Expect(configText).To(ContainSubstring("modelName: base"))
				g.Expect(configText).To(ContainSubstring("socketFile: /tmp/tokenizer/tokenizer-uds.socket"))
				// Verify tokenProcessorConfig was migrated from indexerConfig to top-level parameters
				g.Expect(configText).To(ContainSubstring("tokenProcessorConfig"))
				g.Expect(configText).To(ContainSubstring("blockSize: 16"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should override existing tokenizersPoolConfig values", func(ctx SpecContext) {
			// given
			svcName := "test-llm-uds-tokenizer-override"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			//nolint:gosec // G101: not a credential, scheduler config YAML
			configWithExistingTokenizer := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 16
        hashSeed: "42"
      tokenizersPoolConfig:
        modelName: "wrong-model-name"
        uds:
          socketFile: /wrong/path/tokenizer.socket
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(configWithExistingTokenizer),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the values are overridden with the correct ones
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
				g.Expect(configText).To(ContainSubstring("modelName: base"))
				g.Expect(configText).To(ContainSubstring("socketFile: /tmp/tokenizer/tokenizer-uds.socket"))
				g.Expect(configText).NotTo(ContainSubstring("wrong-model-name"))
				g.Expect(configText).NotTo(ContainSubstring("/wrong/path"))
				// Verify tokenProcessorConfig was migrated from indexerConfig to top-level parameters
				g.Expect(configText).To(ContainSubstring("tokenProcessorConfig"))
				g.Expect(configText).To(ContainSubstring("blockSize: 16"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should not inject tokenizersPoolConfig when no precise-prefix-cache-scorer plugin", func(ctx SpecContext) {
			// given
			svcName := "test-llm-uds-no-precise-prefix"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			configWithoutPrecisePrefix := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(configWithoutPrecisePrefix),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify no tokenizersPoolConfig is injected
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
				g.Expect(configText).NotTo(ContainSubstring("tokenizersPoolConfig"))
				g.Expect(configText).NotTo(ContainSubstring("modelName: base"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("tokenProcessorConfig migration from indexerConfig to top-level parameters", func() {
		It("should migrate tokenProcessorConfig from indexerConfig to top-level plugin parameters", func(ctx SpecContext) {
			// given
			svcName := "test-llm-tpc-migrate"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			oldFormatConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 64
        hashSeed: "42"
      kvBlockIndexConfig:
        enableMetrics: true
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(oldFormatConfig),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify tokenProcessorConfig was migrated to top-level parameters
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
				// tokenProcessorConfig should be at top-level parameters (sibling of indexerConfig)
				g.Expect(configText).To(ContainSubstring("blockSize: 64"))
				g.Expect(configText).To(ContainSubstring("hashSeed: \"42\""))
				// indexerConfig should still have kvBlockIndexConfig but not tokenProcessorConfig
				g.Expect(configText).To(ContainSubstring("kvBlockIndexConfig"))
				g.Expect(configText).To(ContainSubstring("enableMetrics: true"))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should not overwrite top-level tokenProcessorConfig if already present", func(ctx SpecContext) {
			// given
			svcName := "test-llm-tpc-no-overwrite"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			newFormatConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    tokenProcessorConfig:
      blockSize: 128
      hashSeed: "99"
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 64
        hashSeed: "42"
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(newFormatConfig),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify top-level tokenProcessorConfig is preserved, not overwritten
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-router-scheduler",
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected to find --config-text in scheduler deployment")
				// Top-level values should be preserved
				g.Expect(configText).To(ContainSubstring("blockSize: 128"))
				g.Expect(configText).To(ContainSubstring("hashSeed: \"99\""))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Scheduler SA credential propagation", func() {
		It("should propagate annotations and imagePullSecrets from main workload SA to generated scheduler SA", func(ctx SpecContext) {
			// given
			svcName := "test-llm-sa-propagation"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			// Create a main workload SA with annotations and imagePullSecrets
			mainSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "main-workload-sa",
					Namespace: testNs.Name,
					Annotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/my-model-role",
						"custom-annotation":          "custom-value",
					},
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "my-registry-secret"},
				},
			}
			Expect(envTest.Client.Create(ctx, mainSA)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithTemplate(&corev1.PodSpec{
					ServiceAccountName: "main-workload-sa",
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "quay.io/test/vllm:latest",
						},
					},
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the generated scheduler SA has propagated credentials
			schedulerSA := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: testNs.Name,
				}, schedulerSA); err != nil {
					return err
				}

				g.Expect(schedulerSA.Annotations).To(HaveKeyWithValue(
					"eks.amazonaws.com/role-arn", "arn:aws:iam::123456789012:role/my-model-role"))
				g.Expect(schedulerSA.Annotations).To(HaveKeyWithValue(
					"custom-annotation", "custom-value"))
				g.Expect(schedulerSA.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{Name: "my-registry-secret"}))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should not add extra credentials when no main workload SA is specified", func(ctx SpecContext) {
			// given
			svcName := "test-llm-sa-no-propagation"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				// No WithTemplate - so no ServiceAccountName set
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then - verify the generated scheduler SA has no extra credentials
			schedulerSA := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: testNs.Name,
				}, schedulerSA)
			}).WithContext(ctx).Should(Succeed())

			Expect(schedulerSA.ImagePullSecrets).To(BeEmpty())
			// Annotations should only contain system-managed ones (if any), not credential annotations
			Expect(schedulerSA.Annotations).ToNot(HaveKey("eks.amazonaws.com/role-arn"))
		})

		It("should update scheduler SA when main workload SA credentials change", func(ctx SpecContext) {
			// given
			svcName := "test-llm-sa-update"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			// Create a main workload SA with initial annotations
			mainSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "updatable-sa",
					Namespace: testNs.Name,
					Annotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/initial-role",
					},
				},
			}
			Expect(envTest.Client.Create(ctx, mainSA)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithTemplate(&corev1.PodSpec{
					ServiceAccountName: "updatable-sa",
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "quay.io/test/vllm:latest",
						},
					},
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for initial scheduler SA creation with initial annotation
			schedulerSA := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: testNs.Name,
				}, schedulerSA); err != nil {
					return err
				}

				g.Expect(schedulerSA.Annotations).To(HaveKeyWithValue(
					"eks.amazonaws.com/role-arn", "arn:aws:iam::123456789012:role/initial-role"))
				return nil
			}).WithContext(ctx).Should(Succeed())

			// when - update main SA annotations
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedSA := &corev1.ServiceAccount{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(mainSA), updatedSA); err != nil {
					return err
				}
				updatedSA.Annotations["eks.amazonaws.com/role-arn"] = "arn:aws:iam::123456789012:role/updated-role"
				updatedSA.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "new-secret"}}
				return envTest.Update(ctx, updatedSA)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Trigger re-reconciliation by updating the LLMInferenceService
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedSvc := &v1alpha2.LLMInferenceService{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), updatedSvc); err != nil {
					return err
				}
				if updatedSvc.Annotations == nil {
					updatedSvc.Annotations = make(map[string]string)
				}
				updatedSvc.Annotations["reconcile-trigger"] = "update-sa"
				return envTest.Update(ctx, updatedSvc)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// then - scheduler SA should be updated with new credentials
			Eventually(func(g Gomega, ctx context.Context) error {
				updatedSchedulerSA := &corev1.ServiceAccount{}
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-epp-sa",
					Namespace: testNs.Name,
				}, updatedSchedulerSA); err != nil {
					return err
				}

				g.Expect(updatedSchedulerSA.Annotations).To(HaveKeyWithValue(
					"eks.amazonaws.com/role-arn", "arn:aws:iam::123456789012:role/updated-role"))
				g.Expect(updatedSchedulerSA.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{Name: "new-secret"}))
				return nil
			}).WithContext(ctx).Should(Succeed())
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

// hasLeaderElectionFlag checks if the scheduler deployment has the --ha-enable-leader-election flag
func hasLeaderElectionFlag(deployment *appsv1.Deployment) bool {
	return countLeaderElectionFlags(deployment) > 0
}

// countLeaderElectionFlags counts how many leader election flags exist in the scheduler deployment
func countLeaderElectionFlags(deployment *appsv1.Deployment) int {
	count := 0
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == schedulerContainerName {
			for _, arg := range container.Args {
				if arg == "--ha-enable-leader-election" || arg == "-ha-enable-leader-election" {
					count++
				}
			}
		}
	}
	return count
}
