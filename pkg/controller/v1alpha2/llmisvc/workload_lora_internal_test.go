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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
)

func TestRewriteLoRAAdaptersFromLocalModelCache(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "cached-adapter", uri: "hf://org/adapter", scheme: constants.HfURIPrefix},
		{name: "remote-adapter", uri: "hf://org/remote", scheme: constants.HfURIPrefix},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"cached-adapter":{"cache":"adapter-cache","sourceUri":"hf://org/adapter","pvcName":"adapter-cache-gpu1"}}`,
			},
		},
	}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(llmSvc, adapters)
	require.NoError(t, err)
	require.Len(t, rewritten, 2)

	assert.Equal(t, constants.PvcURIPrefix, rewritten[0].scheme)
	assert.True(t, strings.HasPrefix(rewritten[0].uri, "pvc://adapter-cache-gpu1/models/"))
	assert.Equal(t, constants.HfURIPrefix, rewritten[1].scheme)
	assert.Equal(t, "hf://org/remote", rewritten[1].uri)

	pairs := collectLoRADownloadPairs(rewritten)
	require.Len(t, pairs, 1)
	assert.Equal(t, "hf://org/remote", pairs[0].uri)
}

func TestRewriteLoRAAdaptersFromLocalModelCache_NoAnnotation(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "a", uri: "hf://org/a", scheme: constants.HfURIPrefix},
	}
	llmSvc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "test"}}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(llmSvc, adapters)
	require.NoError(t, err)
	assert.Equal(t, adapters, rewritten)
}

func TestRewriteLoRAAdaptersFromLocalModelCache_Subpath(t *testing.T) {
	t.Parallel()

	adapters := []resolvedLoRAAdapter{
		{name: "my-adapter", uri: "hf://org/model/subdir", scheme: constants.HfURIPrefix},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				constants.LocalModelLoRAAnnotationKey: `{"my-adapter":{"cache":"c","sourceUri":"hf://org/model","pvcName":"c-gpu1"}}`,
			},
		},
	}

	rewritten, err := rewriteLoRAAdaptersFromLocalModelCache(llmSvc, adapters)
	require.NoError(t, err)
	assert.Contains(t, rewritten[0].uri, "/subdir")
	assert.Equal(t, constants.PvcURIPrefix, rewritten[0].scheme)
}

func TestBuildCachedPVCURI_MatchesLocalModelCacheHelper(t *testing.T) {
	t.Parallel()
	got := localmodelcache.BuildCachedPVCURI("hf://org/model", "cache-gpu1", "hf://org/model/extra")
	assert.True(t, strings.HasPrefix(got, "pvc://cache-gpu1/models/"))
	assert.True(t, strings.HasSuffix(got, "/extra"))
}
