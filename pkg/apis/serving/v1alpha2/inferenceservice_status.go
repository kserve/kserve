/*
Copyright 2019 kubeflow.org.
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
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// ConditionType represents a Service condition value
const (
	// RoutesReady is set when network configuration has completed.
	RoutesReady apis.ConditionType = "RoutesReady"
	// DefaultPredictorReady is set when default predictor has reported readiness.
	DefaultPredictorReady apis.ConditionType = "DefaultPredictorReady"
	// CanaryPredictorReady is set when canary predictor has reported readiness.
	CanaryPredictorReady apis.ConditionType = "CanaryPredictorReady"
	// DefaultTransformerReady is set when default transformer has reported readiness.
	DefaultTransformerReady apis.ConditionType = "DefaultTransformerReady"
	// CanaryTransformerReady is set when canary transformer has reported readiness.
	CanaryTransformerReady apis.ConditionType = "CanaryTransformerReady"

	// DefaultExplainerReady is set when default explainer has reported readiness.
	DefaultExplainerReady apis.ConditionType = "DefaultExplainerReady"
	// CanaryExplainerReady is set when canary explainer has reported readiness.
	CanaryExplainerReady apis.ConditionType = "CanaryExplainerReady"
)

var defaultConditionsMap = map[constants.InferenceServiceComponent]apis.ConditionType{
	constants.Predictor:   DefaultPredictorReady,
	constants.Explainer:   DefaultExplainerReady,
	constants.Transformer: DefaultTransformerReady,
}

var canaryConditionsMap = map[constants.InferenceServiceComponent]apis.ConditionType{
	constants.Predictor:   CanaryPredictorReady,
	constants.Explainer:   CanaryExplainerReady,
	constants.Transformer: CanaryTransformerReady,
}

// InferenceService Ready condition is depending on default predictor and route readiness condition
// canary readiness condition only present when canary is used and currently does
// not affect InferenceService readiness condition.
var conditionSet = apis.NewLivingConditionSet(
	DefaultPredictorReady,
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
func (ss *InferenceServiceStatus) PropagateDefaultStatus(component constants.InferenceServiceComponent,
	defaultStatus *knservingv1.ServiceStatus) {
	if ss.Default == nil {
		emptyStatusMap := make(map[constants.InferenceServiceComponent]StatusConfigurationSpec)
		ss.Default = &emptyStatusMap
	}

	conditionType := defaultConditionsMap[component]
	if defaultStatus == nil {
		conditionSet.Manage(ss).ClearCondition(conditionType)
		delete(*ss.Default, component)
		return
	}

	statusSpec, ok := (*ss.Default)[component]
	if !ok {
		statusSpec = StatusConfigurationSpec{}
		(*ss.Default)[component] = statusSpec
	}
	ss.propagateStatus(component, false, conditionType, defaultStatus)
}

// PropagateCanaryStatus propagates the status for the canary spec
func (ss *InferenceServiceStatus) PropagateCanaryStatus(component constants.InferenceServiceComponent,
	canaryStatus *knservingv1.ServiceStatus) {
	if ss.Canary == nil {
		emptyStatusMap := make(map[constants.InferenceServiceComponent]StatusConfigurationSpec)
		ss.Canary = &emptyStatusMap
	}

	conditionType := canaryConditionsMap[component]
	if canaryStatus == nil {
		conditionSet.Manage(ss).ClearCondition(conditionType)
		delete(*ss.Canary, component)
		return
	}

	statusSpec, ok := (*ss.Canary)[component]
	if !ok {
		statusSpec = StatusConfigurationSpec{}
		(*ss.Canary)[component] = statusSpec
	}
	ss.propagateStatus(component, true, conditionType, canaryStatus)
}

func (ss *InferenceServiceStatus) propagateStatus(component constants.InferenceServiceComponent, isCanary bool,
	conditionType apis.ConditionType,
	serviceStatus *knservingv1.ServiceStatus) {
	statusSpec := StatusConfigurationSpec{}
	statusSpec.Name = serviceStatus.LatestCreatedRevisionName
	serviceCondition := serviceStatus.GetCondition(knservingv1.ServiceConditionReady)

	switch {
	case serviceCondition == nil:
	case serviceCondition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(conditionType, serviceCondition.Reason, serviceCondition.Message)
		statusSpec.Hostname = ""
	case serviceCondition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(conditionType)
		if serviceStatus.URL != nil {
			statusSpec.Hostname = serviceStatus.URL.Host
		}
	case serviceCondition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(conditionType, serviceCondition.Reason, serviceCondition.Message)
		statusSpec.Hostname = ""
	}
	if isCanary {
		(*ss.Canary)[component] = statusSpec
	} else {
		(*ss.Default)[component] = statusSpec
	}
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *InferenceServiceStatus) PropagateRouteStatus(vs *VirtualServiceStatus) {
	ss.URL = vs.URL
	ss.Address = vs.Address
	ss.Traffic = vs.DefaultWeight
	ss.CanaryTraffic = vs.CanaryWeight

	rc := vs.GetCondition(RoutesReady)

	switch {
	case rc == nil:
	case rc.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(RoutesReady, rc.Reason, rc.Message)
	case rc.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(RoutesReady)
	case rc.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(RoutesReady, rc.Reason, rc.Message)
	}
}
