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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/kernelcache/reporter"
)

func TestCaptureStatusValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	validator := &CaptureStatusValidator{Decoder: admission.NewDecoder(scheme)}

	base := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "model-kcc-abc123", Namespace: "team"},
		Status:     v1alpha1.KernelCacheCaptureStatus{RuntimeResult: map[string]string{"sourcePodName": "pod-a", "state": "Capturing"}},
	}
	request := func(oldCapture, updated *v1alpha1.KernelCacheCapture) admission.Request {
		oldRaw, err := json.Marshal(oldCapture)
		require.NoError(t, err)
		newRaw, err := json.Marshal(updated)
		require.NoError(t, err)
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID: types.UID("1"), Namespace: "team", Operation: admissionv1.Update, SubResource: "status",
			UserInfo: authenticationv1.UserInfo{Username: "system:serviceaccount:team:" + reporter.ServiceAccountName(oldCapture.Name)},
			Object:   runtime.RawExtension{Raw: newRaw}, OldObject: runtime.RawExtension{Raw: oldRaw},
		}}
	}

	t.Run("allows runtime result updates", func(t *testing.T) {
		updated := base.DeepCopy()
		updated.Status.RuntimeResult["state"] = "Pushing"
		response := validator.Handle(t.Context(), request(base, updated))
		require.True(t, response.Allowed)
	})

	t.Run("allows the first runtime result before active session backfill", func(t *testing.T) {
		oldCapture := base.DeepCopy()
		oldCapture.Status.RuntimeResult = nil
		updated := oldCapture.DeepCopy()
		updated.Status.RuntimeResult = map[string]string{
			"sourcePodName":    "pod-a",
			"captureSessionID": "session-a",
			"state":            "Capturing",
		}
		response := validator.Handle(t.Context(), request(oldCapture, updated))
		require.True(t, response.Allowed)
	})

	t.Run("rejects a runtime result for a different active session", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			result map[string]string
		}{
			{
				name: "different pod",
				result: map[string]string{
					"sourcePodName":    "pod-b",
					"captureSessionID": "session-a",
					"state":            "Capturing",
				},
			},
			{
				name: "different session",
				result: map[string]string{
					"sourcePodName":    "pod-a",
					"captureSessionID": "session-b",
					"state":            "Capturing",
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				oldCapture := base.DeepCopy()
				oldCapture.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{
					ID: "session-a", PodName: "pod-a",
				}
				oldCapture.Status.RuntimeResult = nil
				updated := oldCapture.DeepCopy()
				updated.Status.RuntimeResult = tc.result

				response := validator.Handle(t.Context(), request(oldCapture, updated))
				require.False(t, response.Allowed)
				require.Contains(t, response.Result.Message, "reporter identity is not authorized")
			})
		}
	})

	t.Run("allows a runtime result for the active session", func(t *testing.T) {
		oldCapture := base.DeepCopy()
		oldCapture.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{
			ID: "session-a", PodName: "pod-a",
		}
		oldCapture.Status.RuntimeResult = nil
		updated := oldCapture.DeepCopy()
		updated.Status.RuntimeResult = map[string]string{
			"sourcePodName":    "pod-a",
			"captureSessionID": "session-a",
			"state":            "Capturing",
		}

		response := validator.Handle(t.Context(), request(oldCapture, updated))
		require.True(t, response.Allowed)
	})

	t.Run("rejects a claim replacement", func(t *testing.T) {
		updated := base.DeepCopy()
		updated.Status.RuntimeResult["sourcePodName"] = "pod-b"
		response := validator.Handle(t.Context(), request(base, updated))
		require.False(t, response.Allowed)
	})

	t.Run("rejects operator-owned status changes", func(t *testing.T) {
		updated := base.DeepCopy()
		updated.Status.Phase = v1alpha1.KernelCacheCapturePhasePushing
		response := validator.Handle(t.Context(), request(base, updated))
		require.False(t, response.Allowed)
	})

	t.Run("rejects updates after terminal capture", func(t *testing.T) {
		for _, phase := range []v1alpha1.KernelCacheCapturePhase{
			v1alpha1.KernelCacheCapturePhaseComplete,
			v1alpha1.KernelCacheCapturePhaseUnchanged,
			v1alpha1.KernelCacheCapturePhaseFailed,
		} {
			t.Run(string(phase), func(t *testing.T) {
				oldCapture := base.DeepCopy()
				oldCapture.Status.Phase = phase
				updated := oldCapture.DeepCopy()
				updated.Status.RuntimeResult["state"] = "Pushing"

				response := validator.Handle(t.Context(), request(oldCapture, updated))
				require.False(t, response.Allowed)
			})
		}
	})
}
