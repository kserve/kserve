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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestGroupMemberEventHandler(t *testing.T) {
	tests := []struct {
		name            string
		objects         []client.Object
		event           func(*groupMemberEventHandler, workqueue.TypedRateLimitingInterface[reconcile.Request])
		wantEnqueued    []string
		wantNotEnqueued []string
	}{
		{
			name: "update: old group members enqueued when member switches groups",
			objects: objList(
				groupedSvc("v1", "group-a"),
				groupedSvc("v2", "group-a"),
				groupedSvc("v3", "group-b"),
			),
			event: updateEvent(
				groupedSvc("v3", "group-a"),
				groupedSvc("v3", "group-b"),
			),
			wantEnqueued:    []string{"v1", "v2"},
			wantNotEnqueued: []string{"v3"},
		},
		{
			name: "update: old group members enqueued when member leaves group",
			objects: objList(
				groupedSvc("v1", "group-a"),
				ungroupedSvc("v2"),
			),
			event: updateEvent(
				groupedSvc("v2", "group-a"),
				ungroupedSvc("v2"),
			),
			wantEnqueued: []string{"v1"},
		},
		{
			name: "update: peers enqueued on weight change",
			objects: objList(
				groupedSvc("v1", "group-a"),
				groupedSvcWithWeight("v2", "group-a", 5),
			),
			event: updateEvent(
				groupedSvc("v2", "group-a"),
				groupedSvcWithWeight("v2", "group-a", 5),
			),
			wantEnqueued:    []string{"v1"},
			wantNotEnqueued: []string{"v2"},
		},
		{
			name: "delete: peers enqueued on member deletion",
			objects: objList(
				groupedSvc("v1", "group-a"),
				groupedSvc("v2", "group-a"),
			),
			event:           deleteEvent(groupedSvc("v2", "group-a")),
			wantEnqueued:    []string{"v1"},
			wantNotEnqueued: []string{"v2"},
		},
		{
			name:    "create: non-grouped member enqueues nothing",
			objects: objList(ungroupedSvc("solo")),
			event:   createEvent(ungroupedSvc("solo")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &groupMemberEventHandler{
				reconciler: &LLMISVCReconciler{Client: fakeClientWithIndex(t, tt.objects...)},
			}
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			tt.event(h, q)

			names := requestNames(drainQueue(q))
			for _, want := range tt.wantEnqueued {
				assert.Contains(t, names, want)
			}
			for _, notWant := range tt.wantNotEnqueued {
				assert.NotContains(t, names, notWant)
			}
			if tt.wantEnqueued == nil && tt.wantNotEnqueued == nil {
				assert.Empty(t, names)
			}
		})
	}
}

// --- Test helpers ---

func groupedSvc(name, group string) v1alpha2.LLMInferenceService {
	return groupedSvcWithWeight(name, group, 1)
}

func groupedSvcWithWeight(name, group string, weight int32) v1alpha2.LLMInferenceService {
	return v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{constants.LLMRoutingGroupLabelKey: group},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{
					Group:  ptr.To(group),
					Weight: ptr.To(weight),
				},
			},
		},
	}
}

func ungroupedSvc(name string) v1alpha2.LLMInferenceService {
	return v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{},
	}
}

func objList(svcs ...v1alpha2.LLMInferenceService) []client.Object {
	objs := make([]client.Object, len(svcs))
	for i := range svcs {
		objs[i] = &svcs[i]
	}
	return objs
}

func updateEvent(oldSvc, newSvc v1alpha2.LLMInferenceService) func(*groupMemberEventHandler, workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	return func(h *groupMemberEventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		h.Update(context.Background(), event.UpdateEvent{ObjectOld: &oldSvc, ObjectNew: &newSvc}, q)
	}
}

func deleteEvent(svc v1alpha2.LLMInferenceService) func(*groupMemberEventHandler, workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	return func(h *groupMemberEventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		h.Delete(context.Background(), event.DeleteEvent{Object: &svc}, q)
	}
}

func createEvent(svc v1alpha2.LLMInferenceService) func(*groupMemberEventHandler, workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	return func(h *groupMemberEventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		h.Create(context.Background(), event.CreateEvent{Object: &svc}, q)
	}
}

func fakeClientWithIndex(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithIndex(&v1alpha2.LLMInferenceService{}, groupFieldIndex, func(obj client.Object) []string {
			llmSvc := obj.(*v1alpha2.LLMInferenceService)
			if g := llmSvc.Spec.Router.Group(); g != nil {
				return []string{llmSvc.GetNamespace() + "/" + *g}
			}
			return nil
		}).
		Build()
}

func drainQueue(q workqueue.TypedRateLimitingInterface[reconcile.Request]) []reconcile.Request {
	reqs := make([]reconcile.Request, 0, q.Len())
	for q.Len() > 0 {
		req, shutdown := q.Get()
		if shutdown {
			break
		}
		reqs = append(reqs, req)
		q.Done(req)
		q.Forget(req)
	}
	return reqs
}

func requestNames(reqs []reconcile.Request) []string {
	names := make([]string, len(reqs))
	for i, r := range reqs {
		names[i] = r.Name
	}
	return names
}
