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

package llminferenceservice

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
)

func newLLMSvc(modelUri string) *v1alpha2.LLMInferenceService {
	uri, _ := apis.ParseURL(modelUri)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "default",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: *uri,
			},
		},
	}
	return llmSvc
}

func newLLMSvcWithLoRA(modelUri string, adapters ...v1alpha2.LLMModelSpec) *v1alpha2.LLMInferenceService {
	llmSvc := newLLMSvc(modelUri)
	llmSvc.Spec.Model.LoRA = &v1alpha2.LoRASpec{Adapters: adapters}
	return llmSvc
}

const testLoRAAdapterName = "my-adapter"

func newLoRAAdapter(uri string) v1alpha2.LLMModelSpec {
	parsed, _ := apis.ParseURL(uri)
	return v1alpha2.LLMModelSpec{
		Name: ptr.To(testLoRAAdapterName),
		URI:  *parsed,
	}
}

func parseLoRAAnnotation(t *testing.T, llmSvc *v1alpha2.LLMInferenceService) map[string]localmodelcache.LoRACacheEntry {
	t.Helper()
	raw := llmSvc.Annotations[constants.LocalModelLoRAAnnotationKey]
	entries, err := localmodelcache.ParseLoRACacheAnnotation(raw)
	require.NoError(t, err)
	return entries
}

func TestSetLocalModelLabel_ClusterScoped(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.Equal(t, "my-cache", llmSvc.Labels[constants.LocalModelLabel])
	assert.NotContains(t, llmSvc.Labels, constants.LocalModelNamespaceLabel)
	assert.Equal(t, "s3://mybucket/mymodel", llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey])
	assert.Equal(t, "my-cache-gpu1", llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey])
}

func TestSetLocalModelLabel_NamespaceScoped(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ns-cache", Namespace: "default"},
				Spec: v1alpha1.LocalModelNamespaceCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu2"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nsModels)

	// Namespace-scoped takes precedence
	assert.Equal(t, "ns-cache", llmSvc.Labels[constants.LocalModelLabel])
	assert.Equal(t, "default", llmSvc.Labels[constants.LocalModelNamespaceLabel])
	assert.Equal(t, "s3://mybucket/mymodel", llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey])
	assert.Equal(t, "ns-cache-gpu2", llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey])
}

func TestSetLocalModelLabel_NoMatch(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/othermodel")
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
}

func TestSetLocalModelLabel_DisableAnnotation(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	// Pre-set the disable annotation — the Default() method checks this before calling SetLocalModelLabel
	// Here we test that if labels were previously set, they get cleaned up when no match is found
	llmSvc.Labels = map[string]string{
		constants.LocalModelLabel: "old-cache",
	}
	llmSvc.Annotations = map[string]string{
		constants.LocalModelSourceUriAnnotationKey: "old-uri",
		constants.LocalModelPVCNameAnnotationKey:   "old-pvc",
	}

	// No matching models — simulates the disabled case
	SetLocalModelLabel(llmSvc, &v1alpha1.LocalModelCacheList{}, nil)

	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelPVCNameAnnotationKey)
}

func TestDeleteLocalModelMetadata(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	llmSvc.Labels = map[string]string{
		constants.LocalModelLabel:          "my-cache",
		constants.LocalModelNamespaceLabel: "default",
		"other-label":                      "value",
	}
	loraJSON, err := json.Marshal(map[string]localmodelcache.LoRACacheEntry{
		"my-adapter": {Cache: "adapter-cache", PVCName: "adapter-cache-gpu1"},
	})
	require.NoError(t, err)
	llmSvc.Annotations = map[string]string{
		constants.LocalModelSourceUriAnnotationKey: "s3://mybucket/mymodel",
		constants.LocalModelPVCNameAnnotationKey:   "my-cache-gpu1",
		constants.LocalModelLoRAAnnotationKey:      string(loraJSON),
		"other-annotation":                         "value",
	}

	DeleteLocalModelMetadata(llmSvc)

	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	assert.NotContains(t, llmSvc.Labels, constants.LocalModelNamespaceLabel)
	assert.Equal(t, "value", llmSvc.Labels["other-label"])
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelPVCNameAnnotationKey)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelLoRAAnnotationKey)
	assert.Equal(t, "value", llmSvc.Annotations["other-annotation"])
}

func TestSetLocalModelLabel_NodeGroupMatching(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	llmSvc.Annotations = map[string]string{
		constants.NodeGroupAnnotationKey: "gpu2",
	}
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1", "gpu2"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.Equal(t, "my-cache", llmSvc.Labels[constants.LocalModelLabel])
	assert.Equal(t, "my-cache-gpu2", llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey])
}

func TestSetLocalModelLabel_NodeGroupNotMatching(t *testing.T) {
	llmSvc := newLLMSvc("s3://mybucket/mymodel")
	llmSvc.Annotations = map[string]string{
		constants.NodeGroupAnnotationKey: "gpu3",
	}
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/mymodel",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1", "gpu2"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	// Node group doesn't match — no labels set
	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
}

func newLLMSvcV1(modelUri string) *v1alpha1.LLMInferenceService {
	uri, _ := apis.ParseURL(modelUri)
	return &v1alpha1.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-v1",
			Namespace: "default",
		},
		Spec: v1alpha1.LLMInferenceServiceSpec{
			Model: v1alpha1.LLMModelSpec{
				URI: *uri,
			},
		},
	}
}

