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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func schedulerSpecWithTokenizer(version string) v1alpha2.LLMInferenceServiceSpec {
	return v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Annotations: map[string]string{
					"app.kubernetes.io/version": version,
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main"},
						{Name: tokenizerContainerName, Image: "vllm/vllm-openai-cpu:v0.23.0"},
					},
				},
			},
		},
	}
}

func TestShouldDeployStandaloneTokenizer(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha2.LLMInferenceServiceSpec
		want bool
	}{
		{
			name: "version >= 0.9.0 with tokenizer container",
			spec: schedulerSpecWithTokenizer("0.9.0"),
			want: true,
		},
		{
			name: "version 1.0.0 with tokenizer container",
			spec: schedulerSpecWithTokenizer("1.0.0"),
			want: true,
		},
		{
			name: "version 0.8.0 with tokenizer container (below gate)",
			spec: schedulerSpecWithTokenizer("0.8.0"),
			want: false,
		},
		{
			name: "version 0.9.0 without tokenizer container",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Annotations: map[string]string{
							"app.kubernetes.io/version": "0.9.0",
						},
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "no version annotation",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main"},
								{Name: tokenizerContainerName},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "nil router",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldDeployStandaloneTokenizer(tc.spec)
			if got != tc.want {
				t.Errorf("shouldDeployStandaloneTokenizer() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractTokenizerContainer(t *testing.T) {
	g := NewGomegaWithT(t)

	spec := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "tls-certs", MountPath: "/tls"},
							},
						},
						{
							Name:  tokenizerContainerName,
							Image: "vllm/vllm-openai-cpu:v0.23.0",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "tokenizer-tmp", MountPath: "/tmp"},
								{Name: "tokenizer-cache", MountPath: "/.cache"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "tls-certs"},
						{Name: "tokenizer-tmp"},
						{Name: "tokenizer-cache"},
						{Name: "other-volume"},
					},
				},
			},
		},
	}

	container, volumes := extractTokenizerContainer(spec)
	g.Expect(container).ToNot(BeNil())
	g.Expect(container.Name).To(Equal(tokenizerContainerName))
	g.Expect(container.Image).To(Equal("vllm/vllm-openai-cpu:v0.23.0"))

	// Only volumes referenced by tokenizer's volume mounts should be extracted
	g.Expect(volumes).To(HaveLen(2))
	volumeNames := make([]string, len(volumes))
	for i, v := range volumes {
		volumeNames[i] = v.Name
	}
	g.Expect(volumeNames).To(ContainElements("tokenizer-tmp", "tokenizer-cache"))
	g.Expect(volumeNames).ToNot(ContainElement("tls-certs"))
	g.Expect(volumeNames).ToNot(ContainElement("other-volume"))
}

func TestExtractTokenizerContainer_NotFound(t *testing.T) {
	g := NewGomegaWithT(t)

	spec := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main"},
					},
				},
			},
		},
	}

	container, volumes := extractTokenizerContainer(spec)
	g.Expect(container).To(BeNil())
	g.Expect(volumes).To(BeNil())
}

func TestTokenizerLabels(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
	}
	labels := TokenizerLabels(llmSvc)

	if labels[constants.KubernetesComponentLabelKey] != constants.LLMComponentTokenizer {
		t.Errorf("component label = %q, want %q", labels[constants.KubernetesComponentLabelKey], constants.LLMComponentTokenizer)
	}
	if labels[constants.KubernetesAppNameLabelKey] != "my-model" {
		t.Errorf("app name label = %q, want %q", labels[constants.KubernetesAppNameLabelKey], "my-model")
	}
	if labels[constants.KubernetesPartOfLabelKey] != constants.LLMInferenceServicePartOfValue {
		t.Errorf("part-of label = %q, want %q", labels[constants.KubernetesPartOfLabelKey], constants.LLMInferenceServicePartOfValue)
	}
}

func TestTokenizerDeploymentName(t *testing.T) {
	g := NewGomegaWithT(t)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc"},
	}
	g.Expect(tokenizerDeploymentName(llmSvc)).To(ContainSubstring("test-svc"))
}

func TestTokenizerServiceName(t *testing.T) {
	g := NewGomegaWithT(t)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc"},
	}
	g.Expect(tokenizerServiceName(llmSvc)).To(ContainSubstring("test-svc"))
}

func TestTokenizerEndpointURL(t *testing.T) {
	g := NewGomegaWithT(t)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "test-ns"},
	}
	url := tokenizerEndpointURL(llmSvc)
	g.Expect(url).To(HavePrefix("http://"))
	g.Expect(url).To(ContainSubstring(tokenizerServiceName(llmSvc)))
	g.Expect(url).To(ContainSubstring("test-ns"))
	g.Expect(url).To(ContainSubstring(":8000"))
}

