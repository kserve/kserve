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

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
)

var routeCondSet = duckv1alpha1.NewLivingConditionSet(
	RouteConditionAllTrafficAssigned,
	RouteConditionIngressReady,
)

func (r *Route) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Route")
}

func (rs *RouteStatus) IsReady() bool {
	return routeCondSet.Manage(rs).IsHappy()
}

func (rs *RouteStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return routeCondSet.Manage(rs).GetCondition(t)
}

func (rs *RouteStatus) InitializeConditions() {
	routeCondSet.Manage(rs).InitializeConditions()
}

// MarkServiceNotOwned changes the IngressReady status to be false with the reason being that
// there is a pre-existing placeholder service with the name we wanted to use.
func (rs *RouteStatus) MarkServiceNotOwned(name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionIngressReady, "NotOwned",
		fmt.Sprintf("There is an existing placeholder Service %q that we do not own.", name))
}

func (rs *RouteStatus) MarkTrafficAssigned() {
	routeCondSet.Manage(rs).MarkTrue(RouteConditionAllTrafficAssigned)
}

func (rs *RouteStatus) MarkUnknownTrafficError(msg string) {
	routeCondSet.Manage(rs).MarkUnknown(RouteConditionAllTrafficAssigned, "Unknown", msg)
}

func (rs *RouteStatus) MarkConfigurationNotReady(name string) {
	routeCondSet.Manage(rs).MarkUnknown(RouteConditionAllTrafficAssigned,
		"RevisionMissing",
		"Configuration %q is waiting for a Revision to become ready.", name)
}

func (rs *RouteStatus) MarkConfigurationFailed(name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionAllTrafficAssigned,
		"RevisionMissing",
		"Configuration %q does not have any ready Revision.", name)
}

func (rs *RouteStatus) MarkRevisionNotReady(name string) {
	routeCondSet.Manage(rs).MarkUnknown(RouteConditionAllTrafficAssigned,
		"RevisionMissing",
		"Revision %q is not yet ready.", name)
}

func (rs *RouteStatus) MarkRevisionFailed(name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionAllTrafficAssigned,
		"RevisionMissing",
		"Revision %q failed to become ready.", name)
}

func (rs *RouteStatus) MarkMissingTrafficTarget(kind, name string) {
	routeCondSet.Manage(rs).MarkFalse(RouteConditionAllTrafficAssigned,
		kind+"Missing",
		"%s %q referenced in traffic not found.", kind, name)
}

// PropagateClusterIngressStatus update RouteConditionIngressReady condition
// in RouteStatus according to IngressStatus.
func (rs *RouteStatus) PropagateClusterIngressStatus(cs v1alpha1.IngressStatus) {
	cc := cs.GetCondition(v1alpha1.ClusterIngressConditionReady)
	if cc == nil {
		return
	}
	switch {
	case cc.Status == corev1.ConditionUnknown:
		routeCondSet.Manage(rs).MarkUnknown(RouteConditionIngressReady, cc.Reason, cc.Message)
	case cc.Status == corev1.ConditionTrue:
		routeCondSet.Manage(rs).MarkTrue(RouteConditionIngressReady)
	case cc.Status == corev1.ConditionFalse:
		routeCondSet.Manage(rs).MarkFalse(RouteConditionIngressReady, cc.Reason, cc.Message)
	}
}
