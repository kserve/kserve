/*
Copyright 2019 The Knative Authors.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
)

var serviceCondSet = apis.NewLivingConditionSet(
	ServiceConditionConfigurationsReady,
	ServiceConditionRoutesReady,
)

// GetGroupVersionKind returns the GetGroupVersionKind.
func (s *Service) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Service")
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *ServiceStatus) IsReady() bool {
	return serviceCondSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *ServiceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return serviceCondSet.Manage(ss).GetCondition(t)
}

// InitializeConditions sets the initial values to the conditions.
func (ss *ServiceStatus) InitializeConditions() {
	serviceCondSet.Manage(ss).InitializeConditions()
}

// MarkResourceNotConvertible adds a Warning-severity condition to the resource noting that
// it cannot be converted to a higher version.
func (ss *ServiceStatus) MarkResourceNotConvertible(err *CannotConvertError) {
	serviceCondSet.Manage(ss).SetCondition(apis.Condition{
		Type:     ConditionTypeConvertible,
		Status:   corev1.ConditionFalse,
		Severity: apis.ConditionSeverityWarning,
		Reason:   err.Field,
		Message:  err.Message,
	})
}

// MarkConfigurationNotOwned surfaces a failure via the ConfigurationsReady
// status noting that the Configuration with the name we want has already
// been created and we do not own it.
func (ss *ServiceStatus) MarkConfigurationNotOwned(name string) {
	serviceCondSet.Manage(ss).MarkFalse(ServiceConditionConfigurationsReady, "NotOwned",
		fmt.Sprintf("There is an existing Configuration %q that we do not own.", name))
}

// MarkRouteNotOwned surfaces a failure via the RoutesReady status noting that the Route
// with the name we want has already been created and we do not own it.
func (ss *ServiceStatus) MarkRouteNotOwned(name string) {
	serviceCondSet.Manage(ss).MarkFalse(ServiceConditionRoutesReady, "NotOwned",
		fmt.Sprintf("There is an existing Route %q that we do not own.", name))
}

// MarkConfigurationNotReconciled notes that the Configuration controller has not yet
// caught up to the desired changes we have specified.
func (ss *ServiceStatus) MarkConfigurationNotReconciled() {
	serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionConfigurationsReady,
		"OutOfDate", "The Configuration is still working to reflect the latest desired specification.")
}

// PropagateConfigurationStatus takes the Configuration status and applies its values
// to the Service status.
func (ss *ServiceStatus) PropagateConfigurationStatus(cs *ConfigurationStatus) {
	ss.ConfigurationStatusFields = cs.ConfigurationStatusFields

	cc := cs.GetCondition(ConfigurationConditionReady)
	if cc == nil {
		return
	}
	switch {
	case cc.Status == corev1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionConfigurationsReady, cc.Reason, cc.Message)
	case cc.Status == corev1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionConfigurationsReady)
	case cc.Status == corev1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionConfigurationsReady, cc.Reason, cc.Message)
	}
}

// MarkRevisionNameTaken notes that the Route has not been programmed because the revision name is taken by a
// conflicting revision definition.
func (ss *ServiceStatus) MarkRevisionNameTaken(name string) {
	serviceCondSet.Manage(ss).MarkFalse(ServiceConditionRoutesReady, "RevisionNameTaken",
		"The revision name %q is taken by a conflicting Revision, so traffic will not be migrated", name)
}

const (
	trafficNotMigratedReason  = "TrafficNotMigrated"
	trafficNotMigratedMessage = "Traffic is not yet migrated to the latest revision."

	// LatestTrafficTarget is the named constant of the `latest` traffic target.
	LatestTrafficTarget = "latest"

	// CurrentTrafficTarget is the named constnat of the `current` traffic target.
	CurrentTrafficTarget = "current"

	// CandidateTrafficTarget is the named constnat of the `candidate` traffic target.
	CandidateTrafficTarget = "candidate"
)

// MarkRouteNotYetReady marks the service `RouteReady` condition to the `Unknown` state.
// See: #2430, for details.
func (ss *ServiceStatus) MarkRouteNotYetReady() {
	serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionRoutesReady, trafficNotMigratedReason, trafficNotMigratedMessage)
}

// MarkRouteNotReconciled notes that the Route controller has not yet
// caught up to the desired changes we have specified.
func (ss *ServiceStatus) MarkRouteNotReconciled() {
	serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionRoutesReady,
		"OutOfDate", "The Route is still working to reflect the latest desired specification.")
}

// PropagateRouteStatus propagates route's status to the service's status.
func (ss *ServiceStatus) PropagateRouteStatus(rs *RouteStatus) {
	ss.RouteStatusFields = rs.RouteStatusFields

	rc := rs.GetCondition(RouteConditionReady)
	if rc == nil {
		return
	}
	switch {
	case rc.Status == corev1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionRoutesReady, rc.Reason, rc.Message)
	case rc.Status == corev1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionRoutesReady)
	case rc.Status == corev1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionRoutesReady, rc.Reason, rc.Message)
	}
}

func (ss *ServiceStatus) duck() *duckv1beta1.Status {
	return &ss.Status
}
