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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	predictedLatencyProducerPlugin = "predicted-latency-producer"
)

// hasLatencyProducerInSpec checks the LLMInferenceService spec's inline config.
// NOTE: This only checks Config.Inline, not Config.Ref (ConfigMap-based config).
// The ConfigMap Ref is resolved later in expectedSchedulerDeployment, after
// combineBaseRefsConfig has already run. If the plugin is specified via Ref,
// the well-known config will not be auto-injected. This is a known limitation;
// all llm-d guides and examples use Config.Inline.
func hasLatencyProducerInSpec(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Config == nil || spec.Router.Scheduler.Config.Inline == nil {
		return false
	}
	u := unstructured.Unstructured{}
	if err := yaml.Unmarshal(spec.Router.Scheduler.Config.Inline.Raw, &u.Object); err != nil {
		return false
	}
	return hasPluginType(u.Object, predictedLatencyProducerPlugin)
}

func hasPluginType(obj map[string]interface{}, pluginType string) bool {
	val, _, err := unstructured.NestedFieldNoCopy(obj, "plugins")
	if err != nil {
		return false
	}
	plugins, _ := val.([]interface{})
	for _, plugin := range plugins {
		if pm, ok := plugin.(map[string]interface{}); ok && pm["type"] == pluginType {
			return true
		}
	}
	return false
}
