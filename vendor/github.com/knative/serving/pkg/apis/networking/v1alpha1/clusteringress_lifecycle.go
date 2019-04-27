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

var clusterIngressCondSet = apis.NewLivingConditionSet(
	ClusterIngressConditionNetworkConfigured,
	ClusterIngressConditionLoadBalancerReady,
)

func (ci *ClusterIngress) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("ClusterIngress")
}

// IsPublic returns whether the ClusterIngress should be exposed publicly.
func (ci *ClusterIngress) IsPublic() bool {
	return ci.Spec.Visibility == "" || ci.Spec.Visibility == IngressVisibilityExternalIP
}

func (cis *IngressStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return clusterIngressCondSet.Manage(cis).GetCondition(t)
}

func (cis *IngressStatus) InitializeConditions() {
	clusterIngressCondSet.Manage(cis).InitializeConditions()
}

func (cis *IngressStatus) MarkNetworkConfigured() {
	clusterIngressCondSet.Manage(cis).MarkTrue(ClusterIngressConditionNetworkConfigured)
}

// MarkResourceNotOwned changes the "NetworkConfigured" condition to false to reflect that the
// resource of the given kind and name has already been created, and we do not own it.
func (cis *IngressStatus) MarkResourceNotOwned(kind, name string) {
	clusterIngressCondSet.Manage(cis).MarkFalse(ClusterIngressConditionNetworkConfigured, "NotOwned",
		fmt.Sprintf("There is an existing %s %q that we do not own.", kind, name))
}

// MarkLoadBalancerReady marks the Ingress with ClusterIngressConditionLoadBalancerReady,
// and also populate the address of the load balancer.
func (cis *IngressStatus) MarkLoadBalancerReady(lbs []LoadBalancerIngressStatus) {
	cis.LoadBalancer = &LoadBalancerStatus{
		Ingress: []LoadBalancerIngressStatus{},
	}
	cis.LoadBalancer.Ingress = append(cis.LoadBalancer.Ingress, lbs...)
	clusterIngressCondSet.Manage(cis).MarkTrue(ClusterIngressConditionLoadBalancerReady)
}

// IsReady looks at the conditions and if the Status has a condition
// ClusterIngressConditionReady returns true if ConditionStatus is True
func (cis *IngressStatus) IsReady() bool {
	return clusterIngressCondSet.Manage(cis).IsHappy()
}

func (cis *IngressStatus) duck() *duckv1beta1.Status {
	return &cis.Status
}
