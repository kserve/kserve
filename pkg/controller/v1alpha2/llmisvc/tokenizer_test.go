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

func TestIsTokenizerEnabled(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha2.LLMInferenceServiceSpec
		want bool
	}{
		{
			name: "tokenizer field explicitly set",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Tokenizer: &v1alpha2.TokenizerSpec{},
					},
				},
			},
			want: true,
		},
		{
			name: "tokenizer field with template",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Tokenizer: &v1alpha2.TokenizerSpec{
							Template: &corev1.PodSpec{},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "token-producer plugin present",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{
							Inline: &runtime.RawExtension{
								Raw: []byte(`{"plugins":[{"type":"token-producer"},{"type":"precise-prefix-cache-producer"}]}`),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "legacy precise-prefix-cache-scorer present",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{
							Inline: &runtime.RawExtension{
								Raw: []byte(`{"plugins":[{"type":"precise-prefix-cache-scorer"}]}`),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "no tokenizer, no precise-prefix-cache-scorer, no token-producer",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{
							Inline: &runtime.RawExtension{
								Raw: []byte(`{"plugins":[{"type":"prefix-cache-scorer"}]}`),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "nil scheduler",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{},
			},
			want: false,
		},
		{
			name: "nil router",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			want: false,
		},
		{
			name: "empty scheduler, no config",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{},
				},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isTokenizerEnabled(tc.spec)
			if got != tc.want {
				t.Errorf("isTokenizerEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTokenizerServiceURL(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-llm",
			Namespace: "default",
		},
	}

	url := tokenizerServiceURL(llmSvc)
	g := NewGomegaWithT(t)
	g.Expect(url).To(ContainSubstring("my-llm-tokenizer"))
	g.Expect(url).To(ContainSubstring("default"))
	g.Expect(url).To(ContainSubstring(":8000"))
	g.Expect(url).To(HavePrefix("http://"))
}

func TestTokenizerLabels(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-llm",
		},
	}

	labels := TokenizerLabels(llmSvc)
	g := NewGomegaWithT(t)
	g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "tokenizer"))
	g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "my-llm"))
	g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "llminferenceservice"))
}

func TestExpectedTokenizerService(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-llm",
			Namespace: "default",
		},
	}

	r := &LLMISVCReconciler{}
	svc := r.expectedTokenizerService(llmSvc)

	g := NewGomegaWithT(t)
	g.Expect(svc.Name).To(Equal("my-llm-tokenizer"))
	g.Expect(svc.Namespace).To(Equal("default"))
	g.Expect(svc.Spec.Ports).To(HaveLen(1))
	g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8000)))
	g.Expect(svc.Spec.Ports[0].Name).To(Equal("render-http"))
	g.Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/component", "tokenizer"))
	g.Expect(svc.OwnerReferences).To(HaveLen(1))
}

