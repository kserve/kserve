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
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestSetLocalModelLabel(t *testing.T) {
	gpu1 := "gpu1"
	gpu2 := "gpu2"

	model1 := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model1",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "hf://meta-llama/Llama-3-8b",
			ModelSize:      resource.MustParse("10Gi"),
			NodeGroups:     []string{gpu1, gpu2},
		},
	}

	localModels := &v1alpha1.LocalModelCacheList{
		Items: []v1alpha1.LocalModelCache{model1},
	}

	mustParseURL := func(s string) apis.URL {
		u, err := apis.ParseURL(s)
		if err != nil {
			t.Fatalf("failed to parse URL %q: %v", s, err)
		}
		return *u
	}

	scenarios := map[string]struct {
		llmSvc       v1alpha2.LLMInferenceService
		models       *v1alpha1.LocalModelCacheList
		expectMatch  bool
		matchedModel string
		nodeGroup    string
	}{
		"ExactMatch": {
			llmSvc: v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm",
					Namespace: "default",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: mustParseURL("hf://meta-llama/Llama-3-8b"),
					},
				},
			},
			models:       localModels,
			expectMatch:  true,
			matchedModel: "model1",
			nodeGroup:    gpu1,
		},
		"MatchWithNodeGroupAnnotation": {
			llmSvc: v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-gpu2",
					Namespace: "default",
					Annotations: map[string]string{
						constants.NodeGroupAnnotationKey: gpu2,
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: mustParseURL("hf://meta-llama/Llama-3-8b"),
					},
				},
			},
			models:       localModels,
			expectMatch:  true,
			matchedModel: "model1",
			nodeGroup:    gpu2,
		},
		"NoMatch": {
			llmSvc: v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-nomatch",
					Namespace: "default",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: mustParseURL("hf://other-org/other-model"),
					},
				},
			},
			models:      localModels,
			expectMatch: false,
		},
		"DisableLocalModel": {
			llmSvc: v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-disabled",
					Namespace: "default",
					Annotations: map[string]string{
						constants.DisableLocalModelKey: "true",
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: mustParseURL("hf://meta-llama/Llama-3-8b"),
					},
				},
			},
			// When disabled, Default() skips model fetching — both lists stay nil
			models:      nil,
			expectMatch: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			llmSvc := scenario.llmSvc.DeepCopy()

			SetLocalModelLabel(llmSvc, scenario.models, nil)

			if scenario.expectMatch {
				g.Expect(llmSvc.Labels).NotTo(gomega.BeNil())
				g.Expect(llmSvc.Labels[constants.LocalModelLabel]).To(gomega.Equal(scenario.matchedModel))
				g.Expect(llmSvc.Annotations).NotTo(gomega.BeNil())
				g.Expect(llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey]).To(gomega.Equal(model1.Spec.SourceModelUri))
				expectedPVC := scenario.matchedModel + "-" + scenario.nodeGroup
				g.Expect(llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey]).To(gomega.Equal(expectedPVC))
			} else if llmSvc.Labels != nil {
				g.Expect(llmSvc.Labels[constants.LocalModelLabel]).To(gomega.BeEmpty())
			}
		})
	}
}

func TestSetLocalModelLabel_NamespaceScopedPrecedence(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mustParseURL := func(s string) apis.URL {
		u, err := apis.ParseURL(s)
		if err != nil {
			t.Fatalf("failed to parse URL %q: %v", s, err)
		}
		return *u
	}

	clusterModel := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "hf://meta-llama/Llama-3-8b",
			ModelSize:      resource.MustParse("10Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}
	nsModel := v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-cache", Namespace: "default"},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://meta-llama/Llama-3-8b",
			ModelSize:      resource.MustParse("10Gi"),
			NodeGroups:     []string{"gpu"},
		},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: mustParseURL("hf://meta-llama/Llama-3-8b"),
			},
		},
	}

	SetLocalModelLabel(llmSvc,
		&v1alpha1.LocalModelCacheList{Items: []v1alpha1.LocalModelCache{clusterModel}},
		&v1alpha1.LocalModelNamespaceCacheList{Items: []v1alpha1.LocalModelNamespaceCache{nsModel}},
	)

	// Namespace-scoped should win
	g.Expect(llmSvc.Labels[constants.LocalModelLabel]).To(gomega.Equal("ns-cache"))
	g.Expect(llmSvc.Labels[constants.LocalModelNamespaceLabel]).To(gomega.Equal("default"))
}

func TestDeleteLocalModelMetadata(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mustParseURL := func(s string) apis.URL {
		u, err := apis.ParseURL(s)
		if err != nil {
			t.Fatalf("failed to parse URL %q: %v", s, err)
		}
		return *u
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cleanup",
			Namespace: "default",
			Labels: map[string]string{
				constants.LocalModelLabel:          "old-cache",
				constants.LocalModelNamespaceLabel: "old-ns",
			},
			Annotations: map[string]string{
				constants.LocalModelSourceUriAnnotationKey: "hf://old-model",
				constants.LocalModelPVCNameAnnotationKey:   "old-pvc",
			},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: mustParseURL("hf://different-org/different-model"),
			},
		},
	}

	// Call with no matching models — should clean up stale metadata
	SetLocalModelLabel(llmSvc, &v1alpha1.LocalModelCacheList{}, nil)

	g.Expect(llmSvc.Labels[constants.LocalModelLabel]).To(gomega.BeEmpty())
	g.Expect(llmSvc.Labels[constants.LocalModelNamespaceLabel]).To(gomega.BeEmpty())
	g.Expect(llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey]).To(gomega.BeEmpty())
	g.Expect(llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey]).To(gomega.BeEmpty())
}
