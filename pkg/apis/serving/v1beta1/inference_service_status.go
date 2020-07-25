/*
Copyright 2020 kubeflow.org.

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

package v1beta1

import (
	"k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// InferenceServiceStatus defines the observed state of inferenceservice
type InferenceServiceStatus struct {
	duckv1.Status `json:",inline"`
	// Addressable endpoint for the inferenceservice
	Address *duckv1.Addressable `json:"address,omitempty"`
	// Statuses for the components of the inferenceservice
	Components map[ComponentType]ComponentStatusSpec `json:"components,omitempty"`
}

// ComponentStatusSpec describes the state of the component
type ComponentStatusSpec struct {
	// Latest revision name that is in ready state
	Name string `json:"name,omitempty"`
	// Addressable endpoint for the inferenceservice
	Address *duckv1.Addressable `json:"address,omitempty"`
}

// ComponentType contains the different types of components of the service
type ComponentType string

// ComponentType Enum
const (
	PredictorComponent   ComponentType = "predictor"
	ExplainerComponent   ComponentType = "explainer"
	TransformerComponent ComponentType = "transformer"
)

// ConditionType represents a Service condition value
const (
	// RoutesReady is set when network configuration has completed.
	RoutesReady apis.ConditionType = "RoutesReady"
	// DefaultPredictorReady is set when default predictor has reported readiness.
	PredictorReady apis.ConditionType = "PredictorReady"
	// DefaultTransformerReady is set when default transformer has reported readiness.
	TransformerReady apis.ConditionType = "TransformerReady"
	// DefaultExplainerReady is set when default explainer has reported readiness.
	ExplainerReady apis.ConditionType = "ExplainerReady"
)

var defaultConditionsMap = map[ComponentType]apis.ConditionType{
	PredictorComponent:   PredictorReady,
	ExplainerComponent:   ExplainerReady,
	TransformerComponent: TransformerReady,
}

// InferenceService Ready condition is depending on default predictor and route readiness condition
// canary readiness condition only present when canary is used and currently does
// not affect InferenceService readiness condition.
var conditionSet = apis.NewLivingConditionSet(
	PredictorReady,
	RoutesReady,
)

var _ apis.ConditionsAccessor = (*InferenceServiceStatus)(nil)

func (ss *InferenceServiceStatus) InitializeConditions() {
	conditionSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *InferenceServiceStatus) IsReady() bool {
	return conditionSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *InferenceServiceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return conditionSet.Manage(ss).GetCondition(t)
}

// InferenceState describes the Readiness of the InferenceService
type InferenceServiceState string

// Different InferenceServiceState an InferenceService may have.
const (
	InferenceServiceReadyState    InferenceServiceState = "InferenceServiceReady"
	InferenceServiceNotReadyState InferenceServiceState = "InferenceServiceNotReady"
)

// PropagateDefaultStatus propagates the status for the default spec
func (ss *InferenceServiceStatus) PropagateDefaultStatus(component ComponentType,
	defaultStatus *knservingv1.ServiceStatus) {

	/*conditionType := defaultConditionsMap[component]
	if defaultStatus == nil {
		conditionSet.Manage(ss).ClearCondition(conditionType)
		delete(ss.Components, component)
		return
	}

	statusSpec, ok := ss.Components[component]
	if !ok {
		statusSpec = ComponentStatusSpec{}
		ss.Components[component] = statusSpec
	}
	ss.propagateStatus(component, false, conditionType, defaultStatus)*/
}

func (ss *InferenceServiceStatus) propagateStatus(component ComponentType, isCanary bool,
	conditionType apis.ConditionType,
	serviceStatus *knservingv1.ServiceStatus) {
	statusSpec := ComponentStatusSpec{}
	statusSpec.Name = serviceStatus.LatestCreatedRevisionName
	serviceCondition := serviceStatus.GetCondition(knservingv1.ServiceConditionReady)

	switch {
	case serviceCondition == nil:
	case serviceCondition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(conditionType, serviceCondition.Reason, serviceCondition.Message)
	case serviceCondition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(conditionType)
		if serviceStatus.URL != nil {
			statusSpec.Address = serviceStatus.Address
		}
	case serviceCondition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(conditionType, serviceCondition.Reason, serviceCondition.Message)
	}
	ss.Components[component] = statusSpec
}
