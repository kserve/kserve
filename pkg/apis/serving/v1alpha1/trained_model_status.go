/*
Copyright 2021 The KServe Authors.

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
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// TrainedModelStatus defines the observed state of TrainedModel
type TrainedModelStatus struct {
	// Conditions for trained model
	duckv1.Status `json:",inline"`
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// For v1: http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}/v1/models/<trainedmodel>:predict
	// For v2: http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}/v2/models/<trainedmodel>/infer
	URL *apis.URL `json:"url,omitempty"`
	// Addressable endpoint for the deployed trained model
	// http://<inferenceservice.metadata.name>/v1/models/<trainedmodel>.metadata.name
	Address *duckv1.Addressable `json:"address,omitempty"`
}

// ConditionType represents a Service condition value
const (
	// InferenceServiceReady is set when inference service reported readiness
	InferenceServiceReady apis.ConditionType = "InferenceServiceReady"
	// MemoryResourceAvailable is set when inference service reported resources availability
	MemoryResourceAvailable apis.ConditionType = "MemoryResourceAvailable"
	// IsMMSPredictor is set when inference service predictor is set to multi-model serving
	IsMMSPredictor apis.ConditionType = "IsMMSPredictor"
)

// TrainedModel Ready condition is depending on inference service readiness condition
// TODO: Similar to above, add the constants here
var conditionSet = apis.NewLivingConditionSet(
	InferenceServiceReady,
	MemoryResourceAvailable,
	IsMMSPredictor,
)

var _ apis.ConditionsAccessor = (*TrainedModelStatus)(nil)

func (ss *TrainedModelStatus) InitializeConditions() {
	conditionSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *TrainedModelStatus) IsReady() bool {
	return conditionSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *TrainedModelStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return conditionSet.Manage(ss).GetCondition(t)
}

// IsConditionReady returns the readiness for a given condition
func (ss *TrainedModelStatus) IsConditionReady(t apis.ConditionType) bool {
	return conditionSet.Manage(ss).GetCondition(t) != nil && conditionSet.Manage(ss).GetCondition(t).Status == corev1.ConditionTrue
}

func (ss *TrainedModelStatus) SetCondition(conditionType apis.ConditionType, condition *apis.Condition) {
	switch {
	case condition == nil:
	case condition.Status == corev1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(conditionType, condition.Reason, condition.Message)
	case condition.Status == corev1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(conditionType)
	case condition.Status == corev1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(conditionType, condition.Reason, condition.Message)
	}
}
