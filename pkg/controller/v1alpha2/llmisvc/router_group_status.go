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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const reasonMemberDivergence = "MemberDivergence"

// applyGroupStatus sets group conditions and status on the LLMISVC.
// Call after the HTTPRoute write succeeds so status reflects committed state.
func (r *LLMISVCReconciler) applyGroupStatus(llmSvc *v1alpha2.LLMInferenceService, matching, divergent []resolvedMember) {
	updateGroupStatus(llmSvc, matching)
	llmSvc.MarkGroupReady()

	if len(divergent) > 0 {
		msg := divergenceMessage(divergent)
		llmSvc.MarkGroupDegraded(reasonMemberDivergence, msg)
		r.Eventf(llmSvc, corev1.EventTypeWarning, reasonMemberDivergence,
			"Group %q has members with divergent model.name or LoRA adapter sets",
			*llmSvc.Spec.Router.Route.Group)
	} else {
		llmSvc.MarkGroupNotDegraded()
	}
}

// updateGroupStatus populates the group topology in the LLMISVC status.
func updateGroupStatus(llmSvc *v1alpha2.LLMInferenceService, resolved []resolvedMember) {
	groupStatus := &v1alpha2.GroupStatus{
		Name: *llmSvc.Spec.Router.Route.Group,
	}

	for _, m := range resolved {
		ref := m.backendRef
		groupStatus.Members = append(groupStatus.Members, v1alpha2.GroupMemberStatus{
			Name:       m.name,
			Weight:     m.weight,
			Stopped:    m.stopped,
			BackendRef: &ref,
		})
	}

	if llmSvc.Status.Router == nil {
		llmSvc.Status.Router = &v1alpha2.RouterStatus{}
	}
	llmSvc.Status.Router.Group = groupStatus
}

func divergenceMessage(divergent []resolvedMember) string {
	models := make([]string, 0, len(divergent))
	for _, m := range divergent {
		models = append(models, strings.Join(m.modelNames, "+"))
	}
	slices.Sort(models)
	return "group has members with different model sets: " + strings.Join(slices.Compact(models), ", ")
}
