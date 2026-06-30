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

package llmisvc

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestSchedulerConfigTextLoRA(t *testing.T) {
	loraAdapters := []v1alpha2.LLMModelSpec{{}}

	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		wantLoRA bool
	}{
		{
			name:     "no LoRA - standard default config",
			llmSvc:   &v1alpha2.LLMInferenceService{},
			wantLoRA: false,
		},
		{
			name: "LoRA nil pointer - no scorer",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{LoRA: nil},
				},
			},
			wantLoRA: false,
		},
		{
			name: "LoRA spec with empty adapters - no scorer",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{LoRA: &v1alpha2.LoRASpec{}},
				},
			},
			wantLoRA: false,
		},
		{
			name: "LoRA adapters present - scorer included",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						LoRA: &v1alpha2.LoRASpec{Adapters: loraAdapters},
					},
				},
			},
			wantLoRA: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			text := schedulerConfigText(tt.llmSvc)

			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(text), &obj)).To(Succeed())

			plugins := obj["plugins"].([]interface{})
			types := make([]string, 0, len(plugins))
			for _, p := range plugins {
				types = append(types, p.(map[string]interface{})["type"].(string))
			}

			profiles := obj["schedulingProfiles"].([]interface{})
			defaultProfile := profiles[0].(map[string]interface{})
			refs := defaultProfile["plugins"].([]interface{})
			refNames := make([]string, 0, len(refs))
			for _, r := range refs {
				refNames = append(refNames, r.(map[string]interface{})["pluginRef"].(string))
			}

			if tt.wantLoRA {
				g.Expect(types).To(ContainElement(loraAffinityScorerPlugin))
				g.Expect(refNames[0]).To(Equal(loraAffinityScorerPlugin))
				g.Expect(refs[0].(map[string]interface{})["weight"]).To(BeNumerically("==", 4))
			} else {
				g.Expect(types).NotTo(ContainElement(loraAffinityScorerPlugin))
				g.Expect(refNames).NotTo(ContainElement(loraAffinityScorerPlugin))
			}
		})
	}
}

func TestSchedulerConfigTextPDLoRA(t *testing.T) {
	loraAdapters := []v1alpha2.LLMModelSpec{{}}

	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		wantLoRA bool
	}{
		{
			name: "P/D without LoRA - no scorer",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			},
			wantLoRA: false,
		},
		{
			name: "P/D with LoRA adapters - scorer in both profiles",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Prefill: &v1alpha2.WorkloadSpec{},
					Model: v1alpha2.LLMModelSpec{
						LoRA: &v1alpha2.LoRASpec{Adapters: loraAdapters},
					},
				},
			},
			wantLoRA: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			text := schedulerConfigText(tt.llmSvc)

			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(text), &obj)).To(Succeed())

			plugins := obj["plugins"].([]interface{})
			types := make([]string, 0, len(plugins))
			for _, p := range plugins {
				types = append(types, p.(map[string]interface{})["type"].(string))
			}

			profiles := obj["schedulingProfiles"].([]interface{})
			g.Expect(profiles).To(HaveLen(2))

			for _, profile := range profiles {
				refs := profile.(map[string]interface{})["plugins"].([]interface{})
				refNames := make([]string, 0, len(refs))
				for _, r := range refs {
					refNames = append(refNames, r.(map[string]interface{})["pluginRef"].(string))
				}

				if tt.wantLoRA {
					g.Expect(types).To(ContainElement(loraAffinityScorerPlugin))
					// scorer is second (after the profile's filter plugin)
					g.Expect(refNames[1]).To(Equal(loraAffinityScorerPlugin))
					g.Expect(refs[1].(map[string]interface{})["weight"]).To(BeNumerically("==", 4))
				} else {
					g.Expect(refNames).NotTo(ContainElement(loraAffinityScorerPlugin))
				}
			}

			if !tt.wantLoRA {
				g.Expect(types).NotTo(ContainElement(loraAffinityScorerPlugin))
			}
		})
	}
}

func TestPreserveSchedulerConfig(t *testing.T) {
	defaultSvc := &v1alpha2.LLMInferenceService{}
	inlineSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Config: &v1alpha2.SchedulerConfigSpec{
						Inline: &runtime.RawExtension{Raw: []byte("updated-inline-config")},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		curr     *appsv1.Deployment
		expected []string
	}{
		{
			name:     "no current deployment - generates fresh config",
			llmSvc:   defaultSvc,
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
		{
			name:   "current deployment with --config-text - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text", "existing-config-yaml"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", "existing-config-yaml"},
		},
		{
			name:   "current deployment with -config-text - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"-config-text", "old-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"-config-text", "old-config"},
		},
		{
			name:   "current deployment with --config-file - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-file", "/etc/scheduler/config.yaml"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-file", "/etc/scheduler/config.yaml"},
		},
		{
			name:   "current deployment with non-main container - ignored",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "sidecar",
									Args: []string{"--config-text", "sidecar-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
		{
			name:   "inline config overrides existing deployment config",
			llmSvc: inlineSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text", "stale-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", "updated-inline-config"},
		},
		{
			name:     "inline config used when no existing deployment",
			llmSvc:   inlineSvc,
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", "updated-inline-config"},
		},
		{
			name: "template already has --config-text - returns nil to avoid duplication",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Template: &corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "main",
										Args: []string{"--config-text", "template-config", "--poolName", "test"},
									},
								},
							},
						},
					},
				},
			},
			curr:     &appsv1.Deployment{},
			expected: nil,
		},
		{
			name: "inline config overrides template config args",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Config: &v1alpha2.SchedulerConfigSpec{
								Inline: &runtime.RawExtension{Raw: []byte("inline-override")},
							},
							Template: &corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "main",
										Args: []string{"--config-text", "template-config"},
									},
								},
							},
						},
					},
				},
			},
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", "inline-override"},
		},
		{
			name:   "config flag as last arg without value - generates fresh config",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := preserveSchedulerConfig(tt.llmSvc, tt.curr)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestFilterArgs(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		names             map[string]bool
		expectedFiltered  []string
		expectedExtracted map[string]string
	}{
		{
			name:              "no matching args",
			args:              []string{"--poolName", "test-pool", "--grpc-port", "9002"},
			names:             map[string]bool{"kv-cache-usage-percentage-metric": true},
			expectedFiltered:  []string{"--poolName", "test-pool", "--grpc-port", "9002"},
			expectedExtracted: map[string]string{},
		},
		{
			name:              "remove flag with separate value",
			args:              []string{"--poolName", "test-pool", "--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc", "--grpc-port", "9002"},
			names:             map[string]bool{"kv-cache-usage-percentage-metric": true},
			expectedFiltered:  []string{"--poolName", "test-pool", "--grpc-port", "9002"},
			expectedExtracted: map[string]string{"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc"},
		},
		{
			name:              "remove flag with equals value",
			args:              []string{"--poolName", "test-pool", "--kv-cache-usage-percentage-metric=vllm:kv_cache_usage_perc", "--grpc-port", "9002"},
			names:             map[string]bool{"kv-cache-usage-percentage-metric": true},
			expectedFiltered:  []string{"--poolName", "test-pool", "--grpc-port", "9002"},
			expectedExtracted: map[string]string{"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc"},
		},
		{
			name: "remove multiple flags",
			args: []string{
				"--poolName", "test-pool",
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--total-running-requests-metric", "vllm:num_requests_running",
				"--grpc-port", "9002",
			},
			names: map[string]bool{
				"total-queued-requests-metric":  true,
				"total-running-requests-metric": true,
			},
			expectedFiltered: []string{"--poolName", "test-pool", "--grpc-port", "9002"},
			expectedExtracted: map[string]string{
				"total-queued-requests-metric":  "vllm:num_requests_waiting",
				"total-running-requests-metric": "vllm:num_requests_running",
			},
		},
		{
			name:              "flag at end with no value",
			args:              []string{"--poolName", "test-pool", "--lora-info-metric"},
			names:             map[string]bool{"lora-info-metric": true},
			expectedFiltered:  []string{"--poolName", "test-pool"},
			expectedExtracted: map[string]string{"lora-info-metric": ""},
		},
		{
			name:              "flag followed by another flag",
			args:              []string{"--lora-info-metric", "--grpc-port", "9002"},
			names:             map[string]bool{"lora-info-metric": true},
			expectedFiltered:  []string{"--grpc-port", "9002"},
			expectedExtracted: map[string]string{"lora-info-metric": ""},
		},
		{
			name:              "empty args",
			args:              []string{},
			names:             map[string]bool{"kv-cache-usage-percentage-metric": true},
			expectedFiltered:  nil,
			expectedExtracted: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			filtered, extracted := filterArgs(tt.args, tt.names)
			g.Expect(filtered).To(Equal(tt.expectedFiltered))
			g.Expect(extracted).To(Equal(tt.expectedExtracted))
		})
	}
}

