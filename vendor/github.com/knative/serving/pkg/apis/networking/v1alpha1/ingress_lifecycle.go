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

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ingressCondSet = apis.NewLivingConditionSet(
	IngressConditionNetworkConfigured,
	IngressConditionLoadBalancerReady,
)

// GetGroupVersionKind returns SchemeGroupVersion of an Ingress
func (i *Ingress) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Ingress")
}

// IsPublic returns whether the Ingress should be exposed publicly.
func (i *Ingress) IsPublic() bool {
	return i.Spec.Visibility == "" || i.Spec.Visibility == IngressVisibilityExternalIP
}

// GetCondition returns the current condition of a given condition type
func (is *IngressStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return ingressCondSet.Manage(is).GetCondition(t)
}

// InitializeConditions initializes conditions of an IngressStatus
func (is *IngressStatus) InitializeConditions() {
	ingressCondSet.Manage(is).InitializeConditions()
}

// MarkNetworkConfigured set IngressConditionNetworkConfigured in IngressStatus as true
func (is *IngressStatus) MarkNetworkConfigured() {
	ingressCondSet.Manage(is).MarkTrue(IngressConditionNetworkConfigured)
}

// MarkResourceNotOwned changes the "NetworkConfigured" condition to false to reflect that the
// resource of the given kind and name has already been created, and we do not own it.
func (is *IngressStatus) MarkResourceNotOwned(kind, name string) {
	ingressCondSet.Manage(is).MarkFalse(IngressConditionNetworkConfigured, "NotOwned",
		fmt.Sprintf("There is an existing %s %q that we do not own.", kind, name))
}

// MarkLoadBalancerReady marks the Ingress with IngressConditionLoadBalancerReady,
// and also populate the address of the load balancer.
func (is *IngressStatus) MarkLoadBalancerReady(lbs []LoadBalancerIngressStatus) {
	is.LoadBalancer = &LoadBalancerStatus{
		Ingress: []LoadBalancerIngressStatus{},
	}
	is.LoadBalancer.Ingress = append(is.LoadBalancer.Ingress, lbs...)
	ingressCondSet.Manage(is).MarkTrue(IngressConditionLoadBalancerReady)
}

// IsReady looks at the conditions and if the Status has a condition
// IngressConditionReady returns true if ConditionStatus is True
func (is *IngressStatus) IsReady() bool {
	return ingressCondSet.Manage(is).IsHappy()
}

func (is *IngressStatus) duck() *duckv1beta1.Status {
	return &is.Status
}
