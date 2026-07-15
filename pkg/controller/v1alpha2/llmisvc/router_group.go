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
	"cmp"
	"context"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	kmeta "knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const reasonFinalizationPending = "FinalizationPending"

const groupFieldIndex = ".spec.router.route.group"

// resolvedMember holds the effective weight and backend for a single group member.
type resolvedMember struct {
	name       string
	modelNames []string
	weight     int32
	stopped    bool
	backendRef gwapiv1.BackendObjectReference
}

// setupGroupFieldIndex registers a field indexer on spec.router.route.group
// for efficient group member discovery without listing all LLMISVCs in a namespace.
func setupGroupFieldIndex(ctx context.Context, mgr client.FieldIndexer) error {
	return mgr.IndexField(
		ctx,
		&v1alpha2.LLMInferenceService{},
		groupFieldIndex,
		func(obj client.Object) []string {
			llmSvc := obj.(*v1alpha2.LLMInferenceService)
			if g := llmSvc.Spec.Router.Group(); g != nil {
				return []string{llmSvc.Namespace + "/" + *g}
			}
			return nil
		},
	)
}

// injectGroupBackendRefs post-processes the template-rendered HTTPRoute to add
// weighted backendRefs for all group members. Each member's route carries
// backendRefs for ALL members so the gateway can distribute traffic proportionally.
//
// Returns a groupResult for the caller to apply status after the route write.
func (r *LLMISVCReconciler) injectGroupBackendRefs(
	ctx context.Context,
	llmSvc *v1alpha2.LLMInferenceService,
	route *gwapiv1.HTTPRoute,
) (matching, divergent []resolvedMember, err error) {
	members, err := r.listGroupMembers(ctx, llmSvc)
	if err != nil {
		return nil, nil, fmt.Errorf("injecting group backends: %w", err)
	}

	resolved, err := r.resolveGroupMembers(filterEligibleMembers(members, llmSvc.Name))
	if err != nil {
		return nil, nil, err
	}

	selfModels := resolvedModelNames(llmSvc)

	for _, m := range resolved {
		if slices.Equal(m.modelNames, selfModels) {
			matching = append(matching, m)
		} else {
			divergent = append(divergent, m)
		}
	}

	rewriteRulesForGroup(route, llmSvc, matching)

	return matching, divergent, nil
}

// resolveGroupMembers reads each member's backendRef from their status.
// Members without scheduler status use the workload Service as their backend.
func (r *LLMISVCReconciler) resolveGroupMembers(
	members []v1alpha2.LLMInferenceService,
) ([]resolvedMember, error) {
	resolved := make([]resolvedMember, 0, len(members))
	for i := range members {
		m := &members[i]
		stopped := utils.GetForceStopRuntime(m)
		w := ptr.Deref(m.Spec.Router.Weight(), 0)

		backendRef, err := resolveMemberBackendRef(m)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve backend for member %s: %w", m.Name, err)
		}

		resolved = append(resolved, resolvedMember{
			name:       m.Name,
			modelNames: resolvedModelNames(m),
			weight:     w,
			stopped:    stopped,
			backendRef: backendRef,
		})
	}

	slices.SortFunc(resolved, func(a, b resolvedMember) int {
		return cmp.Compare(a.name, b.name)
	})

	return resolved, nil
}