func TestWithRenamePlugin(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		oldType    string
		newType    string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "renames matching plugin type",
			configYAML: `
plugins:
- type: prefill-header-handler
- type: queue-scorer
`,
			oldType: "prefill-header-handler",
			newType: "disagg-headers-handler",
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("disagg-headers-handler"))
				g.Expect(plugins[1].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
			},
		},
		{
			name: "no match - no change",
			configYAML: `
plugins:
- type: queue-scorer
`,
			oldType: "prefill-header-handler",
			newType: "disagg-headers-handler",
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
			},
		},
		{
			name: "already renamed - idempotent",
			configYAML: `
plugins:
- type: disagg-headers-handler
`,
			oldType: "prefill-header-handler",
			newType: "disagg-headers-handler",
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(1))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("disagg-headers-handler"))
			},
		},
		{
			name: "renames pluginRef in schedulingProfiles",
			configYAML: `
plugins:
- type: pd-profile-handler
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: pd-profile-handler
  - pluginRef: queue-scorer
`,
			oldType: "pd-profile-handler",
			newType: "disagg-profile-handler",
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("disagg-profile-handler"))

				profiles := obj["schedulingProfiles"].([]interface{})
				profilePlugins := profiles[0].(map[string]interface{})["plugins"].([]interface{})
				g.Expect(profilePlugins[0].(map[string]interface{})["pluginRef"]).To(Equal("disagg-profile-handler"))
				g.Expect(profilePlugins[1].(map[string]interface{})["pluginRef"]).To(Equal("queue-scorer"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			fn := WithRenamePlugin(tt.oldType, tt.newType)
			g.Expect(fn(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
		})
	}
}

// validateDeciderOrder checks the GIE loader ordering invariant: every plugin
// referenced in a handler's "deciders" map must appear earlier in the plugins
// list. The GIE loader registers plugins in list order, so a handler that
// references a decider declared later will fail with "plugin not found".
func validateDeciderOrder(g Gomega, obj map[string]interface{}) {
	val, ok := obj["plugins"]
	if !ok {
		return
	}
	plugins, ok := val.([]interface{})
	if !ok {
		return
	}

	typeIndex := map[string]int{}
	for i, p := range plugins {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := pm["type"].(string); ok {
			typeIndex[t] = i
		}
	}

	for i, p := range plugins {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		pluginType, _ := pm["type"].(string)
		params, ok := pm["parameters"].(map[string]interface{})
		if !ok {
			continue
		}
		deciders, ok := params["deciders"].(map[string]interface{})
		if !ok {
			continue
		}
		for role, ref := range deciders {
			refName, ok := ref.(string)
			if !ok {
				continue
			}
			refIdx, exists := typeIndex[refName]
			if !exists {
				// Decider not in the plugins list — may be externally
				// declared (e.g. Path A where the user manages it).
				continue
			}
			g.Expect(refIdx).To(BeNumerically("<", i),
				fmt.Sprintf("%s at index %d references decider %q (role %s) at index %d — decider must appear before handler",
					pluginType, i, refName, role, refIdx))
		}
	}
}

// validateDeciderOrderFromYAML is a convenience wrapper that unmarshals a
// config-text YAML string and then runs validateDeciderOrder on the result.
func validateDeciderOrderFromYAML(g Gomega, configText string) {
	var obj map[string]interface{}
	g.Expect(yaml.Unmarshal([]byte(configText), &obj)).To(Succeed())
	validateDeciderOrder(g, obj)
}

func TestWithMigrateDisaggProfileParams(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "migrates deciderPluginName to deciders map",
			configYAML: `
plugins:
- type: disagg-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("deciderPluginName"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "already has deciders map - idempotent",
			configYAML: `
plugins:
- type: disagg-profile-handler
  parameters:
    deciders:
      prefill: always-disagg-pd-decider
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "no matching plugin - no-op",
			configYAML: `
plugins:
- type: queue-scorer
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(1))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
			},
		},
		{
			name: "works with old pd-profile-handler type name",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("deciderPluginName"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "migrates threshold 0 to always-disagg-pd-decider",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: prefill-filter
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(3))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "migrates threshold 0 when decider plugin already exists",
			configYAML: `
plugins:
- type: always-disagg-pd-decider
- type: pd-profile-handler
  parameters:
    threshold: 0
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(2))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "strips threshold when deciderPluginName also present",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    deciderPluginName: prefix-based-pd-decider
    threshold: 0
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("deciderPluginName"))
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "migrates non-zero threshold 100 to prefix-based-pd-decider",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 100
- type: prefill-filter
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(3))
				deciderPlugin := plugins[0].(map[string]interface{})
				g.Expect(deciderPlugin["type"]).To(Equal("prefix-based-pd-decider"))
				deciderParams := deciderPlugin["parameters"].(map[string]interface{})
				g.Expect(deciderParams["nonCachedTokens"]).To(Equal(int64(25)))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "migrates non-zero threshold 5 to prefix-based-pd-decider with ceil",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 5
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				deciderPlugin := plugins[0].(map[string]interface{})
				g.Expect(deciderPlugin["type"]).To(Equal("prefix-based-pd-decider"))
				deciderParams := deciderPlugin["parameters"].(map[string]interface{})
				g.Expect(deciderParams["nonCachedTokens"]).To(Equal(int64(2)))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "migrates non-zero threshold 1 to prefix-based-pd-decider minimum 1 token",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 1
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				deciderPlugin := plugins[0].(map[string]interface{})
				g.Expect(deciderPlugin["type"]).To(Equal("prefix-based-pd-decider"))
				deciderParams := deciderPlugin["parameters"].(map[string]interface{})
				g.Expect(deciderParams["nonCachedTokens"]).To(Equal(int64(1)))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "non-zero threshold idempotent when prefix-based-pd-decider already exists",
			configYAML: `
plugins:
- type: prefix-based-pd-decider
  parameters:
    nonCachedTokens: 50
- type: pd-profile-handler
  parameters:
    threshold: 100
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(2))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("prefix-based-pd-decider"))
				params := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "decider inserted before handler not after - load order matters",
			configYAML: `
