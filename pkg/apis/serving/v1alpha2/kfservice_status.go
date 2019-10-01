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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
)

// ConditionType represents a Service condition value
const (
	// RoutesReady is set when network configuration has completed.
	RoutesReady apis.ConditionType = "RoutesReady"
	// PredictRoutesReady is set when network configuration has completed for predict endpoint verb.
	PredictRoutesReady apis.ConditionType = "PredictRoutesReady"
	// ExplainRoutesReady is set when network configuration has completed for explain endpoint verb.
	ExplainRoutesReady apis.ConditionType = "ExplainRoutesReady"
	// DefaultPredictorReady is set when default predictor has reported readiness.
	DefaultPredictorReady apis.ConditionType = "DefaultPredictorReady"
	// CanaryPredictorReady is set when canary predictor has reported readiness.
	CanaryPredictorReady apis.ConditionType = "CanaryPredictorReady"
	// DefaultExplainerReady is set when default explainer has reported readiness.
	DefaultExplainerReady apis.ConditionType = "DefaultExplainerReady"
	// CanaryExplainerReady is set when canary explainer has reported readiness.
	CanaryExplainerReady apis.ConditionType = "CanaryExplainerReady"
	// DefaultTransformerReady is set when default transformer has reported readiness.
	DefaultTransformerReady apis.ConditionType = "DefaultTransformerReady"
	// CanaryTransformerReady is set when canary transformer has reported readiness.
	CanaryTransformerReady apis.ConditionType = "CanaryTransformerReady"
)

var defaultConditionsMap = map[constants.KFComponent]apis.ConditionType{
	constants.Predictor:   DefaultPredictorReady,
	constants.Explainer:   DefaultExplainerReady,
	constants.Transformer: DefaultTransformerReady,
}

var canaryConditionsMap = map[constants.KFComponent]apis.ConditionType{
	constants.Predictor:   CanaryPredictorReady,
	constants.Explainer:   CanaryExplainerReady,
	constants.Transformer: CanaryTransformerReady,
}

// KFService Ready condition is depending on default predictor and route readiness condition
// canary readiness condition only present when canary is used and currently does
// not affect KFService readiness condition.
var conditionSet = apis.NewLivingConditionSet(
	DefaultPredictorReady,
	RoutesReady,
)

var _ apis.ConditionsAccessor = (*KFServiceStatus)(nil)

func (ss *KFServiceStatus) InitializeConditions() {
	conditionSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *KFServiceStatus) IsReady() bool {
	return conditionSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *KFServiceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return conditionSet.Manage(ss).GetCondition(t)
}

// PropagateDefaultStatus propagates the status for the default spec
func (ss *KFServiceStatus) PropagateDefaultStatus(endpoint constants.KFComponent, defaultStatus *knservingv1alpha1.ServiceStatus) {
	if ss.Default == nil {
		emptyStatusMap := make(EndpointStatusMap)
		ss.Default = &emptyStatusMap
	}
	conditionType := defaultConditionsMap[endpoint]
	statusSpec, ok := (*ss.Default)[endpoint]
	if !ok {
		statusSpec = &StatusConfigurationSpec{}
		(*ss.Default)[endpoint] = statusSpec
	}
	ss.propagateStatus(statusSpec, conditionType, defaultStatus)
}

// PropagateCanaryStatus propagates the status for the canary spec
func (ss *KFServiceStatus) PropagateCanaryStatus(endpoint constants.KFComponent, canaryStatus *knservingv1alpha1.ServiceStatus) {

	conditionType := canaryConditionsMap[endpoint]

	// reset status if canaryServiceStatus is nil
	if canaryStatus == nil {
		emptyStatusMap := make(EndpointStatusMap)
		ss.Canary = &emptyStatusMap
		conditionSet.Manage(ss).ClearCondition(conditionType)
		return
	}

	if ss.Canary == nil {
		emptyStatusMap := make(EndpointStatusMap)
		ss.Canary = &emptyStatusMap
	}

	statusSpec, ok := (*ss.Canary)[endpoint]
	if !ok {
		statusSpec = &StatusConfigurationSpec{}
		(*ss.Canary)[endpoint] = statusSpec
	}

	ss.propagateStatus(statusSpec, conditionType, canaryStatus)
}

func (ss *KFServiceStatus) propagateStatus(statusSpec *StatusConfigurationSpec, conditionType apis.ConditionType, serviceStatus *knservingv1alpha1.ServiceStatus) {
	statusSpec.Name = serviceStatus.LatestCreatedRevisionName
	if serviceStatus.URL != nil {
		statusSpec.Hostname = serviceStatus.URL.Host
	}

	serviceCondition := serviceStatus.GetCondition(knservingv1alpha1.ServiceConditionReady)
	switch {
	case serviceCondition == nil:
	case serviceCondition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(conditionType, serviceCondition.Reason, serviceCondition.Message)
	case serviceCondition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(conditionType)
	case serviceCondition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(conditionType, serviceCondition.Reason, serviceCondition.Message)
	}
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *KFServiceStatus) PropagateRouteStatus(rs *knservingv1alpha1.RouteStatus) {
	ss.URL = rs.URL.String()

	propagateRouteStatus(rs, ss.Default)
	propagateRouteStatus(rs, ss.Canary)

	rc := rs.GetCondition(knservingv1alpha1.RouteConditionReady)

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

func propagateRouteStatus(rs *knservingv1alpha1.RouteStatus, endpointStatusMap *EndpointStatusMap) {
	for _, traffic := range rs.Traffic {
		for _, endpoint := range *endpointStatusMap {
			if endpoint.Name == traffic.RevisionName {
				endpoint.Traffic = traffic.Percent
				if traffic.URL != nil {
					endpoint.Hostname = traffic.URL.Host
				}
			}
		}
	}
}
