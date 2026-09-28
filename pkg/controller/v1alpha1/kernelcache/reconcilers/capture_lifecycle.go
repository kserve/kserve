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

package reconcilers

import (
	"context"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// isReopenableProducerGoneCapture identifies generated captures that can retry
// after their producer Pod disappeared before capture completion.
func isReopenableProducerGoneCapture(capture *v1alpha1.KernelCacheCapture) bool {
	if capture == nil || capture.Labels[constants.KernelCacheCaptureGeneratedLabelKey] != "true" {
		return false
	}
	if capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseFailed ||
		capture.Status.Artifact != nil || capture.Status.KernelCacheRef != nil {
		return false
	}
	condition := meta.FindStatusCondition(capture.Status.Conditions, kernelCacheCaptureReadyConditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == kernelCacheCaptureReasonProducerGone
}

// reopenProducerGoneCapture resets stale capture state before a new Pod claims it.
func reopenProducerGoneCapture(ctx context.Context, c client.Client, reader client.Reader, key types.NamespacedName) (*v1alpha1.KernelCacheCapture, error) {
	if reader == nil {
		reader = c
	}
	var reopened *v1alpha1.KernelCacheCapture
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCacheCapture{}
		if err := reader.Get(ctx, key, current); err != nil {
			if apierrors.IsNotFound(err) {
				reopened = nil
				return nil
			}
			return err
		}
		if !isReopenableProducerGoneCapture(current) {
			// Another reconciler may have completed or replaced the capture while
			// this retry was in progress. Leave the newer terminal state alone.
			reopened = nil
			return nil
		}

		desired := current.Status.DeepCopy()
		desired.ActiveSession = nil
		desired.RuntimeResult = nil
		desired.Artifact = nil
		desired.CapturedAt = nil
		desired.CapturedCacheSizeBytes = nil
		desired.KernelCacheRef = nil
		desired.Signing = nil
		desired.Phase = v1alpha1.KernelCacheCapturePhasePending
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonRetryingProducerGone, "retrying capture after the producer Pod disappeared", current.Generation)
		if reflect.DeepEqual(current.Status, *desired) {
			reopened = current
			return nil
		}
		current.Status = *desired
		if err := c.Status().Update(ctx, current); err != nil {
			return err
		}
		reopened = current
		return nil
	})
	return reopened, err
}