// resolvedModelNames returns the deduplicated, sorted set of model names
// served by a member. Prefers status.Addresses (which reflects baseRef merges)
// over raw spec.
func resolvedModelNames(m *v1alpha2.LLMInferenceService) []string {
	var names []string
	for _, addr := range m.Status.Addresses {
		for _, model := range addr.Models {
			names = append(names, model.Name)
		}
	}

	if len(names) > 0 {
		slices.Sort(names)
		return slices.Compact(names)
	}

	names = append(names, ptr.Deref(m.Spec.Model.Name, m.Name))
	if m.Spec.Model.LoRA != nil {
		for _, a := range m.Spec.Model.LoRA.Adapters {
			if a.Name != nil {
				names = append(names, *a.Name)
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// resolveMemberBackendRef reads the member's backend from its status or spec.
func resolveMemberBackendRef(
	member *v1alpha2.LLMInferenceService,
) (gwapiv1.BackendObjectReference, error) {
	if member.Spec.Router != nil &&
		member.Spec.Router.Scheduler != nil &&
		member.Spec.Router.Scheduler.Pool.HasRef() {
		return gwapiv1.BackendObjectReference{
			Group: ptr.To(gwapiv1.Group(constants.InferencePoolV1Alpha2APIGroupName)),
			Kind:  ptr.To(gwapiv1.Kind("InferencePool")),
			Name:  gwapiv1.ObjectName(member.Spec.Router.Scheduler.Pool.Ref.Name),
			Port:  ptr.To(gwapiv1.PortNumber(8000)),
		}, nil
	}

	if member.Status.Router != nil &&
		member.Status.Router.Scheduler != nil &&
		member.Status.Router.Scheduler.InferencePool != nil {
		pool := member.Status.Router.Scheduler.InferencePool
		return gwapiv1.BackendObjectReference{
			Group: ptr.To(pool.Group),
			Kind:  ptr.To(pool.Kind),
			Name:  pool.Name,
			Port:  ptr.To(gwapiv1.PortNumber(8000)),
		}, nil
	}

	svcName := workloadServiceName(member)
	if svcName != "" {
		return gwapiv1.BackendObjectReference{
			Kind: ptr.To(gwapiv1.Kind("Service")),
			Name: gwapiv1.ObjectName(svcName),
			Port: ptr.To(gwapiv1.PortNumber(8000)),
		}, nil
	}

	return gwapiv1.BackendObjectReference{}, fmt.Errorf(
		"member %s/%s has no scheduler status and no workload service",
		member.Namespace, member.Name)
}

// finalizeGroupMembership ensures other members' routes no longer reference
// this member's backend before allowing deletion to proceed. Returns true when
// all members have converged, false when still waiting.
func (r *LLMISVCReconciler) finalizeGroupMembership(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (bool, error) {
	if !llmSvc.Spec.Router.HasGroup() {
		return true, nil
	}

	logger := log.FromContext(ctx)

	members, err := r.listGroupMembers(ctx, llmSvc)
	if err != nil {
		return false, fmt.Errorf("finalizing group membership: %w", err)
	}

	poolName := (&v1alpha2.SchedulerSpec{}).InferencePoolName(llmSvc)
	svcName := workloadServiceName(llmSvc)

	for i := range members {
		if members[i].Name == llmSvc.Name || !members[i].DeletionTimestamp.IsZero() {
			continue
		}
		route := &gwapiv1.HTTPRoute{}
		routeKey := types.NamespacedName{
			Name:      kmeta.ChildName(members[i].GetName(), "-kserve-route"),
			Namespace: members[i].GetNamespace(),
		}
		if err := r.Get(ctx, routeKey, route); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("checking member route %s for stale backendRefs: %w", routeKey.Name, err)
		}
		if routeReferencesBackend(route, poolName) || routeReferencesBackend(route, svcName) {
			logger.Info("Waiting for member to remove backendRef before deletion",
				"member", members[i].Name, "pool", poolName)
			llmSvc.MarkGroupNotReady(reasonFinalizationPending,
				"waiting for member %s to remove backendRef before deletion", members[i].Name)
			return false, nil
		}
	}

	return true, nil
}

func routeReferencesBackend(route *gwapiv1.HTTPRoute, backendName string) bool {
	for _, rule := range route.Spec.Rules {
		for _, ref := range rule.BackendRefs {
			if string(ref.Name) == backendName {
				return true
			}
		}
	}
	return false
}

// listGroupMembers returns all LLMISVCs in the same namespace with the same group.
func (r *LLMISVCReconciler) listGroupMembers(
	ctx context.Context,
	llmSvc *v1alpha2.LLMInferenceService,
) ([]v1alpha2.LLMInferenceService, error) {
	group := llmSvc.Spec.Router.Group()
	if group == nil {
		return nil, nil
	}
	list := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, list,
		client.InNamespace(llmSvc.Namespace),
		client.MatchingFields{groupFieldIndex: llmSvc.Namespace + "/" + *group},
	); err != nil {
		return nil, fmt.Errorf("failed to list group members for group %q: %w", *group, err)
	}
	return list.Items, nil
}

// filterEligibleMembers returns non-terminating members that are routable or
// force-stopped. Self always passes. Force-stopped members bypass the routable
// check so they remain visible in group status with stopped=true.
func filterEligibleMembers(members []v1alpha2.LLMInferenceService, selfName string) []v1alpha2.LLMInferenceService {
	eligible := make([]v1alpha2.LLMInferenceService, 0, len(members))
	for i := range members {
		if !members[i].DeletionTimestamp.IsZero() {
			continue
		}
		if members[i].Name != selfName &&
			!utils.GetForceStopRuntime(&members[i]) &&
			!isMemberRoutable(&members[i]) {
			continue
		}
		eligible = append(eligible, members[i])
	}
	return eligible
}

// isMemberRoutable reports whether a member has routing infrastructure ready
// and at least one available replica on every configured workload.
func isMemberRoutable(member *v1alpha2.LLMInferenceService) bool {
	cond := member.Status.GetCondition(v1alpha2.RouterReady)
	if cond == nil || !cond.IsTrue() {
		return false
	}
	ws := member.Status.Workloads
	if ws == nil || ws.Primary == nil {
		return false
	}
	for _, w := range []*v1alpha2.ObservedWorkloadStatus{ws.Primary, ws.Prefill, ws.Scheduler} {
		if w != nil && (w.ReadyReplicas == nil || *w.ReadyReplicas == 0) {
			return false
		}
	}
	return true
}
