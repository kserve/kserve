/*
Copyright 2024 The KServe Authors.

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

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestLocalModelNamespaceCache_MatchStorageURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	spec := LocalModelNamespaceCacheSpec{
		SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
		ModelSize:      resource.MustParse("10Gi"),
		NodeGroups:     []string{"gpu"},
	}
	cases := []struct {
		name       string
		spec       LocalModelNamespaceCacheSpec
		storageUri string
		isMatch    bool
	}{
		{
			name:       "exact match",
			spec:       spec,
			storageUri: "hf://meta-llama/meta-llama-3-8b-instruct",
			isMatch:    true,
		},
		{
			name:       "subpath match",
			spec:       spec,
			storageUri: "hf://meta-llama/meta-llama-3-8b-instruct/model.safetensors",
			isMatch:    true,
		},
		{
			name:       "different model does not match",
			spec:       spec,
			storageUri: "hf://meta-llama/meta-llama-3-70b-instruct",
			isMatch:    false,
		},
		{
			name:       "prefix match but different model does not match",
			spec:       spec,
			storageUri: "hf://meta-llama/meta-llama-3-8b-instruct2",
			isMatch:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := tc.spec.MatchStorageURI(tc.storageUri)
			g.Expect(match).To(gomega.Equal(tc.isMatch))
		})
	}
}

func TestLocalModelInfo_GetStatusKey(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name      string
		modelInfo LocalModelInfo
		expected  string
	}{
		{
			name: "cluster-scoped model (empty namespace)",
			modelInfo: LocalModelInfo{
				ModelName:      "llama-cluster",
				SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
				Namespace:      "",
			},
			expected: "llama-cluster",
		},
		{
			name: "namespace-scoped model",
			modelInfo: LocalModelInfo{
				ModelName:      "llama-ns",
				SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
				Namespace:      "ns-a",
			},
			expected: "ns-a/llama-ns",
		},
		{
			name: "namespace-scoped model with different namespace",
			modelInfo: LocalModelInfo{
				ModelName:      "mistral",
				SourceModelUri: "hf://mistral/mistral-7b-instruct",
				Namespace:      "ns-b",
			},
			expected: "ns-b/mistral",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statusKey := tc.modelInfo.GetStatusKey()
			g.Expect(statusKey).To(gomega.Equal(tc.expected))
		})
	}
}

func TestLocalModelInfo_GetStatusKey_UniqueKeys(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test that models with same name but different namespaces have different keys
	clusterModel := LocalModelInfo{
		ModelName:      "llama",
		SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
		Namespace:      "",
	}
	nsModelA := LocalModelInfo{
		ModelName:      "llama",
		SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
		Namespace:      "ns-a",
	}
	nsModelB := LocalModelInfo{
		ModelName:      "llama",
		SourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct",
		Namespace:      "ns-b",
	}

	clusterKey := clusterModel.GetStatusKey()
	nsKeyA := nsModelA.GetStatusKey()
	nsKeyB := nsModelB.GetStatusKey()

	// All three should be different
	g.Expect(clusterKey).NotTo(gomega.Equal(nsKeyA))
	g.Expect(clusterKey).NotTo(gomega.Equal(nsKeyB))
	g.Expect(nsKeyA).NotTo(gomega.Equal(nsKeyB))
}