func TestDecomposePluginPipeline(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		tokenizerURL   string
		validateConfig func(g Gomega, configText string)
	}{
		{
			name: "decomposes and strips tokenizersPoolConfig-only indexerConfig",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenizersPoolConfig:
        modelName: base
        uds:
          socketFile: /tmp/tokenizer/tokenizer-uds.socket
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: max-score-picker
`,
			tokenizerURL: "http://my-llm-tokenizer.default.svc.cluster.local:8000",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).NotTo(ContainSubstring(precisePrefixCacheScorerPlugin))
				g.Expect(configText).To(ContainSubstring(tokenProducerPlugin))
				g.Expect(configText).To(ContainSubstring(precisePrefixCacheProducerPlugin))
				g.Expect(configText).To(ContainSubstring(prefixCacheScorerPlugin))
				g.Expect(configText).To(ContainSubstring("http://my-llm-tokenizer.default.svc.cluster.local:8000"))
				g.Expect(configText).To(ContainSubstring("prefixMatchInfoProducerName"))
				g.Expect(configText).To(ContainSubstring("modelName: /mnt/models/base"))

				// tokenizersPoolConfig should be gone, and since it was the only
				// entry in indexerConfig the producer should have no parameters.
				g.Expect(configText).NotTo(ContainSubstring("tokenizersPoolConfig"))
				g.Expect(configText).NotTo(ContainSubstring("socketFile"))

				u := unstructured.Unstructured{}
				g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())
				val, found, err := unstructured.NestedFieldNoCopy(u.Object, "plugins")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(found).To(BeTrue())
				plugins := val.([]interface{})
				g.Expect(plugins).To(HaveLen(5))

				g.Expect(plugins[0].(map[string]interface{})["type"]).To(Equal("queue-scorer"))
				g.Expect(plugins[1].(map[string]interface{})["type"]).To(Equal(tokenProducerPlugin))
				g.Expect(plugins[2].(map[string]interface{})["type"]).To(Equal(precisePrefixCacheProducerPlugin))
				g.Expect(plugins[3].(map[string]interface{})["type"]).To(Equal(prefixCacheScorerPlugin))
				g.Expect(plugins[4].(map[string]interface{})["type"]).To(Equal("max-score-picker"))

				// Verify schedulingProfiles pluginRef was renamed
				profiles, found, err := unstructured.NestedFieldNoCopy(u.Object, "schedulingProfiles")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(found).To(BeTrue())
				profileList := profiles.([]interface{})
				g.Expect(profileList).To(HaveLen(1))
				profileMap := profileList[0].(map[string]interface{})
				pluginRefs, found, _ := unstructured.NestedFieldNoCopy(profileMap, "plugins")
				g.Expect(found).To(BeTrue())
				refList := pluginRefs.([]interface{})
				refNames := make([]string, len(refList))
				for i, ref := range refList {
					refNames[i] = ref.(map[string]interface{})["pluginRef"].(string)
				}
				g.Expect(refNames).To(ContainElement(prefixCacheScorerPlugin))
				g.Expect(refNames).NotTo(ContainElement(precisePrefixCacheScorerPlugin))
			},
		},
		{
			name: "preserves tokenProcessorConfig, kvEventsConfig, and kvBlockIndexConfig on producer",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    tokenProcessorConfig:
      blockSize: 64
      hashSeed: "42"
    kvEventsConfig:
      topicFilter: "kv@"
      concurrency: 8
      discoverPods: true
      podDiscoveryConfig:
        socketPort: 5556
    indexerConfig:
      tokenizersPoolConfig:
        modelName: base
        uds:
          socketFile: /tmp/tokenizer/tokenizer-uds.socket
      kvBlockIndexConfig:
        enableMetrics: true
        metricsLoggingInterval: 60000000000
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`,
			tokenizerURL: "http://my-llm-tokenizer.default.svc.cluster.local:8000",
			validateConfig: func(g Gomega, configText string) {
				g.Expect(configText).NotTo(ContainSubstring(precisePrefixCacheScorerPlugin))
				g.Expect(configText).To(ContainSubstring(tokenProducerPlugin))
				g.Expect(configText).To(ContainSubstring(precisePrefixCacheProducerPlugin))
				g.Expect(configText).To(ContainSubstring("modelName: /mnt/models/base"))

				// tokenizersPoolConfig must be stripped
				g.Expect(configText).NotTo(ContainSubstring("tokenizersPoolConfig"))
				g.Expect(configText).NotTo(ContainSubstring("socketFile"))

				// All other parameters must be preserved on the producer.
				// YAML marshaling may or may not quote simple strings, so
				// match the unquoted form.
				g.Expect(configText).To(ContainSubstring("blockSize: 64"))
				g.Expect(configText).To(ContainSubstring("hashSeed:"))
				g.Expect(configText).To(ContainSubstring("42"))
				g.Expect(configText).To(ContainSubstring("topicFilter:"))
				g.Expect(configText).To(ContainSubstring("kv@"))
				g.Expect(configText).To(ContainSubstring("concurrency: 8"))
				g.Expect(configText).To(ContainSubstring("discoverPods: true"))
				g.Expect(configText).To(ContainSubstring("socketPort: 5556"))
				g.Expect(configText).To(ContainSubstring("enableMetrics: true"))
				g.Expect(configText).To(ContainSubstring("metricsLoggingInterval: 60000000000"))

				u := unstructured.Unstructured{}
				g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())
				val, _, _ := unstructured.NestedFieldNoCopy(u.Object, "plugins")
				plugins := val.([]interface{})
				producerPlugin := plugins[2].(map[string]interface{})
				g.Expect(producerPlugin["type"]).To(Equal(precisePrefixCacheProducerPlugin))
				params := producerPlugin["parameters"].(map[string]interface{})
				g.Expect(params).To(HaveKey("tokenProcessorConfig"))
				g.Expect(params).To(HaveKey("kvEventsConfig"))
				g.Expect(params).To(HaveKey("indexerConfig"))

				indexerConfig := params["indexerConfig"].(map[string]interface{})
				g.Expect(indexerConfig).To(HaveKey("kvBlockIndexConfig"))
				g.Expect(indexerConfig).NotTo(HaveKey("tokenizersPoolConfig"))
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
								{Name: "main", Args: []string{"--config-text", tt.configYAML}},
							},
						},
					},
				},
			}

			g.Expect(mutateSchedulerConfig(context.Background(), d, withDecomposePrecisePrefixCacheScorer(tt.tokenizerURL))).To(Succeed())

			configText := d.Spec.Template.Spec.Containers[0].Args[1]
			tt.validateConfig(g, configText)
		})
	}
}

