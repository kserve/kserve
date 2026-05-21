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

const (
	PresetsCombined apis.ConditionType = "PresetsCombined"
	WorkloadReady   apis.ConditionType = "WorkloadsReady"
	RouterReady     apis.ConditionType = "RouterReady"
)

const (
	MainWorkloadReady          apis.ConditionType = "MainWorkloadReady"
	WorkerWorkloadReady        apis.ConditionType = "WorkerWorkloadReady"
	PrefillWorkloadReady       apis.ConditionType = "PrefillWorkloadReady"
	PrefillWorkerWorkloadReady apis.ConditionType = "PrefillWorkerWorkloadReady"
	ScalingReady               apis.ConditionType = "ScalingReady"
	PrefillScalingReady        apis.ConditionType = "PrefillScalingReady"
)

const (
	SchedulerWorkloadReady apis.ConditionType = "SchedulerWorkloadReady"
)

const (
	GatewaysReady      apis.ConditionType = "GatewaysReady"
	HTTPRoutesReady    apis.ConditionType = "HTTPRoutesReady"
	InferencePoolReady apis.ConditionType = "InferencePoolReady"
)

var llmInferenceServiceCondSet = apis.NewLivingConditionSet(
	WorkloadReady,
	RouterReady,
)

func (in *LLMInferenceService) GetStatus() *duckv1.Status {
	return &in.Status.Status
}

func (in *LLMInferenceService) GetConditionSet() apis.ConditionSet {
	return llmInferenceServiceCondSet
}

func (in *LLMInferenceService) MarkMainWorkloadReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(MainWorkloadReady)
}

func (in *LLMInferenceService) MarkMainWorkloadNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(MainWorkloadReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkMainWorkloadUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(MainWorkloadReady)
}

func (in *LLMInferenceService) MarkWorkerWorkloadReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(WorkerWorkloadReady)
}

func (in *LLMInferenceService) MarkWorkerWorkloadNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(WorkerWorkloadReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkWorkerWorkloadUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(WorkerWorkloadReady)
}

func (in *LLMInferenceService) MarkPrefillWorkloadReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(PrefillWorkloadReady)
}

func (in *LLMInferenceService) MarkPrefillWorkloadNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(PrefillWorkloadReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkPrefillWorkloadUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(PrefillWorkloadReady)
}

func (in *LLMInferenceService) MarkPrefillWorkerWorkloadReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(PrefillWorkerWorkloadReady)
}

func (in *LLMInferenceService) MarkPrefillWorkerWorkloadNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(PrefillWorkerWorkloadReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkPrefillWorkerWorkloadUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(PrefillWorkerWorkloadReady)
}

func (in *LLMInferenceService) MarkScalingReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(ScalingReady)
}

func (in *LLMInferenceService) MarkScalingNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(ScalingReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkScalingUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(ScalingReady)
}

func (in *LLMInferenceService) MarkPrefillScalingReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(PrefillScalingReady)
}

func (in *LLMInferenceService) MarkPrefillScalingNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(PrefillScalingReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkPrefillScalingUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(PrefillScalingReady)
}

// DetermineWorkloadReadiness rolls up sub-conditions into the top-level WorkloadsReady
// condition. Any sub-condition that is False blocks overall readiness.
//
// ScalingReady and PrefillScalingReady are included in the rollup because a misconfigured
// autoscaler (e.g. broken metrics pipeline) should surface as a service-level issue.
// When scaling is not configured, the controller calls MarkScalingUnset / MarkPrefillScalingUnset
// which clears the condition entirely — GetCondition returns nil, and nil entries are
// skipped by the loop below. This means unconfigured scaling never blocks readiness.
func (in *LLMInferenceService) DetermineWorkloadReadiness() {
	subConditions := []*apis.Condition{
		in.GetStatus().GetCondition(MainWorkloadReady),
		in.GetStatus().GetCondition(WorkerWorkloadReady),
		in.GetStatus().GetCondition(PrefillWorkloadReady),
		in.GetStatus().GetCondition(PrefillWorkerWorkloadReady),
		in.GetStatus().GetCondition(ScalingReady),
		in.GetStatus().GetCondition(PrefillScalingReady),
	}

	for _, cond := range subConditions {
		if cond == nil {
			continue
		}
		if cond.IsFalse() {
			in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(WorkloadReady, cond.Reason, cond.Message)
			return
		}
	}
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(WorkloadReady)
}

func (in *LLMInferenceService) MarkPresetsCombinedReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(PresetsCombined)
}

func (in *LLMInferenceService) MarkPresetsCombinedNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(PresetsCombined, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkSchedulerWorkloadReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(SchedulerWorkloadReady)
}

func (in *LLMInferenceService) MarkSchedulerWorkloadNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(SchedulerWorkloadReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkSchedulerWorkloadUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(SchedulerWorkloadReady)
}

func (in *LLMInferenceService) MarkGatewaysReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(GatewaysReady)
}

func (in *LLMInferenceService) MarkGatewaysReadyUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(GatewaysReady)
}

func (in *LLMInferenceService) MarkGatewaysNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(GatewaysReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkHTTPRoutesReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(HTTPRoutesReady)
}

func (in *LLMInferenceService) MarkHTTPRoutesNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(HTTPRoutesReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkHTTPRoutesReadyUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(HTTPRoutesReady)
}

func (in *LLMInferenceService) MarkInferencePoolReady() {
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(InferencePoolReady)
}

func (in *LLMInferenceService) MarkInferencePoolNotReady(reason, messageFormat string, messageA ...interface{}) {
	in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(InferencePoolReady, reason, messageFormat, messageA...)
}

func (in *LLMInferenceService) MarkInferencePoolReadyUnset() {
	_ = in.GetConditionSet().Manage(in.GetStatus()).ClearCondition(InferencePoolReady)
}

func (in *LLMInferenceService) DetermineRouterReadiness() {
	subConditions := []*apis.Condition{
		in.GetStatus().GetCondition(GatewaysReady),
		in.GetStatus().GetCondition(HTTPRoutesReady),
		in.GetStatus().GetCondition(InferencePoolReady),
		in.GetStatus().GetCondition(SchedulerWorkloadReady),
	}

	for _, cond := range subConditions {
		if cond == nil {
			continue
		}
		if cond.IsFalse() {
			in.GetConditionSet().Manage(in.GetStatus()).MarkFalse(RouterReady, cond.Reason, cond.Message)
			return
		}
	}
	in.GetConditionSet().Manage(in.GetStatus()).MarkTrue(RouterReady)
}
