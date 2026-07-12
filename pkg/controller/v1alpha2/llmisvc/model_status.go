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
	"sort"

	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func observeModelStatus(llmSvc *v1alpha2.LLMInferenceService, model *v1alpha2.LLMModelSpec) {
	status := &v1alpha2.ObservedModelStatus{
		Name: ptr.Deref(model.Name, llmSvc.Name),
	}

	if model.LoRA != nil {
		for _, adapter := range model.LoRA.Adapters {
			// Webhook validation requires adapter names; guard is defensive against direct API writes.
			if adapter.Name != nil {
				status.Adapters = append(status.Adapters, *adapter.Name)
			}
		}
		sort.Strings(status.Adapters)
	}

	llmSvc.Status.Model = status
}
