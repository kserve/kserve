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

package pod

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/kserve/kserve/pkg/constants"
)

// TestPodMutatorWebhookCoversLLMInferenceServicePods guards the objectSelector
// of the /mutate-pods webhook entries. LLMInferenceService workload pods do
// not carry the serving.kserve.io/inferenceservice label the first entry
// selects; without a second entry selecting
// app.kubernetes.io/part-of: llminferenceservice they never reach the pod
// mutator and the CacheRequired import-record gate is bypassed. Selector
// expressions within one entry are AND-ed, so both label sets cannot be
// covered by a single objectSelector.
func TestPodMutatorWebhookCoversLLMInferenceServicePods(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	manifestPath := filepath.Join(filepath.Dir(filename), "../../../..", "config", "webhook", "manifests.yaml")
	data, err := os.ReadFile(manifestPath) // #nosec G304 -- the path is resolved from this test source file
	require.NoError(t, err)

	type objectSelector struct {
		MatchLabels      map[string]string `json:"matchLabels"`
		MatchExpressions []struct {
			Key      string   `json:"key"`
			Operator string   `json:"operator"`
			Values   []string `json:"values"`
		} `json:"matchExpressions"`
	}
	type webhook struct {
		Name         string `json:"name"`
		ClientConfig struct {
			Service struct {
				Path string `json:"path"`
			} `json:"service"`
		} `json:"clientConfig"`
		ObjectSelector *objectSelector `json:"objectSelector"`
	}
	type manifest struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Webhooks []webhook `json:"webhooks"`
	}

	var isvcSelected, llmSelected bool
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
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
			if webhook.ClientConfig.Service.Path != "/mutate-pods" {
				continue
			}
			require.NotNil(t, webhook.ObjectSelector, "pod mutator entry %s must select its pods", webhook.Name)
			if len(webhook.ObjectSelector.MatchExpressions) == 1 &&
				webhook.ObjectSelector.MatchExpressions[0].Key == constants.InferenceServicePodLabelKey &&
				webhook.ObjectSelector.MatchExpressions[0].Operator == "Exists" {
				isvcSelected = true
			}
			if webhook.ObjectSelector.MatchLabels[constants.KubernetesPartOfLabelKey] == constants.LLMInferenceServicePartOfValue {
				llmSelected = true
			}
		}
	}
	require.True(t, isvcSelected, "no /mutate-pods entry selects InferenceService pods by %s", constants.InferenceServicePodLabelKey)
	require.True(t, llmSelected, "no /mutate-pods entry selects LLMInferenceService pods by %s=%s",
		constants.KubernetesPartOfLabelKey, constants.LLMInferenceServicePartOfValue)
}
