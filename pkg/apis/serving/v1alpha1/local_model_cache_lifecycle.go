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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalModelCacheConditionType is the type of a LocalModelCacheStatus condition.
type LocalModelCacheConditionType string

const (
	// LocalModelCacheReady is a positive-polarity condition that is True once the model has
	// been imported and is available for serving. For shared-PVC mode it is the stable
	// polling contract for Dashboard, Model Registry, and serving admission.
	LocalModelCacheReady LocalModelCacheConditionType = "Ready"
)

// Ready condition reasons. These are part of the API contract and must remain stable.
const (
	// ReasonPVCNotFound indicates the referenced PVC does not exist in the cache namespace.
	ReasonPVCNotFound = "PVCNotFound"
	// ReasonUnsupportedVolumeMode indicates the referenced PVC is not filesystem volume mode.
	ReasonUnsupportedVolumeMode = "UnsupportedVolumeMode"
	// ReasonUnsupportedAccessMode indicates the referenced PVC does not include ReadWriteMany.
	ReasonUnsupportedAccessMode = "UnsupportedAccessMode"
	// ReasonInsufficientCapacity indicates the referenced PVC cannot hold the model.
	ReasonInsufficientCapacity = "InsufficientCapacity"
	// ReasonDestinationConflict indicates another cache already owns the destination tuple.
	ReasonDestinationConflict = "DestinationConflict"
	// ReasonImportPending indicates the import Job has not started running yet.
	ReasonImportPending = "ImportPending"
	// ReasonImportRunning indicates the import Job is running.
	ReasonImportRunning = "ImportRunning"
	// ReasonImportFailed indicates the import Job failed terminally.
	ReasonImportFailed = "ImportFailed"
	// ReasonImportJobConflict indicates the deterministic import Job name is occupied by a foreign Job.
	ReasonImportJobConflict = "ImportJobConflict"
	// ReasonImportSucceeded indicates the import Job completed successfully.
	ReasonImportSucceeded = "ImportSucceeded"
)

// GetCondition returns the condition of the given type, or nil if absent.
func (s *LocalModelCacheStatus) GetCondition(conditionType LocalModelCacheConditionType) *metav1.Condition {
	return meta.FindStatusCondition(s.Conditions, string(conditionType))
}

// IsReady reports whether the cache has a Ready=True condition for its current generation.
func (in *LocalModelNamespaceCache) IsReady() bool {
	condition := in.Status.GetCondition(LocalModelCacheReady)
	return condition != nil && condition.Status == metav1.ConditionTrue && condition.ObservedGeneration == in.Generation
}

// setCondition sets a condition, preserving lastTransitionTime unless the status changed.
// meta.SetStatusCondition updates observedGeneration and only bumps lastTransitionTime
// when the status (True/False/Unknown) changes.
func (s *LocalModelCacheStatus) setCondition(conditionType LocalModelCacheConditionType, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
}

// MarkReady marks the Ready condition True with reason ImportSucceeded.
func (s *LocalModelCacheStatus) MarkReady(observedGeneration int64) {
	s.setCondition(LocalModelCacheReady, metav1.ConditionTrue, ReasonImportSucceeded, "Model import completed", observedGeneration)
}

// MarkNotReady marks the Ready condition False with the given reason and message.
func (s *LocalModelCacheStatus) MarkNotReady(reason, message string, observedGeneration int64) {
	s.setCondition(LocalModelCacheReady, metav1.ConditionFalse, reason, message, observedGeneration)
}

// MarkReadyUnknown marks the Ready condition Unknown with the given reason and message.
func (s *LocalModelCacheStatus) MarkReadyUnknown(reason, message string, observedGeneration int64) {
	s.setCondition(LocalModelCacheReady, metav1.ConditionUnknown, reason, message, observedGeneration)
}
