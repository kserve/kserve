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

// PropagateDefaultConfigurationStatus propagates the default Configuration status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateDefaultConfigurationStatus(dcs *knservingv1alpha1.ConfigurationStatus) {
	ss.Default.Name = dcs.LatestCreatedRevisionName
	//TODO @yuzisun populate configuration status conditions
}

// PropagateCanaryConfigurationStatus propagates the canary Configuration status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateCanaryConfigurationStatus(ccs *knservingv1alpha1.ConfigurationStatus) {
	ss.Canary.Name = ccs.LatestCreatedRevisionName
	//TODO @yuzisun populate configuration status conditions
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *KFServiceStatus) PropagateRouteStatus(rs *knservingv1alpha1.RouteStatus) {
	if rs.Address != nil {
		ss.URI.Internal = rs.Address.Hostname
	}

	for _, traffic := range rs.Traffic {
		switch traffic.RevisionName {
		case ss.Default.Name:
			ss.Default.Traffic = traffic.Percent
		case ss.Canary.Name:
			ss.Canary.Traffic = traffic.Percent
		default:
		}
	}

	//TODO @yuzisun populate route status conditions
}