func TestDecomposePluginPipeline_NoPrecisePrefix(t *testing.T) {
	configYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
`

	g := NewGomegaWithT(t)

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

	g.Expect(mutateSchedulerConfig(context.Background(), d, withDecomposePrecisePrefixCacheScorer("http://tokenizer:8000"))).To(Succeed())

	configText := d.Spec.Template.Spec.Containers[0].Args[1]
	g.Expect(configText).NotTo(ContainSubstring(tokenProducerPlugin))
	g.Expect(configText).NotTo(ContainSubstring(precisePrefixCacheProducerPlugin))
	g.Expect(configText).To(ContainSubstring("prefix-cache-scorer"))
}

func TestDecomposePluginPipeline_DuplicatePrefixCacheScorer(t *testing.T) {
	configYAML := `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
- type: precise-prefix-cache-scorer
  parameters:
    tokenProcessorConfig:
      blockSize: 64
    indexerConfig:
      tokenizersPoolConfig:
        modelName: base
        uds:
          socketFile: /tmp/tokenizer/tokenizer-uds.socket
- type: prefix-cache-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: prefix-cache-scorer
    weight: 2
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: max-score-picker
`

	g := NewGomegaWithT(t)

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

	g.Expect(mutateSchedulerConfig(context.Background(), d, withDecomposePrecisePrefixCacheScorer("http://tokenizer:8000"))).To(Succeed())

	configText := d.Spec.Template.Spec.Containers[0].Args[1]

	u := unstructured.Unstructured{}
	g.Expect(yaml.Unmarshal([]byte(configText), &u)).To(Succeed())

	// Verify no duplicate prefix-cache-scorer in plugins
	val, found, err := unstructured.NestedFieldNoCopy(u.Object, "plugins")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	plugins := val.([]interface{})

	prefixCacheScorerCount := 0
	for _, plugin := range plugins {
		pluginMap := plugin.(map[string]interface{})
		if pluginMap["type"] == prefixCacheScorerPlugin {
			prefixCacheScorerCount++
		}
	}
	g.Expect(prefixCacheScorerCount).To(Equal(1), "should have exactly one prefix-cache-scorer, got duplicates")

	// Verify no duplicate pluginRef in schedulingProfiles
	profiles, found, err := unstructured.NestedFieldNoCopy(u.Object, "schedulingProfiles")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	profileList := profiles.([]interface{})
	profileMap := profileList[0].(map[string]interface{})
	pluginRefs, found, _ := unstructured.NestedFieldNoCopy(profileMap, "plugins")
	g.Expect(found).To(BeTrue())
	refList := pluginRefs.([]interface{})

	prefixCacheRefCount := 0
	for _, ref := range refList {
		refMap := ref.(map[string]interface{})
		if refMap["pluginRef"] == prefixCacheScorerPlugin {
			prefixCacheRefCount++
		}
	}
	g.Expect(prefixCacheRefCount).To(Equal(1), "should have exactly one prefix-cache-scorer pluginRef, got duplicates")

	// Verify no precise-prefix-cache-scorer remains
	g.Expect(configText).NotTo(ContainSubstring(precisePrefixCacheScorerPlugin))

	// Verify modelName was set on the token-producer
	g.Expect(configText).To(ContainSubstring("modelName: /mnt/models/base"))
}

func TestInjectTokenProducerConfig(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		tokenizerURL string
		wantURL      bool
		wantModel    bool
	}{
		{
			name: "injects modelName and vllm.url when token-producer has no params",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: token-producer
- type: queue-scorer
`,
			tokenizerURL: "http://my-llm-tokenizer.default.svc.cluster.local:8000",
			wantURL:      true,
			wantModel:    true,
		},
		{
			name: "does not overwrite existing modelName or vllm.url",
			configYAML: `apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: token-producer
  parameters:
    modelName: custom-model
    vllm:
      url: http://custom-url:9999
- type: queue-scorer
`,
			tokenizerURL: "http://my-llm-tokenizer.default.svc.cluster.local:8000",
			wantURL:      false,
			wantModel:    false,
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
								{Name: "main", Args: []string{"--config-text", tt.configYAML}},
							},
						},
					},
				},
			}

			g.Expect(mutateSchedulerConfig(context.Background(), d, withInjectTokenProducerConfig(tt.tokenizerURL))).To(Succeed())

			configText := d.Spec.Template.Spec.Containers[0].Args[1]
			if tt.wantURL {
				g.Expect(configText).To(ContainSubstring(tt.tokenizerURL))
				g.Expect(configText).To(ContainSubstring("modelName: /mnt/models/base"))
			} else {
				g.Expect(configText).To(ContainSubstring("custom-model"))
				g.Expect(configText).To(ContainSubstring("http://custom-url:9999"))
				g.Expect(configText).NotTo(ContainSubstring(tt.tokenizerURL))
			}
		})
	}
}

func TestShouldDeleteTokenizer(t *testing.T) {
	tests := []struct {
		name   string
		llmSvc *v1alpha2.LLMInferenceService
		want   bool
	}{
		{
			name: "tokenizer enabled",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Tokenizer: &v1alpha2.TokenizerSpec{},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "no router",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{},
			},
			want: true,
		},
		{
			name: "no scheduler",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{},
				},
			},
			want: true,
		},
		{
			name: "external pool ref",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Tokenizer: &v1alpha2.TokenizerSpec{},
							Pool: &v1alpha2.InferencePoolSpec{
								Ref: &corev1.LocalObjectReference{Name: "external-pool"},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "tokenizer not enabled",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldDeleteTokenizer(tc.llmSvc)
			if got != tc.want {
				t.Errorf("shouldDeleteTokenizer() = %v, want %v", got, tc.want)
			}
		})
	}
}
