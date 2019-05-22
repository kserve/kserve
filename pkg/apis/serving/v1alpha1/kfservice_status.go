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

package v1alpha1

import (
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"k8s.io/api/core/v1"
)

// ConditionType represents a Service condition value
const (
	// ServiceConditionRoutesReady is set when the service's underlying
	// routes have reported readiness.
	ServiceConditionRoutesReady duckv1alpha1.ConditionType = "RoutesReady"
	// ServiceConditionDefaultConfigurationsReady is set when the service's underlying
	// default configuration have reported readiness.
	ServiceConditionDefaultConfigurationsReady duckv1alpha1.ConditionType = "DefaultConfigurationReady"
	// ServiceConditionCanaryConfigurationsReady is set when the service's underlying
	// canary configuration have reported readiness.
	ServiceConditionCanaryConfigurationsReady duckv1alpha1.ConditionType = "CanaryConfigurationReady"
)

var serviceCondSet = duckv1alpha1.NewLivingConditionSet(
	ServiceConditionDefaultConfigurationsReady,
	ServiceConditionCanaryConfigurationsReady,
	ServiceConditionRoutesReady,
)

var _ duckv1alpha1.ConditionsAccessor = (*KFServiceStatus)(nil)

func (ss *KFServiceStatus) InitializeConditions() {
	serviceCondSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *KFServiceStatus) IsReady() bool {
	return serviceCondSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *KFServiceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return serviceCondSet.Manage(ss).GetCondition(t)
}

// GetConditions returns the Conditions array. This enables generic handling of
// conditions by implementing the duckv1alpha1.Conditions interface.
func (ss *KFServiceStatus) GetConditions() duckv1alpha1.Conditions {
	return ss.Conditions
}

// SetConditions sets the Conditions array. This enables generic handling of
// conditions by implementing the duckv1alpha1.Conditions interface.
func (ss *KFServiceStatus) SetConditions(conditions duckv1alpha1.Conditions) {
	ss.Conditions = conditions
}

// PropagateDefaultConfigurationStatus propagates the default Configuration status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateDefaultConfigurationStatus(dcs *knservingv1alpha1.ConfigurationStatus) {
	ss.Default.Name = dcs.LatestCreatedRevisionName
	cc := dcs.GetCondition(knservingv1alpha1.ConfigurationConditionReady)
	if cc == nil {
		return
	}
	switch {
	case cc.Status == v1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionDefaultConfigurationsReady, cc.Reason, cc.Message)
	case cc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionDefaultConfigurationsReady)
	case cc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionDefaultConfigurationsReady, cc.Reason, cc.Message)
	}
}

// PropagateCanaryConfigurationStatus propagates the canary Configuration status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateCanaryConfigurationStatus(ccs *knservingv1alpha1.ConfigurationStatus) {
	ss.Canary.Name = ccs.LatestCreatedRevisionName
	cc := ccs.GetCondition(knservingv1alpha1.ConfigurationConditionReady)
	if cc == nil {
		return
	}
	switch {
	case cc.Status == v1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionCanaryConfigurationsReady, cc.Reason, cc.Message)
	case cc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionCanaryConfigurationsReady)
	case cc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionCanaryConfigurationsReady, cc.Reason, cc.Message)
	}
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *KFServiceStatus) PropagateRouteStatus(rs *knservingv1alpha1.RouteStatus) {
	ss.URL = rs.Domain

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
	if rc == nil {
		return
	}
	switch {
	case rc.Status == v1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionRoutesReady, rc.Reason, rc.Message)
	case rc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionRoutesReady)
	case rc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionRoutesReady, rc.Reason, rc.Message)
	}
}