func TestExpectedTokenizerService(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default", UID: "test-uid"},
	}

	r := &LLMISVCReconciler{}
	svc := r.expectedTokenizerService(llmSvc)

	g.Expect(svc.Name).To(Equal(tokenizerServiceName(llmSvc)))
	g.Expect(svc.Namespace).To(Equal("default"))
	g.Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
	g.Expect(svc.Spec.Ports).To(HaveLen(1))
	g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(tokenizerServicePort)))
	g.Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(tokenizerServicePort)))
	g.Expect(svc.Spec.Selector).To(Equal(TokenizerLabels(llmSvc)))
	g.Expect(svc.OwnerReferences).To(HaveLen(1))
}

func TestWithTokenProducerPlugin(t *testing.T) {
	g := NewGomegaWithT(t)

	configYAML := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: queue-scorer
- type: precise-prefix-cache-scorer
  parameters:
    tokenProcessorConfig:
      blockSize: 64
    indexerConfig:
      kvBlockIndexConfig:
        enableMetrics: true
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

	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(configYAML), &u.Object); err != nil {
		t.Fatalf("failed to unmarshal test config YAML: %v", err)
	}

	endpointURL := "http://my-tokenizer-service.default.svc.cluster.local:8000"
	fn := WithTokenProducerPlugin(endpointURL)
	err := fn(context.Background(), u)
	g.Expect(err).ToNot(HaveOccurred())

	plugins, found, err := unstructured.NestedSlice(u.Object, "plugins")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())

	// precise-prefix-cache-scorer should be removed and replaced with 3 plugins
	// Expected order: single-profile-handler, queue-scorer, token-producer,
	//                 precise-prefix-cache-producer, prefix-cache-scorer, max-score-picker
	g.Expect(plugins).To(HaveLen(6))

	pluginTypes := make([]string, len(plugins))
	for i, p := range plugins {
		pm := p.(map[string]interface{})
		pluginTypes[i] = pm["type"].(string)
	}
	g.Expect(pluginTypes).To(Equal([]string{
		"single-profile-handler", "queue-scorer",
		"token-producer", "precise-prefix-cache-producer", "prefix-cache-scorer",
		"max-score-picker",
	}))

	// Verify token-producer parameters
	tp := plugins[2].(map[string]interface{})
	modelName, _, _ := unstructured.NestedString(tp, "parameters", "modelName")
	g.Expect(modelName).To(Equal(udsTokenizerBaseModelName))
	vllmURL, _, _ := unstructured.NestedString(tp, "parameters", "vllm", "url")
	g.Expect(vllmURL).To(Equal(endpointURL))

	// Verify precise-prefix-cache-producer parameters (migrated from old scorer)
	producer := plugins[3].(map[string]interface{})
	blockSize, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "tokenProcessorConfig", "blockSize")
	g.Expect(f).To(BeTrue())
	g.Expect(blockSize).To(BeEquivalentTo(64))
	enableMetrics, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "indexerConfig", "kvBlockIndexConfig", "enableMetrics")
	g.Expect(f).To(BeTrue())
	g.Expect(enableMetrics).To(BeTrue())
	discoverPods, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "kvEventsConfig", "discoverPods")
	g.Expect(f).To(BeTrue())
	g.Expect(discoverPods).To(BeTrue())

	// Verify prefix-cache-scorer parameters
	scorer := plugins[4].(map[string]interface{})
	producerName, _, _ := unstructured.NestedString(scorer, "parameters", "prefixMatchInfoProducerName")
	g.Expect(producerName).To(Equal("precise-prefix-cache-producer"))

	// Verify schedulingProfiles updated
	profiles, _, _ := unstructured.NestedSlice(u.Object, "schedulingProfiles")
	profileMap := profiles[0].(map[string]interface{})
	refs, _, _ := unstructured.NestedSlice(profileMap, "plugins")
	refTypes := make([]string, len(refs))
	for i, r := range refs {
		rm := r.(map[string]interface{})
		refTypes[i] = rm["pluginRef"].(string)
	}
	g.Expect(refTypes).To(ContainElement("prefix-cache-scorer"))
	g.Expect(refTypes).NotTo(ContainElement("precise-prefix-cache-scorer"))
}

