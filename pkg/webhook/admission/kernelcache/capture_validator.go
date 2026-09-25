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

package kernelcache

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/kernelcache/reporter"
)

// CaptureStatusValidator restricts authenticated MCV reporters to the
// untrusted runtimeResult status map, accepts reports only from the active
// session, and makes the first source Pod claim immutable. It intentionally
// does not promote runtimeResult to trusted state.
type CaptureStatusValidator struct {
	Decoder admission.Decoder
}

func (v *CaptureStatusValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != "UPDATE" || req.SubResource != "status" || !reporter.IsReporterUsername(req.UserInfo.Username) {
		return admission.Allowed("")
	}
	oldCapture := &v1alpha1.KernelCacheCapture{}
	newCapture := &v1alpha1.KernelCacheCapture{}
	if err := v.Decoder.DecodeRaw(req.OldObject, oldCapture); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := v.Decoder.Decode(req, newCapture); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if !reporter.IsReporterForCapture(req.UserInfo.Username, req.Namespace, newCapture.Name) {
		return admission.Denied("reporter identity is not authorized for this KernelCacheCapture")
	}
	if isTerminalCapturePhase(oldCapture.Status.Phase) {
		return admission.Denied("terminal KernelCacheCapture does not accept reporter updates")
	}

	oldStatus := oldCapture.Status.DeepCopy()
	newStatus := newCapture.Status.DeepCopy()
	oldRuntimeResult := oldStatus.RuntimeResult
	newRuntimeResult := newStatus.RuntimeResult
	oldStatus.RuntimeResult = nil
	newStatus.RuntimeResult = nil
	if !reflect.DeepEqual(oldStatus, newStatus) {
		return admission.Denied("MCV reporters may modify only status.runtimeResult")
	}

	if active := oldCapture.Status.ActiveSession; active != nil &&
		(newRuntimeResult["captureSessionID"] != active.ID ||
			newRuntimeResult["sourcePodName"] != active.PodName) {
		return admission.Denied("reporter identity is not authorized for this KernelCacheCapture")
	}

	oldSource := oldRuntimeResult["sourcePodName"]
	newSource := newRuntimeResult["sourcePodName"]
	if oldSource != "" && newSource != oldSource {
		return admission.Denied(fmt.Sprintf("runtimeResult.sourcePodName is already claimed by %q", oldSource))
	}
	if oldSource == "" && newSource == "" {
		return admission.Denied("runtimeResult.sourcePodName is required for an MCV claim")
	}
	return admission.Allowed("")
}

func isTerminalCapturePhase(phase v1alpha1.KernelCacheCapturePhase) bool {
	switch phase {
	case v1alpha1.KernelCacheCapturePhaseComplete,
		v1alpha1.KernelCacheCapturePhaseUnchanged,
		v1alpha1.KernelCacheCapturePhaseFailed:
		return true
	default:
		return false
	}
}
