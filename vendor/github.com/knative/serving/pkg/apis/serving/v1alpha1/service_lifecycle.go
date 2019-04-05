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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	authv1 "k8s.io/api/authentication/v1"
)

var serviceCondSet = duckv1alpha1.NewLivingConditionSet(
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
func (ss *ServiceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return serviceCondSet.Manage(ss).GetCondition(t)
}

// InitializeConditions sets the initial values to the conditions.
func (ss *ServiceStatus) InitializeConditions() {
	serviceCondSet.Manage(ss).InitializeConditions()
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

// SetManualStatus updates the service conditions to unknown as the underlying Route
// can have TrafficTargets to Configurations not owned by the service. We do not want to falsely
// report Ready.
func (ss *ServiceStatus) SetManualStatus() {
	const (
		reason  = "Manual"
		message = "Service is set to Manual, and is not managing underlying resources."
	)

	// Clear our fields by creating a new status and copying over only the fields and conditions we want
	newStatus := &ServiceStatus{}
	newStatus.InitializeConditions()
	serviceCondSet.Manage(newStatus).MarkUnknown(ServiceConditionConfigurationsReady, reason, message)
	serviceCondSet.Manage(newStatus).MarkUnknown(ServiceConditionRoutesReady, reason, message)

	newStatus.Address = ss.Address
	newStatus.Domain = ss.Domain
	newStatus.DeprecatedDomainInternal = ss.DeprecatedDomainInternal

	*ss = *newStatus
}

const (
	// CreatorAnnotation is the annotation key to describe the user that
	// created the resource.
	CreatorAnnotation = "serving.knative.dev/creator"
	// UpdaterAnnotation is the annotation key to describe the user that
	// last updated the resource.
	UpdaterAnnotation = "serving.knative.dev/lastModifier"
)

// AnnotateUserInfo satisfay the apis.Annotatable interface, and set the proper annotations
// on the Service resource about the user that performed the action.
func (s *Service) AnnotateUserInfo(ctx context.Context, prev apis.Annotatable, ui *authv1.UserInfo) {
	ans := s.GetAnnotations()
	if ans == nil {
		ans = map[string]string{}
		defer s.SetAnnotations(ans)
	}

	// WebHook makes sure we get the proper type here.
	ps, _ := prev.(*Service)

	// Creation.
	if ps == nil {
		ans[CreatorAnnotation] = ui.Username
		ans[UpdaterAnnotation] = ui.Username
		return
	}

	// Compare the Spec's, we update the `lastModifier` key iff
	// there's a change in the spec.
	if equality.Semantic.DeepEqual(ps.Spec, s.Spec) {
		return
	}
	ans[UpdaterAnnotation] = ui.Username
}