plugins:
- type: queue-scorer
- type: pd-profile-handler
  parameters:
    threshold: 100
- type: prefill-filter
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(4))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
				deciderPlugin := plugins[1].(map[string]interface{})
				g.Expect(deciderPlugin["type"]).To(Equal("prefix-based-pd-decider"))
				handlerPlugin := plugins[2].(map[string]interface{})
				g.Expect(handlerPlugin["type"]).To(Equal("pd-profile-handler"))
				g.Expect(plugins[3].(map[string]interface{})["type"]).To(Equal("prefill-filter"))
			},
		},
		{
			name: "always-disagg decider inserted before handler not after",
			configYAML: `
plugins:
- type: queue-scorer
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: prefill-filter
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(4))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
				g.Expect(plugins[1].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
				handlerPlugin := plugins[2].(map[string]interface{})
				g.Expect(handlerPlugin["type"]).To(Equal("pd-profile-handler"))
				g.Expect(plugins[3].(map[string]interface{})["type"]).To(Equal("prefill-filter"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(WithMigrateDisaggProfileParams(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
			validateDeciderOrder(g, u.Object)
		})
	}
}

func TestThresholdToNonCachedTokens(t *testing.T) {
	tests := []struct {
		name     string
		val      interface{}
		expected int64
	}{
		{name: "int64 100", val: int64(100), expected: 25},
		{name: "int64 5", val: int64(5), expected: 2},
		{name: "int64 1", val: int64(1), expected: 1},
		{name: "int64 3", val: int64(3), expected: 1},
		{name: "float64 100.0", val: float64(100.0), expected: 25},
		{name: "float64 0.5", val: float64(0.5), expected: 1},
		{name: "float64 7.0", val: float64(7.0), expected: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(thresholdToNonCachedTokens(tt.val)).To(Equal(tt.expected))
		})
	}
}

func TestWithMigrateDisaggProfileHandlerThreshold(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "migrates non-zero threshold with rename and prefix-based-pd-decider",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0.5
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(2))
				deciderPlugin := plugins[0].(map[string]interface{})
				g.Expect(deciderPlugin["type"]).To(Equal("prefix-based-pd-decider"))
				deciderParams := deciderPlugin["parameters"].(map[string]interface{})
				g.Expect(deciderParams["nonCachedTokens"]).To(Equal(int64(1)))
				pluginMap := plugins[1].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
		{
			name: "handles threshold 0 without deciderPluginName",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: prefill-filter
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(3))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
				pluginMap := plugins[1].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "handles threshold 0 when decider plugin already present",
			configYAML: `
plugins:
- type: always-disagg-pd-decider
- type: pd-profile-handler
  parameters:
    threshold: 0
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(2))
				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
				pluginMap := plugins[1].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
			},
		},
		{
			name: "migrates non-zero threshold with deciderPluginName - uses specified decider",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    deciderPluginName: prefix-based-pd-decider
    threshold: 0.5
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				pluginMap := plugins[0].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("deciderPluginName"))
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "prefix-based-pd-decider"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(withMigrateDisaggProfileHandler(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
			validateDeciderOrder(g, u.Object)
		})
	}
}

func TestWithRemoveHashBlockSize(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "removes hashBlockSize",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("hashBlockSize"))
				g.Expect(params).To(HaveKey("blockSizeTokens"))
			},
		},
		{
			name: "no hashBlockSize - no-op",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    blockSizeTokens: 16
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).To(HaveKey("blockSizeTokens"))
			},
		},
		{
			name: "removes from multiple plugins",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
- type: precise-prefix-cache-scorer
  parameters:
    hashBlockSize: 32
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				for _, p := range plugins {
					params := p.(map[string]interface{})["parameters"].(map[string]interface{})
					g.Expect(params).NotTo(HaveKey("hashBlockSize"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(WithRemoveHashBlockSize(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
		})
	}
}

func TestWithRemovePrefixCacheScorerParametersV09(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "removes all parameters except prefixMatchInfoProducerName",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    blockSizeTokens: 16
    hashBlockSize: 64
    prefixMatchInfoProducerName: my-producer
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).To(HaveKey("prefixMatchInfoProducerName"))
				g.Expect(params).NotTo(HaveKey("blockSizeTokens"))
				g.Expect(params).NotTo(HaveKey("hashBlockSize"))
			},
		},
		{
			name: "removes parameters entirely when no prefixMatchInfoProducerName",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    blockSizeTokens: 16
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})).NotTo(HaveKey("parameters"))
			},
		},
		{
			name: "no parameters - no-op",
			configYAML: `
plugins:
- type: prefix-cache-scorer
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})).NotTo(HaveKey("parameters"))
			},
		},
		{
			name: "does not affect other plugins",
			configYAML: `
plugins:
- type: prefix-cache-scorer
  parameters:
    blockSizeTokens: 16
- type: queue-scorer
  parameters:
    someParam: value
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins[0].(map[string]interface{})).NotTo(HaveKey("parameters"))
				queueParams := plugins[1].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(queueParams).To(HaveKeyWithValue("someParam", "value"))
			},
		},
		{
			name: "no plugins - no-op",
			configYAML: `
schedulingProfiles:
- name: default
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				g.Expect(obj).NotTo(HaveKey("plugins"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(withRemovePrefixCacheScorerParametersV09(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
		})
	}
}

func TestWithCoreMetricsExtractorPlugin(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		extracted  map[string]string
		validate   func(g Gomega, obj map[string]interface{})
	}{
		{
			name: "injects plugin with extracted values",
			configYAML: `
plugins:
- type: queue-scorer
`,
			extracted: map[string]string{
				"total-queued-requests-metric":     "vllm:num_requests_waiting",
				"total-running-requests-metric":    "vllm:num_requests_running",
				"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc",
				"lora-info-metric":                 "vllm:lora_requests_info",
				"cache-info-metric":                "vllm:cache_config_info",
			},
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(2))
				pluginMap := plugins[1].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal(coreMetricsExtractorPlugin))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params["defaultEngine"]).To(Equal("vllm"))
				engineConfigs := params["engineConfigs"].([]interface{})
				engine := engineConfigs[0].(map[string]interface{})
				g.Expect(engine["queuedRequestsSpec"]).To(Equal("vllm:num_requests_waiting"))
				g.Expect(engine["runningRequestsSpec"]).To(Equal("vllm:num_requests_running"))
				g.Expect(engine["kvUsageSpec"]).To(Equal("vllm:kv_cache_usage_perc"))
			},
		},
		{
			name: "skips when plugin already exists",
			configYAML: `
plugins:
- type: model-server-protocol-metrics
  parameters:
    defaultEngine: vllm
`,
			extracted: map[string]string{
				"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc",
			},
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(1))
			},
		},
		{
			name: "skips when renamed plugin already exists",
			configYAML: `
plugins:
- type: core-metrics-extractor
  parameters:
    defaultEngine: vllm
`,
			extracted: map[string]string{
				"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc",
			},
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(1))
			},
		},
		{
			name: "skips when no values extracted",
			configYAML: `
plugins:
- type: queue-scorer
`,
			extracted: map[string]string{},
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				g.Expect(plugins).To(HaveLen(1))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			fn := withCoreMetricsExtractorPlugin(tt.extracted)
			g.Expect(fn(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
		})
	}
}

func TestSchedulerTransform(t *testing.T) {
	oldConfigYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: pd-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
`

	tests := []struct {
		name           string
		version        string
		validateConfig func(g Gomega, configText string)
	}{
		{
			name:    "applies all migrations for v0.7.0",
			version: "0.7.0",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).To(ContainSubstring("prefill: always-disagg-pd-decider"))
				g.Expect(configText).NotTo(ContainSubstring("deciderPluginName"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
				g.Expect(configText).To(ContainSubstring("blockSizeTokens"))
			},
		},
		{
			name:    "skips all migrations for v0.6.0",
			version: "0.6.0",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).NotTo(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).To(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).To(ContainSubstring("deciderPluginName"))
				g.Expect(configText).To(ContainSubstring("hashBlockSize"))
			},
		},
		{
			name:    "skips all migrations when no version annotation",
			version: "",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).To(ContainSubstring("deciderPluginName"))
				g.Expect(configText).To(ContainSubstring("hashBlockSize"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			d := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"app.kubernetes.io/version": tt.version,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Args: []string{"--config-text", oldConfigYAML}},
							},
						},
					},
				},
			}

			g.Expect(schedulerTransform(context.Background(), d)).To(Succeed())

			configText := d.Spec.Template.Spec.Containers[0].Args[1]
			tt.validateConfig(g, configText)
		})
	}
}

