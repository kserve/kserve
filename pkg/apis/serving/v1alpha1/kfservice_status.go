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
	"github.com/knative/pkg/apis"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"k8s.io/api/core/v1"
)

// ConditionType represents a Service condition value
const (
	// KFServiceConditionRoutesReady is set when the service's underlying
	// routes have reported readiness.
	KFServiceConditionRoutesReady apis.ConditionType = "RoutesReady"
	// KFServiceConditionDefaultConfigurationsReady is set when the service's underlying
	// default configuration have reported readiness.
	KFServiceConditionDefaultConfigurationsReady apis.ConditionType = "DefaultConfigurationReady"
	// ServiceConditionCanaryConfigurationsReady is set when the service's underlying
	// canary configuration have reported readiness.
	KFServiceConditionCanaryConfigurationsReady apis.ConditionType = "CanaryConfigurationReady"
)

// KFService Ready condition is depending on default configuration and route readiness condition
// canary configuration readiness condition only present when canary is used and currently does
// not affect KFService readiness condition.
var serviceCondSet = apis.NewLivingConditionSet(
	KFServiceConditionDefaultConfigurationsReady,
	KFServiceConditionRoutesReady,
)

var _ apis.ConditionsAccessor = (*KFServiceStatus)(nil)

func (ss *KFServiceStatus) InitializeConditions() {
	serviceCondSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *KFServiceStatus) IsReady() bool {
	return serviceCondSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *KFServiceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return serviceCondSet.Manage(ss).GetCondition(t)
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
		serviceCondSet.Manage(ss).MarkUnknown(KFServiceConditionDefaultConfigurationsReady, cc.Reason, cc.Message)
	case cc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(KFServiceConditionDefaultConfigurationsReady)
	case cc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(KFServiceConditionDefaultConfigurationsReady, cc.Reason, cc.Message)
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
		serviceCondSet.Manage(ss).MarkUnknown(KFServiceConditionCanaryConfigurationsReady, cc.Reason, cc.Message)
	case cc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(KFServiceConditionCanaryConfigurationsReady)
	case cc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(KFServiceConditionCanaryConfigurationsReady, cc.Reason, cc.Message)
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
		serviceCondSet.Manage(ss).MarkUnknown(KFServiceConditionRoutesReady, rc.Reason, rc.Message)
	case rc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(KFServiceConditionRoutesReady)
	case rc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(KFServiceConditionRoutesReady, rc.Reason, rc.Message)
	}
}
