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

package v1alpha2

import (
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// llmInferenceServiceConfigCondSet defines the living condition set for LLMInferenceServiceConfig.
// The Ready condition is the top-level aggregate that reflects the overall state of the config.
var llmInferenceServiceConfigCondSet = apis.NewLivingConditionSet()

func (in *LLMInferenceServiceConfig) GetStatus() *duckv1.Status {
	return &in.Status.Status
}

func (in *LLMInferenceServiceConfig) GetConditionSet() apis.ConditionSet {
	return llmInferenceServiceConfigCondSet
}

func (in *LLMInferenceServiceConfig) MarkReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(apis.ConditionReady)
}

func (in *LLMInferenceServiceConfig) MarkNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(apis.ConditionReady, reason, messageFormat, messageA...)
}