func TestSchedulerTransformThreshold(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		version        string
		validateConfig func(g Gomega, configText string)
	}{
		{
			name: "migrates non-zero threshold in full transform",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: pd-profile-handler
  parameters:
    threshold: 0.5
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
`,
			version: "0.7.0",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).To(ContainSubstring("prefix-based-pd-decider"))
				g.Expect(configText).To(ContainSubstring("nonCachedTokens"))
				g.Expect(configText).NotTo(ContainSubstring("threshold"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
				g.Expect(configText).To(ContainSubstring("blockSizeTokens"))
			},
		},
		{
			name: "migrates non-zero threshold with deciderPluginName in full transform",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: pd-profile-handler
  parameters:
    deciderPluginName: prefix-based-pd-decider
    threshold: 0.5
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
`,
			version: "0.7.0",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("deciderPluginName"))
				g.Expect(configText).NotTo(ContainSubstring("threshold"))
				g.Expect(configText).To(ContainSubstring("prefill: prefix-based-pd-decider"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
			},
		},
		{
			name: "migrates threshold 0 in full transform",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
`,
			version: "0.7.0",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).To(ContainSubstring("prefill: always-disagg-pd-decider"))
				g.Expect(configText).NotTo(ContainSubstring("threshold"))
				g.Expect(configText).To(ContainSubstring("always-disagg-pd-decider"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			d := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"app.kubernetes.io/version": tt.version,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Args: []string{"--config-text", tt.configYAML}},
							},
						},
					},
				},
			}

			g.Expect(schedulerTransform(context.Background(), d)).To(Succeed())

			configText := d.Spec.Template.Spec.Containers[0].Args[1]
			tt.validateConfig(g, configText)
			validateDeciderOrderFromYAML(g, configText)
		})
	}
}

func TestFullMigrationPipeline(t *testing.T) {
	// Realistic old v0.6 config with all deprecated features.
	oldConfigYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: prefill-filter
- type: decode-filter
- type: queue-scorer
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
    indexerConfig:
      tokenProcessorConfig:
        batchSize: 1024
- type: max-score-picker
- type: always-disagg-pd-decider
- type: pd-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
schedulingProfiles:
- name: prefill
  plugins:
  - pluginRef: prefill-filter
  - pluginRef: queue-scorer
  - pluginRef: max-score-picker
- name: decode
  plugins:
  - pluginRef: decode-filter
  - pluginRef: prefix-cache-scorer
  - pluginRef: queue-scorer
  - pluginRef: max-score-picker
`

	tests := []struct {
		name           string
		version        string
		extraArgs      []string
		validateConfig func(g Gomega, configText string)
		validateArgs   func(g Gomega, args []string)
	}{
		{
			name:    "full migration of old v0.6 config to v0.7",
			version: "0.7.0",
			extraArgs: []string{
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--total-running-requests-metric", "vllm:num_requests_running",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--grpc-port", "9002",
			},
			validateConfig: func(g Gomega, configText string) {
				// Plugin renames applied
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))

				// Parameter restructuring applied
				g.Expect(configText).To(ContainSubstring("prefill: always-disagg-pd-decider"))
				g.Expect(configText).NotTo(ContainSubstring("deciderPluginName"))

				// Deprecated field removed
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))

				// Pre-existing migrations applied
				g.Expect(configText).To(ContainSubstring("blockSizeTokens"))

				// CLI flag values moved to model-server-protocol-metrics plugin
				g.Expect(configText).To(ContainSubstring("model-server-protocol-metrics"))
				g.Expect(configText).To(ContainSubstring("vllm:num_requests_waiting"))
				g.Expect(configText).To(ContainSubstring("vllm:num_requests_running"))
				g.Expect(configText).To(ContainSubstring("vllm:kv_cache_usage_perc"))

				// Unchanged plugins preserved
				g.Expect(configText).To(ContainSubstring("prefill-filter"))
				g.Expect(configText).To(ContainSubstring("decode-filter"))
				g.Expect(configText).To(ContainSubstring("queue-scorer"))
				g.Expect(configText).To(ContainSubstring("max-score-picker"))
			},
			validateArgs: func(g Gomega, args []string) {
				// Metric flags removed
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("total-running-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("kv-cache-usage-percentage-metric"))
				}
				// Non-metric flags preserved
				g.Expect(args).To(ContainElement("--grpc-port"))
				g.Expect(args).To(ContainElement("9002"))
			},
		},
		{
			name:    "v0.8.0 renames model-server-protocol-metrics to core-metrics-extractor",
			version: "0.8.0",
			extraArgs: []string{
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--total-running-requests-metric", "vllm:num_requests_running",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--grpc-port", "9002",
			},
			validateConfig: func(g Gomega, configText string) {
				// v0.8.0 plugin name used
				g.Expect(configText).To(ContainSubstring("core-metrics-extractor"))
				g.Expect(configText).NotTo(ContainSubstring("model-server-protocol-metrics"))

				// Metric values present
				g.Expect(configText).To(ContainSubstring("vllm:num_requests_waiting"))
				g.Expect(configText).To(ContainSubstring("vllm:num_requests_running"))
				g.Expect(configText).To(ContainSubstring("vllm:kv_cache_usage_perc"))

				// v0.7 renames also applied
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
			},
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("total-running-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("kv-cache-usage-percentage-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
				g.Expect(args).To(ContainElement("9002"))
			},
		},
		{
			name:    "v0.9.0 disagg config gets params stripped from prefix-cache-scorer",
			version: "0.9.0",
			extraArgs: []string{
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--total-running-requests-metric", "vllm:num_requests_running",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--grpc-port", "9002",
			},
			validateConfig: func(g Gomega, configText string) {
				// v0.7 + v0.8 renames applied
				g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
				g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("core-metrics-extractor"))
				g.Expect(configText).NotTo(ContainSubstring("model-server-protocol-metrics"))

				// prefix-cache-scorer params stripped (blockSizeTokens removed)
				g.Expect(configText).NotTo(ContainSubstring("blockSizeTokens"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))

				// No split happened (no precise-prefix-cache-scorer in input)
				g.Expect(configText).NotTo(ContainSubstring("precise-prefix-cache-producer"))
				g.Expect(configText).NotTo(ContainSubstring("token-producer"))
				g.Expect(configText).NotTo(ContainSubstring("endpoint-notification-source"))

				// prefix-cache-scorer plugin itself still exists
				g.Expect(configText).To(ContainSubstring("prefix-cache-scorer"))
			},
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("total-running-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("kv-cache-usage-percentage-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
				g.Expect(args).To(ContainElement("9002"))
			},
		},
		{
			name:    "old config left untouched for v0.6.0",
			version: "0.6.0",
			extraArgs: []string{
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
			},
			validateConfig: func(g Gomega, configText string) {
				// v0.7 renames NOT applied
				g.Expect(configText).To(ContainSubstring("prefill-header-handler"))
				g.Expect(configText).To(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).To(ContainSubstring("deciderPluginName"))
				g.Expect(configText).To(ContainSubstring("hashBlockSize"))

				// No model-server-protocol-metrics injected
				g.Expect(configText).NotTo(ContainSubstring("model-server-protocol-metrics"))

				// Pre-existing migrations still applied (unconditional)
				g.Expect(configText).To(ContainSubstring("blockSizeTokens"))
			},
			validateArgs: func(g Gomega, args []string) {
				// CLI flags preserved for v0.6 binary
				g.Expect(args).To(ContainElement("--total-queued-requests-metric"))
				g.Expect(args).To(ContainElement("vllm:num_requests_waiting"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			args := append([]string{"--config-text", oldConfigYAML}, tt.extraArgs...)
			d := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"app.kubernetes.io/version": tt.version,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Args: args},
							},
						},
					},
				},
			}

			ctx := context.Background()

			// Stage 1: unconditional pre-v0.7 migrations
			g.Expect(mutateSchedulerConfig(ctx, d,
				WithMigrateTokenProcessorConfig,
				WithMigrateBlockSizeToBlockSizeTokens,
			)).To(Succeed())

			// Stage 2: version-gated v0.7 migrations (single pass)
			g.Expect(schedulerTransform(ctx, d)).To(Succeed())

			resultArgs := d.Spec.Template.Spec.Containers[0].Args
			for i, a := range resultArgs {
				if a == "--config-text" && i+1 < len(resultArgs) {
					tt.validateConfig(g, resultArgs[i+1])
					validateDeciderOrderFromYAML(g, resultArgs[i+1])
				}
			}
			tt.validateArgs(g, resultArgs)
		})
	}
}

