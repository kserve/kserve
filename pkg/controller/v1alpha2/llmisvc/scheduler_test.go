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

package llmisvc

import (
	"context"
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
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
				g.Expect(plugins).To(HaveLen(3))
				g.Expect(plugins[2].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
			},
		},
		{
			name: "migrates threshold 0 when decider plugin already exists",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: always-disagg-pd-decider
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				params := plugins[0].(map[string]interface{})["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
				g.Expect(plugins).To(HaveLen(2))
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(WithMigrateDisaggProfileParams(context.Background(), &u)).To(Succeed())
			tt.validate(g, u.Object)
		})
	}
}

func TestHasNonZeroThreshold(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		expected   bool
	}{
		{
			name: "no threshold - returns false",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
`,
			expected: false,
		},
		{
			name: "threshold 0 - returns false",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0
`,
			expected: false,
		},
		{
			name: "threshold 0.5 - returns true",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0.5
`,
			expected: true,
		},
		{
			name: "threshold 1 - returns true",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 1
`,
			expected: true,
		},
		{
			name: "deciders already present - returns false even with threshold",
			configYAML: `
plugins:
- type: disagg-profile-handler
  parameters:
    deciders:
      prefill: always-disagg-pd-decider
    threshold: 0.5
`,
			expected: false,
		},
		{
			name: "non-profile plugin with threshold - returns false",
			configYAML: `
plugins:
- type: some-other-plugin
  parameters:
    threshold: 0.5
`,
			expected: false,
		},
		{
			name: "no plugins - returns false",
			configYAML: `
schedulingProfiles:
- name: prefill
`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			var obj map[string]interface{}
			g.Expect(yaml.Unmarshal([]byte(tt.configYAML), &obj)).To(Succeed())
			u := unstructured.Unstructured{Object: obj}
			g.Expect(hasNonZeroThreshold(&u)).To(Equal(tt.expected))
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
			name: "skips all migration for non-zero threshold",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0.5
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				pluginMap := plugins[0].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("pd-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("deciders"))
				g.Expect(params).To(HaveKey("threshold"))
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
				pluginMap := plugins[0].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
				g.Expect(plugins).To(HaveLen(3))
				g.Expect(plugins[2].(map[string]interface{})["type"]).To(Equal("always-disagg-pd-decider"))
			},
		},
		{
			name: "handles threshold 0 when decider plugin already present",
			configYAML: `
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 0
- type: always-disagg-pd-decider
`,
			validate: func(g Gomega, obj map[string]interface{}) {
				plugins := obj["plugins"].([]interface{})
				pluginMap := plugins[0].(map[string]interface{})
				g.Expect(pluginMap["type"]).To(Equal("disagg-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).NotTo(HaveKey("threshold"))
				deciders := params["deciders"].(map[string]interface{})
				g.Expect(deciders).To(HaveKeyWithValue("prefill", "always-disagg-pd-decider"))
				g.Expect(plugins).To(HaveLen(2))
			},
		},
		{
			name: "handles non-zero threshold with deciderPluginName - skips all",
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
				g.Expect(pluginMap["type"]).To(Equal("pd-profile-handler"))
				params := pluginMap["parameters"].(map[string]interface{})
				g.Expect(params).To(HaveKey("deciderPluginName"))
				g.Expect(params).To(HaveKey("threshold"))
				g.Expect(params).NotTo(HaveKey("deciders"))
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
			name: "skips profile handler rename for non-zero threshold",
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
				g.Expect(configText).To(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).To(ContainSubstring("threshold"))
				g.Expect(configText).NotTo(ContainSubstring("hashBlockSize"))
			},
		},
		{
			name: "skips profile handler for non-zero threshold with deciderPluginName",
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
				g.Expect(configText).To(ContainSubstring("pd-profile-handler"))
				g.Expect(configText).NotTo(ContainSubstring("disagg-profile-handler"))
				g.Expect(configText).To(ContainSubstring("deciderPluginName"))
				g.Expect(configText).To(ContainSubstring("threshold"))
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

				// CLI flag values moved to core-metrics-extractor plugin
				g.Expect(configText).To(ContainSubstring("core-metrics-extractor"))
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

				// No core-metrics-extractor injected
				g.Expect(configText).NotTo(ContainSubstring("core-metrics-extractor"))

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
				}
			}
			tt.validateArgs(g, resultArgs)
		})
	}
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
						g.Expect(args[i+1]).To(ContainSubstring("core-metrics-extractor"))
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
						g.Expect(args[i+1]).To(ContainSubstring("core-metrics-extractor"))
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
						g.Expect(args[i+1]).To(ContainSubstring("core-metrics-extractor"))
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
						g.Expect(args[i+1]).To(ContainSubstring("core-metrics-extractor"))
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
