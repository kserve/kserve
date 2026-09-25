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

package kernelcache

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/kserve/kserve/pkg/constants"
)

func TestKernelCacheWebhookMatchConditionDoesNotFilterSidecarInjection(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	manifestPath := filepath.Join(filepath.Dir(filename), "../../../../config/webhook/localmodel/manifests.yaml")
	data, err := os.ReadFile(manifestPath) // #nosec G304 -- the path is resolved from this test source file
	require.NoError(t, err)

	type manifest struct {
		Kind     string `json:"kind"`
		Webhooks []struct {
			Name            string `json:"name"`
			MatchConditions []struct {
				Expression string `json:"expression"`
			} `json:"matchConditions"`
		} `json:"webhooks"`
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	found := false
	for {
		object := manifest{}
		err := decoder.Decode(&object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if object.Kind != "MutatingWebhookConfiguration" {
			continue
		}
		for _, webhook := range object.Webhooks {
			if webhook.Name != "kernelcache.kserve-webhook-server.pod-mutator" {
				continue
			}
			found = true
			require.NotEmpty(t, webhook.MatchConditions)
			for _, condition := range webhook.MatchConditions {
				require.NotContains(t, condition.Expression, constants.KernelCacheSidecarInjectionAnnotationKey)
				require.True(t, strings.Contains(condition.Expression, constants.InferenceServicePodLabelKey))
			}
		}
	}
	require.True(t, found, "KernelCache Pod mutator webhook was not found")
}