func TestFullMigrationPipelineNonZeroThreshold(t *testing.T) {
	// This tests a different input config (with threshold:100) so it stays
	// as its own top-level test but is logically part of the pipeline suite.
	oldConfigYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: prefill-filter
- type: decode-filter
- type: queue-scorer
- type: prefix-cache-scorer
  parameters:
    hashBlockSize: 64
    blockSizeTokens: 16
- type: max-score-picker
- type: pd-profile-handler
  parameters:
    threshold: 100
schedulingProfiles:
- name: prefill
  plugins:
  - pluginRef: prefill-filter
  - pluginRef: queue-scorer
  - pluginRef: max-score-picker
- name: decode
  plugins:
  - pluginRef: decode-filter
  - pluginRef: prefix-cache-scorer
  - pluginRef: queue-scorer
  - pluginRef: max-score-picker
`
	g := NewGomegaWithT(t)

	d := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"app.kubernetes.io/version": "0.7.0",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Args: []string{"--config-text", oldConfigYAML}},
					},
				},
			},
		},
	}

	ctx := context.Background()
	g.Expect(schedulerTransform(ctx, d)).To(Succeed())

	configText := d.Spec.Template.Spec.Containers[0].Args[1]

	// Plugin renames applied
	g.Expect(configText).To(ContainSubstring("disagg-headers-handler"))
	g.Expect(configText).NotTo(ContainSubstring("prefill-header-handler"))
	g.Expect(configText).To(ContainSubstring("disagg-profile-handler"))
	g.Expect(configText).NotTo(ContainSubstring("pd-profile-handler"))

	// Non-zero threshold migrated to prefix-based-pd-decider
	g.Expect(configText).NotTo(ContainSubstring("threshold"))
	g.Expect(configText).To(ContainSubstring("prefix-based-pd-decider"))
	g.Expect(configText).To(ContainSubstring("nonCachedTokens"))
	g.Expect(configText).To(ContainSubstring("prefill: prefix-based-pd-decider"))

	// Deprecated field removed
	g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
	g.Expect(configText).To(ContainSubstring("blockSizeTokens"))

	// Unchanged plugins preserved
	g.Expect(configText).To(ContainSubstring("prefill-filter"))
	g.Expect(configText).To(ContainSubstring("decode-filter"))
	g.Expect(configText).To(ContainSubstring("queue-scorer"))
	g.Expect(configText).To(ContainSubstring("max-score-picker"))

	// Decider ordering invariant
	validateDeciderOrderFromYAML(g, configText)
}

// TestPrecisePrefixCacheMigrationV09 is the unified test suite for all v0.9
// precise-prefix-cache migration scenarios. It covers the split migration,
// schema validation, orphan removal, idempotency, and edge cases.
func TestPrecisePrefixCacheMigrationV09(t *testing.T) {
	// Shared RHOAI-style old config used by multiple subtests
	oldConfigYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
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
      tokenizersPoolConfig:
        modelName: base
        uds:
          socketFile: /tmp/tokenizer/tokenizer-uds.socket
    kvEventsConfig:
      topicFilter: kv
      zmqEndpoint: tcp://*:5557
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`

	// Helper: build a v0.9 deployment from config YAML with optional tokenizer sidecar
	makeDeployment := func(configYAML string, withTokenizer bool) *appsv1.Deployment {
		containers := []corev1.Container{
			{Name: "main", Args: []string{"--config-text", configYAML}},
		}
		if withTokenizer {
			containers = append(containers, corev1.Container{Name: "tokenizer"})
		}
		return &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"app.kubernetes.io/version": "0.9.0",
						},
					},
					Spec: corev1.PodSpec{Containers: containers},
				},
			},
		}
	}

	// Helper: run the full production pipeline (Stage 1 + Stage 2)
	runFullPipeline := func(g Gomega, d *appsv1.Deployment) string {
		ctx := context.Background()
		g.Expect(mutateSchedulerConfig(ctx, d,
			WithUdsTokenizerConfig,
			WithMigrateTokenProcessorConfig,
			WithMigrateBlockSizeToBlockSizeTokens,
		)).To(Succeed())
		g.Expect(schedulerTransform(ctx, d)).To(Succeed())
		return d.Spec.Template.Spec.Containers[0].Args[1]
	}

	t.Run("split_migration/full_pipeline_converts_old_config", func(t *testing.T) {
		g := NewGomegaWithT(t)
		d := makeDeployment(oldConfigYAML, true)
		configText := runFullPipeline(g, d)

		// Deprecated plugin replaced
		g.Expect(configText).NotTo(ContainSubstring("precise-prefix-cache-scorer"))

		// New split plugins present
		g.Expect(configText).To(ContainSubstring("precise-prefix-cache-producer"))
		g.Expect(configText).To(ContainSubstring("prefix-cache-scorer"))
		g.Expect(configText).To(ContainSubstring("token-producer"))
		g.Expect(configText).To(ContainSubstring("endpoint-notification-source"))

		// token-producer has UDS config
		g.Expect(configText).To(ContainSubstring("socketFile"))
		g.Expect(configText).NotTo(ContainSubstring("modelName"))

		// prefix-cache-scorer references the producer
		g.Expect(configText).To(ContainSubstring("prefixMatchInfoProducerName: precise-prefix-cache-producer"))

		// tokenProcessorConfig at top level of producer params
		g.Expect(configText).To(ContainSubstring("tokenProcessorConfig"))
		g.Expect(configText).To(ContainSubstring("blockSize"))
		g.Expect(configText).To(ContainSubstring("hashSeed"))

		// tokenizersPoolConfig removed from indexerConfig
		g.Expect(configText).NotTo(ContainSubstring("tokenizersPoolConfig"))

		// kvEventsConfig preserved
		g.Expect(configText).To(ContainSubstring("kvEventsConfig"))
		g.Expect(configText).To(ContainSubstring("topicFilter"))

		// dataLayer wired
		g.Expect(configText).To(ContainSubstring("dataLayer"))

		// schedulingProfiles updated
		g.Expect(configText).To(ContainSubstring("pluginRef: prefix-cache-scorer"))

		// apiVersion updated
		g.Expect(configText).To(ContainSubstring("llm-d.ai/v1alpha1"))

		// tokenizer sidecar kept (token-producer uses UDS)
		hasTokenizer := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "tokenizer" {
				hasTokenizer = true
			}
		}
		g.Expect(hasTokenizer).To(BeTrue(), "tokenizer sidecar should be kept for UDS token-producer")
	})

	t.Run("split_migration/no_op_when_precise_prefix_not_present", func(t *testing.T) {
		g := NewGomegaWithT(t)

		basicConfig := `apiVersion: inference.networking.x-k8s.io/v1alpha1
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
		d := makeDeployment(basicConfig, true)
		ctx := context.Background()
		g.Expect(schedulerTransform(ctx, d)).To(Succeed())

		configText := d.Spec.Template.Spec.Containers[0].Args[1]

		// No split happened
		g.Expect(configText).NotTo(ContainSubstring("precise-prefix-cache-producer"))
		g.Expect(configText).NotTo(ContainSubstring("token-producer"))

		// Tokenizer sidecar removed (no UDS plugins)
		hasTokenizer := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "tokenizer" {
				hasTokenizer = true
			}
		}
		g.Expect(hasTokenizer).To(BeFalse(), "tokenizer sidecar should be removed when no UDS plugins exist")
	})

	t.Run("schema_validation/output_matches_v09_strict_decoder", func(t *testing.T) {
		g := NewGomegaWithT(t)

		schemaTestConfig := `apiVersion: inference.networking.x-k8s.io/v1alpha1
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
        metricsLoggingInterval: 60000000000
      tokenizersPoolConfig:
        modelName: base
        uds:
          socketFile: /tmp/tokenizer/tokenizer-uds.socket
    kvEventsConfig:
      topicFilter: kv
      zmqEndpoint: tcp://*:5557
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`
		d := makeDeployment(schemaTestConfig, true)
		configText := runFullPipeline(g, d)

		u := unstructured.Unstructured{}
		g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())

		plugins, found, _ := unstructured.NestedSlice(u.Object, "plugins")
		g.Expect(found).To(BeTrue(), "plugins must exist in output")

		validIndexerFields := map[string]bool{
			"kvBlockIndexConfig":    true,
			"kvCacheBackendConfigs": true,
			"tokenizersPoolConfig":  true,
		}
		validProducerParams := map[string]bool{
			"tokenProcessorConfig": true,
			"indexerConfig":        true,
			"kvEventsConfig":       true,
			"speculativeIndexing":  true,
			"speculativeTTL":       true,
		}
		validTokenProducerParams := map[string]bool{
			"modelName":          true,
			"vllm":               true,
			"udsTokenizerConfig": true,
			"estimate":           true,
		}

		for _, p := range plugins {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			pluginType, _ := pm["type"].(string)
			params, _ := pm["parameters"].(map[string]interface{})

			switch pluginType {
			case "precise-prefix-cache-producer":
				for key := range params {
					g.Expect(validProducerParams).To(HaveKey(key),
						"producer parameter %q would be rejected by v0.9 strict decoder", key)
				}
				ic, _ := params["indexerConfig"].(map[string]interface{})
				for key := range ic {
					g.Expect(validIndexerFields).To(HaveKey(key),
						"indexerConfig field %q would be rejected by v0.9 strict decoder", key)
				}
				_, found, _ := unstructured.NestedFieldNoCopy(pm, "parameters", "indexerConfig", "tokenProcessorConfig")
				g.Expect(found).To(BeFalse(), "tokenProcessorConfig inside indexerConfig would crash v0.9")
				_, found, _ = unstructured.NestedFieldNoCopy(pm, "parameters", "indexerConfig", "tokenizersPoolConfig")
				g.Expect(found).To(BeFalse(), "tokenizersPoolConfig inside indexerConfig would be rejected at v0.9")
				_, found, _ = unstructured.NestedFieldNoCopy(pm, "parameters", "tokenProcessorConfig")
				g.Expect(found).To(BeTrue(), "tokenProcessorConfig must be at top level")

			case "token-producer":
				for key := range params {
					g.Expect(validTokenProducerParams).To(HaveKey(key),
						"token-producer parameter %q would be rejected by v0.9 strict decoder", key)
				}
				_, hasUDS := params["udsTokenizerConfig"]
				g.Expect(hasUDS).To(BeTrue(), "token-producer must have udsTokenizerConfig")

			case "prefix-cache-scorer":
				for key := range params {
					g.Expect(key).To(Equal("prefixMatchInfoProducerName"),
						"prefix-cache-scorer should only have prefixMatchInfoProducerName at v0.9, got %q", key)
				}

			case "precise-prefix-cache-scorer":
				t.Fatal("deprecated precise-prefix-cache-scorer must not exist in v0.9 output")
			}
		}

		dl, found, _ := unstructured.NestedFieldNoCopy(u.Object, "dataLayer")
		g.Expect(found).To(BeTrue(), "dataLayer must be present")
		dlMap, ok := dl.(map[string]interface{})
		g.Expect(ok).To(BeTrue())
		sources, ok := dlMap["sources"].([]interface{})
		g.Expect(ok).To(BeTrue())
		g.Expect(sources).ToNot(BeEmpty())
	})

	t.Run("orphan_removal/promote_only_does_not_delete_from_indexerConfig", func(t *testing.T) {
		g := NewGomegaWithT(t)

		configYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 64
        hashSeed: "42"
      kvBlockIndexConfig:
        enableMetrics: true
    kvEventsConfig:
      topicFilter: kv
`
		d := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Args: []string{"--config-text", configYAML}},
						},
					},
				},
			},
		}

		ctx := context.Background()
		// WithMigrateTokenProcessorConfig only promotes — does NOT delete from indexerConfig.
		// Deletion is handled by withSplitPrecisePrefixCacheScorerV09 (v0.9-gated).
		g.Expect(mutateSchedulerConfig(ctx, d, WithMigrateTokenProcessorConfig)).To(Succeed())

		configText := d.Spec.Template.Spec.Containers[0].Args[1]

		u := unstructured.Unstructured{}
		g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())

		plugins, _, _ := unstructured.NestedSlice(u.Object, "plugins")
		for _, p := range plugins {
			pm, ok := p.(map[string]interface{})
			if !ok || pm["type"] != "precise-prefix-cache-scorer" {
				continue
			}
			// Field remains in indexerConfig (valid for v0.7/v0.8 kvcache.Config)
			_, found, _ := unstructured.NestedFieldNoCopy(pm, "parameters", "indexerConfig", "tokenProcessorConfig")
			g.Expect(found).To(BeTrue(), "tokenProcessorConfig should remain in indexerConfig for v0.7/v0.8 compat")

			// Field is also promoted to top level
			_, found, _ = unstructured.NestedFieldNoCopy(pm, "parameters", "tokenProcessorConfig")
			g.Expect(found).To(BeTrue(), "tokenProcessorConfig should exist at top level after promotion")
		}
	})

	t.Run("orphan_removal/v09_split_removes_from_indexerConfig", func(t *testing.T) {
		g := NewGomegaWithT(t)

		configYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 64
        hashSeed: "42"
      kvBlockIndexConfig:
        enableMetrics: true
    kvEventsConfig:
      topicFilter: kv
`
		d := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Args: []string{"--config-text", configYAML}},
						},
					},
				},
			},
		}

		ctx := context.Background()
		// Full v0.9 pipeline: promote then split (which removes orphan)
		g.Expect(mutateSchedulerConfig(ctx, d, WithMigrateTokenProcessorConfig, withSplitPrecisePrefixCacheScorerV09)).To(Succeed())

		configText := d.Spec.Template.Spec.Containers[0].Args[1]

		u := unstructured.Unstructured{}
		g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())

		// After split, precise-prefix-cache-producer gets indexerConfig without tokenProcessorConfig
		plugins, _, _ := unstructured.NestedSlice(u.Object, "plugins")
		for _, p := range plugins {
			pm, ok := p.(map[string]interface{})
			if !ok || pm["type"] != "precise-prefix-cache-producer" {
				continue
			}
			_, found, _ := unstructured.NestedFieldNoCopy(pm, "parameters", "indexerConfig", "tokenProcessorConfig")
			g.Expect(found).To(BeFalse(), "tokenProcessorConfig must be removed from indexerConfig in v0.9 split")

			_, found, _ = unstructured.NestedFieldNoCopy(pm, "parameters", "tokenProcessorConfig")
			g.Expect(found).To(BeTrue(), "tokenProcessorConfig should exist at producer top level")
		}
	})

	t.Run("idempotency/already_migrated_config_unchanged", func(t *testing.T) {
		g := NewGomegaWithT(t)

		migratedConfigYAML := `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- name: token-producer
  type: token-producer
  parameters:
    udsTokenizerConfig:
      socketFile: /tmp/tokenizer/tokenizer-uds.socket
- name: endpoint-notification-source
  type: endpoint-notification-source
- name: precise-prefix-cache-producer
  type: precise-prefix-cache-producer
  parameters:
    tokenProcessorConfig:
      blockSize: 64
      hashSeed: "42"
    indexerConfig:
      kvBlockIndexConfig:
        enableMetrics: true
    kvEventsConfig:
      topicFilter: kv
      zmqEndpoint: tcp://*:5557
- name: prefix-cache-scorer
  type: prefix-cache-scorer
  parameters:
    prefixMatchInfoProducerName: precise-prefix-cache-producer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
dataLayer:
  sources:
  - pluginRef: endpoint-notification-source
    extractors:
    - pluginRef: precise-prefix-cache-producer
`
		d := makeDeployment(migratedConfigYAML, true)
		resultConfig := runFullPipeline(g, d)

		// Critical: no duplicates or breakage introduced
		g.Expect(resultConfig).To(ContainSubstring("precise-prefix-cache-producer"))
		g.Expect(resultConfig).To(ContainSubstring("token-producer"))
		g.Expect(resultConfig).To(ContainSubstring("prefix-cache-scorer"))
		g.Expect(resultConfig).To(ContainSubstring("endpoint-notification-source"))
		g.Expect(resultConfig).To(ContainSubstring("prefixMatchInfoProducerName: precise-prefix-cache-producer"))
		g.Expect(resultConfig).NotTo(ContainSubstring("precise-prefix-cache-scorer"))

		// Structural validation
		result := unstructured.Unstructured{}
		g.Expect(yaml.Unmarshal([]byte(resultConfig), &result)).To(Succeed())

		plugins, _, _ := unstructured.NestedSlice(result.Object, "plugins")
		for _, p := range plugins {
			pm, ok := p.(map[string]interface{})
			if !ok || pm["type"] != "precise-prefix-cache-producer" {
				continue
			}
			_, found, _ := unstructured.NestedFieldNoCopy(pm, "parameters", "indexerConfig", "tokenProcessorConfig")
			g.Expect(found).To(BeFalse(), "tokenProcessorConfig must not appear inside indexerConfig after re-run")
			_, found, _ = unstructured.NestedFieldNoCopy(pm, "parameters", "tokenProcessorConfig")
			g.Expect(found).To(BeTrue(), "tokenProcessorConfig must remain at top level")
		}

		// Tokenizer sidecar preserved
		hasTokenizer := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "tokenizer" {
				hasTokenizer = true
			}
		}
		g.Expect(hasTokenizer).To(BeTrue(), "tokenizer sidecar must be kept on re-run")

		g.Expect(resultConfig).To(ContainSubstring("llm-d.ai/v1alpha1"))
	})
}

func TestExtractDeprecatedMetricFlags(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedFiltered  []string
		expectedExtracted map[string]string
	}{
		{
			name: "extracts all metric flags",
			args: []string{
				"--config-text", "someyaml",
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--grpc-port", "9002",
			},
			expectedFiltered: []string{"--config-text", "someyaml", "--grpc-port", "9002"},
			expectedExtracted: map[string]string{
				"total-queued-requests-metric":     "vllm:num_requests_waiting",
				"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc",
			},
		},
		{
			name:              "no metric flags - returns nil",
			args:              []string{"--config-text", "someyaml", "--grpc-port", "9002"},
			expectedFiltered:  []string{"--config-text", "someyaml", "--grpc-port", "9002"},
			expectedExtracted: nil,
		},
		{
			name: "extracts all five deprecated flags",
			args: []string{
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--total-running-requests-metric", "vllm:num_requests_running",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--lora-info-metric", "vllm:lora_requests_info",
				"--cache-info-metric", "vllm:cache_config_info",
			},
			expectedFiltered: nil,
			expectedExtracted: map[string]string{
				"total-queued-requests-metric":     "vllm:num_requests_waiting",
				"total-running-requests-metric":    "vllm:num_requests_running",
				"kv-cache-usage-percentage-metric": "vllm:kv_cache_usage_perc",
				"lora-info-metric":                 "vllm:lora_requests_info",
				"cache-info-metric":                "vllm:cache_config_info",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			d := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Args: tt.args},
							},
						},
					},
				},
			}
			extracted := extractDeprecatedMetricFlags(d)
			g.Expect(d.Spec.Template.Spec.Containers[0].Args).To(Equal(tt.expectedFiltered))
			g.Expect(extracted).To(Equal(tt.expectedExtracted))
		})
	}
}

func TestSchedulerTransformGatesMetricFlagExtraction(t *testing.T) {
	configYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
`

	tests := []struct {
		name           string
		args           []string
		expectErr      bool
		errSubstring   string
		validateArgs   func(g Gomega, args []string)
		validateConfig func(g Gomega, args []string)
	}{
		{
			name: "config-file with deprecated flags returns error",
			args: []string{
				"--config-file", "/etc/scheduler/config.yaml",
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
			},
			expectErr:    true,
			errSubstring: "no inline --config-text",
		},
		{
			name: "no config flag with deprecated flags returns error",
			args: []string{
				"--grpc-port", "9002",
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
			},
			expectErr:    true,
			errSubstring: "no inline --config-text",
		},
		{
			name: "config-file without deprecated flags succeeds",
			args: []string{
				"--config-file", "/etc/scheduler/config.yaml",
				"--grpc-port", "9002",
			},
			expectErr: false,
			validateArgs: func(g Gomega, args []string) {
				g.Expect(args).To(ContainElement("--config-file"))
				g.Expect(args).To(ContainElement("/etc/scheduler/config.yaml"))
				g.Expect(args).To(ContainElement("--grpc-port"))
			},
		},
		{
			name: "configText camelCase with deprecated flags succeeds",
			args: []string{
				"--configText", configYAML,
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--kv-cache-usage-percentage-metric", "vllm:kv_cache_usage_perc",
				"--grpc-port", "9002",
			},
			expectErr: false,
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
					g.Expect(a).NotTo(ContainSubstring("kv-cache-usage-percentage-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
			},
			validateConfig: func(g Gomega, args []string) {
				for i, a := range args {
					if a == "--configText" && i+1 < len(args) {
						g.Expect(args[i+1]).To(ContainSubstring("model-server-protocol-metrics"))
						g.Expect(args[i+1]).To(ContainSubstring("vllm:num_requests_waiting"))
						g.Expect(args[i+1]).To(ContainSubstring("vllm:kv_cache_usage_perc"))
						return
					}
				}
				g.Expect(true).To(BeFalse(), "expected --configText arg not found")
			},
		},
		{
			name: "config-text with deprecated flags succeeds (existing behavior)",
			args: []string{
				"--config-text", configYAML,
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--grpc-port", "9002",
			},
			expectErr: false,
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
			},
			validateConfig: func(g Gomega, args []string) {
				for i, a := range args {
					if a == "--config-text" && i+1 < len(args) {
						g.Expect(args[i+1]).To(ContainSubstring("model-server-protocol-metrics"))
						g.Expect(args[i+1]).To(ContainSubstring("vllm:num_requests_waiting"))
						return
					}
				}
				g.Expect(true).To(BeFalse(), "expected --config-text arg not found")
			},
		},
		{
			name: "single-dash -config-text with deprecated flags succeeds",
			args: []string{
				"-config-text", configYAML,
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--grpc-port", "9002",
			},
			expectErr: false,
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
			},
			validateConfig: func(g Gomega, args []string) {
				for i, a := range args {
					if a == "-config-text" && i+1 < len(args) {
						g.Expect(args[i+1]).To(ContainSubstring("model-server-protocol-metrics"))
						g.Expect(args[i+1]).To(ContainSubstring("vllm:num_requests_waiting"))
						return
					}
				}
				g.Expect(true).To(BeFalse(), "expected -config-text arg not found")
			},
		},
		{
			name: "single-dash -configText with deprecated flags succeeds",
			args: []string{
				"-configText", configYAML,
				"--total-queued-requests-metric", "vllm:num_requests_waiting",
				"--grpc-port", "9002",
			},
			expectErr: false,
			validateArgs: func(g Gomega, args []string) {
				for _, a := range args {
					g.Expect(a).NotTo(ContainSubstring("total-queued-requests-metric"))
				}
				g.Expect(args).To(ContainElement("--grpc-port"))
			},
			validateConfig: func(g Gomega, args []string) {
				for i, a := range args {
					if a == "-configText" && i+1 < len(args) {
						g.Expect(args[i+1]).To(ContainSubstring("model-server-protocol-metrics"))
						g.Expect(args[i+1]).To(ContainSubstring("vllm:num_requests_waiting"))
						return
					}
				}
				g.Expect(true).To(BeFalse(), "expected -configText arg not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			d := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scheduler",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"app.kubernetes.io/version": "0.7.0",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main", Args: tt.args},
							},
						},
					},
				},
			}

			err := schedulerTransform(context.Background(), d)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstring))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			resultArgs := d.Spec.Template.Spec.Containers[0].Args
			if tt.validateArgs != nil {
				tt.validateArgs(g, resultArgs)
			}
			if tt.validateConfig != nil {
				tt.validateConfig(g, resultArgs)
			}
		})
	}
}
