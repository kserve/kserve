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
	"slices"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var _ handler.EventHandler = &groupMemberEventHandler{}

// groupMemberEventHandler implements handler.EventHandler to enqueue group
// siblings when a grouped LLMISVC changes. Using a typed handler (instead of
// EnqueueRequestsFromMapFunc) gives access to both old and new objects on
// updates - needed for old-group cleanup since the defaulting webhook
// synchronizes label and spec before the controller sees the new object.
type groupMemberEventHandler struct {
	reconciler *LLMISVCReconciler
}

func (h *groupMemberEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCurrentGroupMembers(ctx, e.Object, q)
}

func (h *groupMemberEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSvc := e.ObjectOld.(*v1alpha2.LLMInferenceService)
	newSvc := e.ObjectNew.(*v1alpha2.LLMInferenceService)

	h.enqueueCurrentGroupMembers(ctx, newSvc, q)

	// When group changes, also enqueue OLD group members so they remove the
	// departed member's backendRef. The old object carries the previous group
	// value (informer cache snapshot before the update).
	oldGroup := oldSvc.Spec.Router.Group()
	newGroup := newSvc.Spec.Router.Group()
	if oldGroup != nil && !ptr.Equal(oldGroup, newGroup) {
		h.enqueueOldGroupMembers(ctx, oldSvc, newSvc.Name, q)
	}
}

func (h *groupMemberEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCurrentGroupMembers(ctx, e.Object, q)
}

func (h *groupMemberEventHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *groupMemberEventHandler) enqueueCurrentGroupMembers(ctx context.Context, obj client.Object, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	llmSvc := obj.(*v1alpha2.LLMInferenceService)
	if !llmSvc.Spec.Router.HasGroup() {
		return
	}

	members, err := h.reconciler.listGroupMembers(ctx, llmSvc)
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to list group members for enqueue")
		return
	}

	for i := range members {
		if members[i].Name == llmSvc.Name {
			continue
		}
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: members[i].Namespace,
				Name:      members[i].Name,
			},
		})
	}
}

func (h *groupMemberEventHandler) enqueueOldGroupMembers(
	ctx context.Context,
	oldSvc *v1alpha2.LLMInferenceService,
	selfName string,
	q workqueue.TypedRateLimitingInterface[reconcile.Request],
) {
	oldGroup := *oldSvc.Spec.Router.Group()
	list := &v1alpha2.LLMInferenceServiceList{}
	if err := h.reconciler.List(ctx, list,
		client.InNamespace(oldSvc.Namespace),
		client.MatchingLabels{constants.LLMRoutingGroupLabelKey: oldGroup},
	); err != nil {
		log.FromContext(ctx).Error(err, "failed to list old group members for enqueue",
			"oldGroup", oldGroup)
		return
	}

	for i := range list.Items {
		if list.Items[i].Name == selfName {
			continue
		}
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: list.Items[i].Namespace,
				Name:      list.Items[i].Name,
			},
		})
	}
}

// groupMemberChangePredicate limits fan-out to traffic-relevant changes only.
// Without this, any spec change to a grouped LLMISVC triggers reconciliation
// of all group members.
func groupMemberChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.(*v1alpha2.LLMInferenceService).Spec.Router.HasGroup()
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSvc := e.ObjectOld.(*v1alpha2.LLMInferenceService)
			newSvc := e.ObjectNew.(*v1alpha2.LLMInferenceService)
			return trafficFieldsChanged(oldSvc, newSvc)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.(*v1alpha2.LLMInferenceService).Spec.Router.HasGroup()
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func trafficFieldsChanged(old, new *v1alpha2.LLMInferenceService) bool {
	if old.DeletionTimestamp.IsZero() != new.DeletionTimestamp.IsZero() {
		return true
	}

	if utils.GetForceStopRuntime(old) != utils.GetForceStopRuntime(new) {
		return true
	}

	if !equality.Semantic.DeepEqual(old.Spec.Router, new.Spec.Router) {
		return true
	}

	if !equality.Semantic.DeepEqual(old.Spec.Model, new.Spec.Model) {
		return true
	}

	if !slices.Equal(old.Spec.BaseRefs, new.Spec.BaseRefs) {
		return true
	}

	if isMemberRoutable(old) != isMemberRoutable(new) {
		return true
	}

	return false
}