func newDefaulterForDefaultTests(t *testing.T, localModelEnabled bool, objects ...runtime.Object) *LLMInferenceServiceDefaulter {
	t.Helper()

	scheme := runtime.NewScheme()
	assert.NoError(t, v1alpha1.AddToScheme(scheme))
	assert.NoError(t, v1alpha2.AddToScheme(scheme))

	fakeClient := ctrlfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: map[string]string{
			v1beta1.LocalModelConfigName: `{"enabled": ` + map[bool]string{true: "true", false: "false"}[localModelEnabled] + `}`,
		},
	}
	fakeClientset := k8sfake.NewSimpleClientset(configMap)

	return &LLMInferenceServiceDefaulter{
		Client:    fakeClient,
		Clientset: fakeClientset,
	}
}

func TestDefault_V1Alpha1Object_DoesNotErrorAndSetsMetadata(t *testing.T) {
	model := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://mybucket/mymodel",
			ModelSize:      resource.MustParse("10Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	defaulter := newDefaulterForDefaultTests(t, true, model)
	llmSvc := newLLMSvcV1("s3://mybucket/mymodel")

	err := defaulter.Default(context.Background(), llmSvc)
	assert.NoError(t, err)
	assert.Equal(t, "my-cache", llmSvc.Labels[constants.LocalModelLabel])
	assert.Equal(t, "s3://mybucket/mymodel", llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey])
	assert.Equal(t, "my-cache-gpu1", llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey])
}

func TestSetLocalModelLabel_LoRAAdapter_ClusterScoped(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/base", newLoRAAdapter("hf://org/adapter"))
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "adapter-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	entries := parseLoRAAnnotation(t, llmSvc)
	assert.Equal(t, "adapter-cache", entries["my-adapter"].Cache)
	assert.Equal(t, "hf://org/adapter", entries["my-adapter"].SourceURI)
	assert.Equal(t, "adapter-cache-gpu1", entries["my-adapter"].PVCName)
}

func TestSetLocalModelLabel_LoRAAdapter_NamespaceScoped(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/base", newLoRAAdapter("hf://org/adapter"))
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-adapter-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{
		Items: []v1alpha1.LocalModelNamespaceCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ns-adapter-cache", Namespace: "default"},
				Spec: v1alpha1.LocalModelNamespaceCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu2"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nsModels)

	entries := parseLoRAAnnotation(t, llmSvc)
	assert.Equal(t, "ns-adapter-cache", entries["my-adapter"].Cache)
	assert.Equal(t, "default", entries["my-adapter"].Namespace)
	assert.Equal(t, "ns-adapter-cache-gpu2", entries["my-adapter"].PVCName)
}

func TestSetLocalModelLabel_LoRAAdapter_NoMatch(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/base", newLoRAAdapter("hf://org/other"))
	llmSvc.Annotations = map[string]string{
		constants.LocalModelLoRAAnnotationKey: `{"stale":{"cache":"old"}}`,
	}
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "adapter-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelLoRAAnnotationKey)
}

func TestSetLocalModelLabel_MixedBaseCachedAdapterRemote(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/base", newLoRAAdapter("hf://org/remote-adapter"))
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "base-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "s3://mybucket/base",
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.Equal(t, "base-cache", llmSvc.Labels[constants.LocalModelLabel])
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelLoRAAnnotationKey)
}

func TestSetLocalModelLabel_MixedBaseRemoteAdapterCached(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/remote-base", newLoRAAdapter("hf://org/adapter"))
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "adapter-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	entries := parseLoRAAnnotation(t, llmSvc)
	assert.Equal(t, "adapter-cache", entries["my-adapter"].Cache)
}

func TestSetLocalModelLabel_LoRAAdapter_NodeGroupNotMatching(t *testing.T) {
	llmSvc := newLLMSvcWithLoRA("s3://mybucket/base", newLoRAAdapter("hf://org/adapter"))
	llmSvc.Annotations = map[string]string{
		constants.NodeGroupAnnotationKey: "gpu3",
	}
	models := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "adapter-cache"},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: "hf://org/adapter",
					ModelSize:      resource.MustParse("1Gi"),
					NodeGroups:     []string{"gpu1", "gpu2"},
				},
			},
		},
	}

	SetLocalModelLabel(llmSvc, models, nil)

	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelLoRAAnnotationKey)
}

func TestDefault_V1Alpha1Object_Disabled_CleansStaleMetadata(t *testing.T) {
	defaulter := newDefaulterForDefaultTests(t, true)
	llmSvc := newLLMSvcV1("s3://mybucket/mymodel")
	llmSvc.Annotations = map[string]string{
		constants.DisableLocalModelKey:             "true",
		constants.LocalModelSourceUriAnnotationKey: "s3://mybucket/old",
		constants.LocalModelPVCNameAnnotationKey:   "old-pvc",
	}
	llmSvc.Labels = map[string]string{
		constants.LocalModelLabel:          "old-cache",
		constants.LocalModelNamespaceLabel: "default",
	}

	llmSvc.Annotations[constants.LocalModelLoRAAnnotationKey] = `{"adapter":{"cache":"old"}}`

	err := defaulter.Default(context.Background(), llmSvc)
	assert.NoError(t, err)
	assert.NotContains(t, llmSvc.Labels, constants.LocalModelLabel)
	assert.NotContains(t, llmSvc.Labels, constants.LocalModelNamespaceLabel)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelSourceUriAnnotationKey)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelPVCNameAnnotationKey)
	assert.NotContains(t, llmSvc.Annotations, constants.LocalModelLoRAAnnotationKey)
}
