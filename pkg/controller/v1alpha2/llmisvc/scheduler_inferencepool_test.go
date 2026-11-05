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
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
)

func TestMultiPortLegacyInferencePoolCleanup(t *testing.T) {
	seedInferencePoolV1Alpha2Discovery(t)
	for _, tc := range []struct {
		name         string
		shouldDelete bool
	}{
		{name: "preserves the legacy pool during migration"},
		{name: "deletes the legacy pool when cleanup is requested", shouldDelete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, igwapiv1alpha2.Install(scheme))
			r := &LLMISVCReconciler{
				Client:        fake.NewClientBuilder().WithScheme(scheme).Build(),
				EventRecorder: record.NewFakeRecorder(10),
			}
			svc := managedPoolService()
			svc.UID = "service-uid"
			svc.Spec.Router.Scheduler.Pool.Spec.TargetPorts = []igwapi.Port{{Number: 8000}, {Number: 8001}}
			legacy := r.expectedSchedulerInferencePoolV1Alpha2(t.Context(), svc)
			// The existing legacy pool must not be rewritten from the multi-port spec.
			legacy.Spec.TargetPortNumber = 7777
			require.NoError(t, r.Create(t.Context(), legacy))

			require.NoError(t, r.reconcileV1Alpha2InferencePool(t.Context(), svc, tc.shouldDelete))
			current := &igwapiv1alpha2.InferencePool{}
			err := r.Get(t.Context(), client.ObjectKeyFromObject(legacy), current)
			if tc.shouldDelete {
				require.True(t, apierrors.IsNotFound(err), "the legacy pool should be deleted, got %v", err)
				// Repeating cleanup must succeed after the resource is gone.
				require.NoError(t, r.reconcileV1Alpha2InferencePool(t.Context(), svc, true))
			} else {
				require.NoError(t, err)
				require.Equal(t, legacy.Spec, current.Spec)
			}
		})
	}
}
