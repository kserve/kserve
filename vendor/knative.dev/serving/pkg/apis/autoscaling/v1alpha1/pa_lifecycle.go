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
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
)

var podCondSet = apis.NewLivingConditionSet(
	PodAutoscalerConditionActive,
)

func (pa *PodAutoscaler) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("PodAutoscaler")
}

func (pa *PodAutoscaler) Class() string {
	if c, ok := pa.Annotations[autoscaling.ClassAnnotationKey]; ok {
		return c
	}
	// Default to "kpa" class for backward compatibility.
	return autoscaling.KPA
}

// Metric returns the contents of the metric annotation or a default.
func (pa *PodAutoscaler) Metric() string {
	if m, ok := pa.Annotations[autoscaling.MetricAnnotationKey]; ok {
		return m
	}
	// TODO: defaulting here is awkward and is already taken care of by defaulting logic.
	return defaultMetric(pa.Class())
}

func (pa *PodAutoscaler) annotationInt32(key string) int32 {
	if s, ok := pa.Annotations[key]; ok {
		// no error check: relying on validation
		i, _ := strconv.ParseInt(s, 10, 32)
		if i < 0 {
			return 0
		}
		return int32(i)
	}
	return 0
}

func (pa *PodAutoscaler) annotationFloat64(key string) (float64, bool) {
	if s, ok := pa.Annotations[key]; ok {
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0.0, false
}

// ScaleBounds returns scale bounds annotations values as a tuple:
// `(min, max int32)`. The value of 0 for any of min or max means the bound is
// not set.
// Note: min will be ignored if the PA is not reachable
func (pa *PodAutoscaler) ScaleBounds() (min, max int32) {
	if pa.Spec.Reachability != ReachabilityUnreachable {
		min = pa.annotationInt32(autoscaling.MinScaleAnnotationKey)
	}
	max = pa.annotationInt32(autoscaling.MaxScaleAnnotationKey)

	return
}

// Target returns the target annotation value or false if not present, or invalid.
func (pa *PodAutoscaler) Target() (float64, bool) {
	return pa.annotationFloat64(autoscaling.TargetAnnotationKey)
}

// TargetUtilization returns the target capacity utilization as a fraction,
// if the corresponding annotation is set.
func (pa *PodAutoscaler) TargetUtilization() (float64, bool) {
	if tu, ok := pa.annotationFloat64(autoscaling.TargetUtilizationPercentageKey); ok {
		return tu / 100, true
	}
	return 0, false
}

// TargetBC returns the target burst capacity,
// if the corresponding annotation is set.
func (pa *PodAutoscaler) TargetBC() (float64, bool) {
	// The value is validated in the webhook.
	return pa.annotationFloat64(autoscaling.TargetBurstCapacityKey)
}

// Window returns the window annotation value or false if not present.
func (pa *PodAutoscaler) Window() (window time.Duration, ok bool) {
	// The value is validated in the webhook.
	if s, ok := pa.Annotations[autoscaling.WindowAnnotationKey]; ok {
		d, err := time.ParseDuration(s)
		return d, err == nil
	}
	return 0, false
}

// PanicWindowPercentage returns panic window annotation value or false if not present.
func (pa *PodAutoscaler) PanicWindowPercentage() (percentage float64, ok bool) {
	// The value is validated in the webhook.
	return pa.annotationFloat64(autoscaling.PanicWindowPercentageAnnotationKey)
}

// PanicThresholdPercentage return the panic target annotation value or false if not present.
func (pa *PodAutoscaler) PanicThresholdPercentage() (percentage float64, ok bool) {
	// The value is validated in the webhook.
	return pa.annotationFloat64(autoscaling.PanicThresholdPercentageAnnotationKey)
}

// IsReady looks at the conditions and if the Status has a condition
// PodAutoscalerConditionReady returns true if ConditionStatus is True
func (pas *PodAutoscalerStatus) IsReady() bool {
	return podCondSet.Manage(pas.duck()).IsHappy()
}

// IsActivating returns true if the pod autoscaler is Activating if it is neither
// Active nor Inactive
func (pas *PodAutoscalerStatus) IsActivating() bool {
	cond := pas.GetCondition(PodAutoscalerConditionActive)
	return cond != nil && cond.Status == corev1.ConditionUnknown
}

// IsInactive returns true if the pod autoscaler is Inactive.
func (pas *PodAutoscalerStatus) IsInactive() bool {
	cond := pas.GetCondition(PodAutoscalerConditionActive)
	return cond != nil && cond.Status == corev1.ConditionFalse
}

// GetCondition gets the condition `t`.
func (pas *PodAutoscalerStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return podCondSet.Manage(pas.duck()).GetCondition(t)
}

// InitializeConditions initializes the conditionhs of the PA.
func (pas *PodAutoscalerStatus) InitializeConditions() {
	podCondSet.Manage(pas.duck()).InitializeConditions()
}

// MarkActive marks the PA active.
func (pas *PodAutoscalerStatus) MarkActive() {
	podCondSet.Manage(pas.duck()).MarkTrue(PodAutoscalerConditionActive)
}

// MarkActivating marks the PA as activating.
func (pas *PodAutoscalerStatus) MarkActivating(reason, message string) {
	podCondSet.Manage(pas.duck()).MarkUnknown(PodAutoscalerConditionActive, reason, message)
}

// MarkInactive marks the PA as inactive.
func (pas *PodAutoscalerStatus) MarkInactive(reason, message string) {
	podCondSet.Manage(pas.duck()).MarkFalse(PodAutoscalerConditionActive, reason, message)
}

// MarkResourceNotOwned changes the "Active" condition to false to reflect that the
// resource of the given kind and name has already been created, and we do not own it.
func (pas *PodAutoscalerStatus) MarkResourceNotOwned(kind, name string) {
	pas.MarkInactive("NotOwned",
		fmt.Sprintf("There is an existing %s %q that we do not own.", kind, name))
}

// MarkResourceFailedCreation changes the "Active" condition to false to reflect that a
// critical resource of the given kind and name was unable to be created.
func (pas *PodAutoscalerStatus) MarkResourceFailedCreation(kind, name string) {
	pas.MarkInactive("FailedCreate",
		fmt.Sprintf("Failed to create %s %q.", kind, name))
}

// CanScaleToZero checks whether the pod autoscaler has been in an inactive state
// for at least the specified grace period.
func (pas *PodAutoscalerStatus) CanScaleToZero(now time.Time, gracePeriod time.Duration) bool {
	return pas.inStatusFor(corev1.ConditionFalse, now, gracePeriod) > 0
}

// ActiveFor returns the time PA spent being active.
func (pas *PodAutoscalerStatus) ActiveFor(now time.Time) time.Duration {
	return pas.inStatusFor(corev1.ConditionTrue, now, 0)
}

// CanFailActivation checks whether the pod autoscaler has been activating
// for at least the specified idle period.
func (pas *PodAutoscalerStatus) CanFailActivation(now time.Time, idlePeriod time.Duration) bool {
	return pas.inStatusFor(corev1.ConditionUnknown, now, idlePeriod) > 0
}

// inStatusFor returns positive duration if the PodAutoscalerStatus's Active condition has stayed in
// the specified status for at least the specified duration. Otherwise it returns negative duration,
// including when the status is undetermined (Active condition is not found.)
func (pas *PodAutoscalerStatus) inStatusFor(status corev1.ConditionStatus, now time.Time, dur time.Duration) time.Duration {
	cond := pas.GetCondition(PodAutoscalerConditionActive)
	if cond == nil || cond.Status != status {
		return -1
	}
	return now.Sub(cond.LastTransitionTime.Inner.Add(dur))
}

func (pas *PodAutoscalerStatus) duck() *duckv1.Status {
	return (*duckv1.Status)(&pas.Status)
}

// GetDesiredScale returns the desired scale if ever set, or -1.
func (pas *PodAutoscalerStatus) GetDesiredScale() int32 {
	if pas.DesiredScale != nil {
		return *pas.DesiredScale
	}
	return -1
}

// GetActualScale returns the desired scale if ever set, or -1.
func (pas *PodAutoscalerStatus) GetActualScale() int32 {
	if pas.ActualScale != nil {
		return *pas.ActualScale
	}
	return -1
}