func TestWithTokenProducerPlugin_MigratesOldTokenProcessorConfig(t *testing.T) {
	g := NewGomegaWithT(t)

	configYAML := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 32
        hashSeed: "42"
      tokenizersPoolConfig:
        modelName: "old-model"
        uds:
          socketFile: /old/path/tokenizer.socket
      kvBlockIndexConfig:
        enableMetrics: true
    kvEventsConfig:
      discoverPods: false
      zmqPort: 5556
`

	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(configYAML), &u.Object); err != nil {
		t.Fatalf("failed to unmarshal test config YAML: %v", err)
	}

	endpointURL := "http://my-tokenizer.ns.svc.cluster.local:8000"
	fn := WithTokenProducerPlugin(endpointURL)
	err := fn(context.Background(), u)
	g.Expect(err).ToNot(HaveOccurred())

	plugins, found, err := unstructured.NestedSlice(u.Object, "plugins")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(plugins).To(HaveLen(3))

	// Verify precise-prefix-cache-scorer is gone
	for _, p := range plugins {
		pm := p.(map[string]interface{})
		g.Expect(pm["type"]).NotTo(Equal("precise-prefix-cache-scorer"))
	}

	// Verify producer has tokenProcessorConfig migrated from indexerConfig
	producer := plugins[1].(map[string]interface{})
	blockSize, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "tokenProcessorConfig", "blockSize")
	g.Expect(f).To(BeTrue())
	g.Expect(blockSize).To(BeEquivalentTo(32))
	hashSeed, f, _ := unstructured.NestedString(producer, "parameters", "tokenProcessorConfig", "hashSeed")
	g.Expect(f).To(BeTrue())
	g.Expect(hashSeed).To(Equal("42"))

	// indexerConfig should NOT have tokenProcessorConfig or tokenizersPoolConfig
	_, tpcFound, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "indexerConfig", "tokenProcessorConfig")
	g.Expect(tpcFound).To(BeFalse(), "tokenProcessorConfig should be removed from indexerConfig")
	_, tokPoolFound, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "indexerConfig", "tokenizersPoolConfig")
	g.Expect(tokPoolFound).To(BeFalse(), "tokenizersPoolConfig should be removed")

	// indexerConfig should still have kvBlockIndexConfig
	enableMetrics, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "indexerConfig", "kvBlockIndexConfig", "enableMetrics")
	g.Expect(f).To(BeTrue())
	g.Expect(enableMetrics).To(BeTrue())

	// kvEventsConfig should be preserved from user config
	discoverPods, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "kvEventsConfig", "discoverPods")
	g.Expect(f).To(BeTrue())
	g.Expect(discoverPods).To(BeFalse())
	zmqPort, f, _ := unstructured.NestedFieldNoCopy(producer, "parameters", "kvEventsConfig", "zmqPort")
	g.Expect(f).To(BeTrue())
	g.Expect(zmqPort).To(BeEquivalentTo(5556))
}

func TestWithTokenProducerPlugin_NoPrecisePrefixPlugin(t *testing.T) {
	g := NewGomegaWithT(t)

	configYAML := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
`

	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(configYAML), &u.Object); err != nil {
		t.Fatalf("failed to unmarshal test config YAML: %v", err)
	}

	fn := WithTokenProducerPlugin("http://tokenizer:8000")
	err := fn(context.Background(), u)
	g.Expect(err).ToNot(HaveOccurred())

	plugins, _, _ := unstructured.NestedSlice(u.Object, "plugins")
	g.Expect(plugins).To(HaveLen(4))
	for _, plugin := range plugins {
		pluginMap := plugin.(map[string]interface{})
		g.Expect(pluginMap["type"]).NotTo(Equal("token-producer"), "no token-producer should be added")
		g.Expect(pluginMap["type"]).NotTo(Equal("precise-prefix-cache-producer"), "no producer should be added")
	}
}

func TestStripTokenizerSidecar(t *testing.T) {
	g := NewGomegaWithT(t)

	d := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "tls-certs", MountPath: "/tls"},
								{Name: "tokenizer-uds", MountPath: "/tmp/tokenizer"},
							},
						},
						{
							Name: tokenizerContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "tokenizer-uds", MountPath: "/tmp/tokenizer"},
								{Name: "tokenizer-tmp", MountPath: "/tmp"},
								{Name: "tokenizer-cache", MountPath: "/.cache"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "tls-certs"},
						{Name: "tokenizer-uds"},
						{Name: "tokenizer-tmp"},
						{Name: "tokenizer-cache"},
					},
				},
			},
		},
	}

	stripTokenizerSidecar(d)

	g.Expect(d.Spec.Template.Spec.Containers).To(HaveLen(1))
	g.Expect(d.Spec.Template.Spec.Containers[0].Name).To(Equal("main"))

	g.Expect(d.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(d.Spec.Template.Spec.Volumes[0].Name).To(Equal("tls-certs"))

	g.Expect(d.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(d.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("tls-certs"))
}

func TestStripTokenizerSidecar_NoTokenizer(t *testing.T) {
	g := NewGomegaWithT(t)

	d := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
					Volumes:    []corev1.Volume{{Name: "tls-certs"}},
				},
			},
		},
	}

	stripTokenizerSidecar(d)
	g.Expect(d.Spec.Template.Spec.Containers).To(HaveLen(1))
	g.Expect(d.Spec.Template.Spec.Volumes).To(HaveLen(1))
}
