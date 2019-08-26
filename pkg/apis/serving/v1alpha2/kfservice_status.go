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
	"k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
)

// ConditionType represents a Service condition value
const (
	// RoutesReady is set when network configuration has completed.
	RoutesReady apis.ConditionType = "RoutesReady"
	// DefaultPredictorReady is set when default predictor has reported readiness.
	DefaultPredictorReady apis.ConditionType = "DefaultPredictorReady"
	// CanaryPredictorReady is set when canary predictor has reported readiness.
	CanaryPredictorReady apis.ConditionType = "CanaryPredictorReady"
)

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

// PropagateDefaultPredictorStatus propagates the default predictor status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateDefaultPredictorStatus(defaultStatus *knservingv1alpha1.ServiceStatus) {
	ss.Default.Name = defaultStatus.LatestCreatedRevisionName
	serviceCondition := defaultStatus.GetCondition(knservingv1alpha1.ServiceConditionReady)

	switch {
	case serviceCondition == nil:
	case serviceCondition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(DefaultPredictorReady, serviceCondition.Reason, serviceCondition.Message)
	case serviceCondition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(DefaultPredictorReady)
	case serviceCondition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(DefaultPredictorReady, serviceCondition.Reason, serviceCondition.Message)
	}
}

// PropagateCanaryPredictorStatus propagates the canary predictor status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateCanaryPredictorStatus(canaryStatus *knservingv1alpha1.ServiceStatus) {
	// reset status if canarServiceStatus is nil
	if canaryStatus == nil {
		ss.Canary = StatusConfigurationSpec{}
		conditionSet.Manage(ss).ClearCondition(CanaryPredictorReady)
		return
	}
	ss.Canary.Name = canaryStatus.LatestCreatedRevisionName
	serviceCondition := canaryStatus.GetCondition(knservingv1alpha1.ServiceConditionReady)

	switch {
	case serviceCondition == nil:
	case serviceCondition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(CanaryPredictorReady, serviceCondition.Reason, serviceCondition.Message)
	case serviceCondition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(CanaryPredictorReady)
	case serviceCondition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(CanaryPredictorReady, serviceCondition.Reason, serviceCondition.Message)
	}
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *KFServiceStatus) PropagateRouteStatus(rs *knservingv1alpha1.RouteStatus) {
	ss.URL = rs.URL.String()

	for _, traffic := range rs.Traffic {
		switch traffic.RevisionName {
		case ss.Default.Name:
			ss.Default.Traffic = traffic.Percent
		case ss.Canary.Name:
			ss.Canary.Traffic = traffic.Percent
		default:
		}
	}

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
