package v1alpha1

import (
	"fmt"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"k8s.io/api/core/v1"
)

// ConditionType represents a Service condition value
const (
	// ServiceConditionReady is set when the service is configured
	// and has available backends ready to receive traffic.
	ServiceConditionReady = duckv1alpha1.ConditionReady
	// ServiceConditionRoutesReady is set when the service's underlying
	// routes have reported readiness.
	ServiceConditionRoutesReady duckv1alpha1.ConditionType = "RoutesReady"
)

var serviceCondSet = duckv1alpha1.NewLivingConditionSet(
	ServiceConditionReady,
	ServiceConditionRoutesReady,
)

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *KFServiceStatus) IsReady() bool {
	return serviceCondSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *KFServiceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return serviceCondSet.Manage(ss).GetCondition(t)
}

// InitializeConditions sets the initial values to the conditions.
func (ss *KFServiceStatus) InitializeConditions() {
	serviceCondSet.Manage(ss).InitializeConditions()
}

// MarkConfigurationNotOwned surfaces a failure via the ConfigurationsReady
// status noting that the Configuration with the name we want has already
// been created and we do not own it.
func (ss *KFServiceStatus) MarkConfigurationNotOwned(name string) {
	serviceCondSet.Manage(ss).MarkFalse(ServiceConditionReady, "NotOwned",
		fmt.Sprintf("There is an existing Service %q that we do not own.", name))
}

// MarkRouteNotOwned surfaces a failure via the RoutesReady status noting that the Route
// with the name we want has already been created and we do not own it.
func (ss *KFServiceStatus) MarkRouteNotOwned(name string) {
	serviceCondSet.Manage(ss).MarkFalse(ServiceConditionRoutesReady, "NotOwned",
		fmt.Sprintf("There is an existing Route %q that we do not own.", name))
}

// PropagateConfigurationStatus takes the Configuration status and applies its values
// to the Service status.
func (ss *KFServiceStatus) PropagateConfigurationStatus(cs *v1alpha1.ServiceStatus) {
	//ss.LatestReadyRevisionName = cs.LatestReadyRevisionName
	//ss.LatestCreatedRevisionName = cs.LatestCreatedRevisionName

	cc := cs.GetCondition(ServiceConditionReady)
	if cc == nil {
		return
	}
	switch {
	case cc.Status == v1.ConditionUnknown:
		serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionReady, cc.Reason, cc.Message)
	case cc.Status == v1.ConditionTrue:
		serviceCondSet.Manage(ss).MarkTrue(ServiceConditionReady)
	case cc.Status == v1.ConditionFalse:
		serviceCondSet.Manage(ss).MarkFalse(ServiceConditionReady, cc.Reason, cc.Message)
	}
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
func (ss *KFServiceStatus) MarkRouteNotYetReady() {
	serviceCondSet.Manage(ss).MarkUnknown(ServiceConditionRoutesReady, trafficNotMigratedReason, trafficNotMigratedMessage)
}

// PropagateRouteStatus propagates route's status to the service's status.
/*
func (ss *KFServiceStatus) PropagateRouteStatus(rs *RouteStatus) {
	ss.Domain = rs.Domain
	ss.DeprecatedDomainInternal = rs.DeprecatedDomainInternal
	ss.Address = rs.Address
	ss.Traffic = rs.Traffic

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
}*/
