/*
Copyright 2026 The KServe Authors.

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

package llmisvc

import (
	"context"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// reconcileTerminatingGroupBackends drops terminating group members from this
// service's own group HTTPRoute. A deleting member waits for its peers' routes
// to release its backend (see finalizeGroupMembership), so this runs ahead of
// and independently of desired-state reconciliation: a peer whose own
// configuration is invalid must not hold a group deletion hostage.
func (r *LLMISVCReconciler) reconcileTerminatingGroupBackends(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if !llmSvc.Spec.Router.HasGroup() {
		return nil
	}

	// The logical route may be split over several HTTPRoutes; prune each one.
	routes, err := managedHTTPRoutes(ctx, r, llmSvc)
	if err != nil {
		return err
	}

	// A partial write after a group change can leave shards labelled with
	// different groups, so members are listed per group, not once.
	membersByGroup := map[string][]v1alpha2.LLMInferenceService{}
	firstGroup := ""
	for _, route := range routes {
		group := route.Labels[constants.LLMRoutingGroupLabelKey]
		if group == "" {
			continue
		}

		members, listed := membersByGroup[group]
		if !listed {
			list := &v1alpha2.LLMInferenceServiceList{}
			if err := r.List(ctx, list, client.InNamespace(route.Namespace),
				client.MatchingFields{groupFieldIndex: route.Namespace + "/" + group}); err != nil {
				return fmt.Errorf("failed to list routing group %q: %w", group, err)
			}
			members = list.Items
			membersByGroup[group] = members
			if firstGroup == "" {
				firstGroup = group
			}
		}

		stored := route.DeepCopy()
		if removed := removeTerminatingGroupBackends(route, members); len(removed) > 0 {
			if err := r.Patch(ctx, route, client.MergeFromWithOptions(stored, client.MergeFromWithOptimisticLock{})); err != nil {
				if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
					continue
				}
				return fmt.Errorf("failed to prune terminating backends from group HTTPRoute %s/%s: %w",
					route.Namespace, route.Name, err)
			}
		}
	}
	if firstGroup == "" {
		return nil
	}

	// Recompute membership even when nothing was pruned here: an earlier reconcile
	// may have patched the route successfully and then failed to persist status.
	// Membership comes from the member list rather than the route, so a route this
	// service was unable to write cannot erase a live peer from status. The
	// status names its group; without a listed match, the first shard's group
	// is used as before.
	if llmSvc.Status.Router != nil && llmSvc.Status.Router.Group != nil {
		members, ok := membersByGroup[llmSvc.Status.Router.Group.Name]
		if !ok {
			members = membersByGroup[firstGroup]
		}
		live := make(map[string]bool, len(members))
		for i := range members {
			if members[i].DeletionTimestamp.IsZero() {
				live[members[i].Name] = true
			}
		}
		llmSvc.Status.Router.Group.Members = slices.DeleteFunc(llmSvc.Status.Router.Group.Members,
			func(member v1alpha2.GroupMemberStatus) bool {
				return !live[member.Name]
			})
	}

	return nil
}

// removeTerminatingGroupBackends strips backendRefs belonging to terminating
// members from every rule, leaving matches, hostnames, rule order and surviving
// weights untouched. It returns the names of the members it pruned.
func removeTerminatingGroupBackends(route *gwapiv1.HTTPRoute, members []v1alpha2.LLMInferenceService) []string {
	var removed []string
	for i := range members {
		member := &members[i]
		if member.DeletionTimestamp.IsZero() {
			continue
		}
		changed := false
		for j := range route.Spec.Rules {
			rule := &route.Spec.Rules[j]
			before := len(rule.BackendRefs)
			rule.BackendRefs = slices.DeleteFunc(rule.BackendRefs, func(ref gwapiv1.HTTPBackendRef) bool {
				return backendRefersTo(ref, route.Namespace, member)
			})
			changed = changed || len(rule.BackendRefs) != before
		}
		if changed {
			removed = append(removed, member.Name)
		}
	}
	return removed
}
