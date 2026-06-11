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

// Top-level conditions. Ready aggregates WorkloadsReady and RouterReady via the
// Knative LivingConditionSet. PresetsCombined is an independent gate that blocks
// reconciliation when False but is not part of the Ready rollup.
const (
	// PresetsCombined is True when all referenced LLMInferenceServiceConfig resources
	// have been found and merged successfully. False with reason ConfigNotFound when a
	// referenced config does not exist, or CombineBaseError on merge failure.
	// Set by the config reconciler (config_merge.go). Always present.
	// Not part of the Ready rollup - blocks reconciliation instead.
	PresetsCombined apis.ConditionType = "PresetsCombined"

	// WorkloadReady is True when all workload sub-conditions (MainWorkloadReady,
	// WorkerWorkloadReady, PrefillWorkloadReady, PrefillWorkerWorkloadReady,
	// ScalingReady, PrefillScalingReady) that are present are True.
	// Aggregated by DetermineWorkloadReadiness. Always present.
	WorkloadReady apis.ConditionType = "WorkloadsReady"

	// RouterReady is True when all router sub-conditions (GatewaysReady,
	// HTTPRoutesReady, InferencePoolReady, SchedulerWorkloadReady) that are
	// present are True. Aggregated by DetermineRouterReadiness. Always present.
	RouterReady apis.ConditionType = "RouterReady"
)

// Workload sub-conditions rolled up into WorkloadsReady.
const (
	// MainWorkloadReady is True when the primary model-serving Deployment has
	// reached its desired replica count and all pods are passing readiness probes.
	// Set by the workload reconciler. Only present in single-node mode; cleared
	// in multi-node mode where WorkerWorkloadReady tracks the primary workload.
	MainWorkloadReady apis.ConditionType = "MainWorkloadReady"

	// WorkerWorkloadReady is True when the LeaderWorkerSet workload is available
	// (all groups ready). Set by the workload reconciler.
	// Only present in multi-node mode; in this mode it replaces MainWorkloadReady
	// as the primary workload condition.
	WorkerWorkloadReady apis.ConditionType = "WorkerWorkloadReady"

	// PrefillWorkloadReady is True when the prefill-phase workload is ready.
	// Set by the workload reconciler. Only present for prefill/decode (P/D)
	// disaggregated serving topologies.
	PrefillWorkloadReady apis.ConditionType = "PrefillWorkloadReady"

	// PrefillWorkerWorkloadReady is True when the multi-node worker pods for
	// the prefill workload are ready. Set by the workload reconciler.
	// Only present for multi-node P/D disaggregated serving topologies.
	PrefillWorkerWorkloadReady apis.ConditionType = "PrefillWorkerWorkloadReady"

	// ScalingReady is True when the autoscaler for the primary workload is
	// configured and operational. Set by the scaling reconciler.
	// Only present when autoscaling is configured; cleared (unset) otherwise,
	// so it does not block WorkloadsReady.
	ScalingReady apis.ConditionType = "ScalingReady"

	// PrefillScalingReady is True when the autoscaler for the prefill workload
	// is configured and operational. Set by the scaling reconciler.
	// Only present when autoscaling is configured for the prefill workload in
	// P/D disaggregated serving topologies; cleared (unset) otherwise.
	PrefillScalingReady apis.ConditionType = "PrefillScalingReady"
)

// Router sub-conditions rolled up into RouterReady.
const (
	// SchedulerWorkloadReady is True when the Endpoint Picker (EPP) scheduler
	// Deployment has reached its desired replica count. Set by the scheduler
	// reconciler. Only present when the scheduler is enabled.
	SchedulerWorkloadReady apis.ConditionType = "SchedulerWorkloadReady"
)

const (
	// GatewaysReady is True when all referenced Gateway resources exist and
	// report ready status. Set by the router reconciler.
	// Only present when gateway refs are configured; cleared otherwise.
	GatewaysReady apis.ConditionType = "GatewaysReady"

	// HTTPRoutesReady is True when all HTTPRoute resources have been created
	// and accepted by their parent Gateways. Set by the router reconciler.
	// Only present when HTTP route configuration exists; cleared otherwise.
	HTTPRoutesReady apis.ConditionType = "HTTPRoutesReady"

	// InferencePoolReady is True when the InferencePool resource has been
	// created and is ready. Set by the router reconciler.
	// Only present when the scheduler is enabled.
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
