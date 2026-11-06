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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// updateStatus decides on readiness Events from the stored status against the new one,
// and the envtest specs can't tell when a reconcile that writes nothing has run.
func TestUpdateStatusReadinessEvents(t *testing.T) {
	readiness := func(ready corev1.ConditionStatus, observedGeneration int64) v1alpha2.LLMInferenceServiceStatus {
		return v1alpha2.LLMInferenceServiceStatus{Status: duckv1.Status{
			ObservedGeneration: observedGeneration,
			Conditions:         duckv1.Conditions{{Type: apis.ConditionReady, Status: ready}},
		}}
	}

	tests := []struct {
		name       string
		stored     v1alpha2.LLMInferenceServiceStatus
		desired    v1alpha2.LLMInferenceServiceStatus
		wantEvents []string
	}{
		{
			name:    "unchanged Ready status writes nothing and emits nothing",
			stored:  readiness(corev1.ConditionTrue, 1),
			desired: readiness(corev1.ConditionTrue, 1),
		},
		{
			name:    "unchanged NotReady status writes nothing and emits nothing",
			stored:  readiness(corev1.ConditionFalse, 1),
			desired: readiness(corev1.ConditionFalse, 1),
		},
		{
			name:    "a status write that leaves Ready True emits nothing",
			stored:  readiness(corev1.ConditionTrue, 1),
			desired: readiness(corev1.ConditionTrue, 2),
		},
		{
			name:    "a status write that leaves Ready False emits nothing",
			stored:  readiness(corev1.ConditionFalse, 1),
			desired: readiness(corev1.ConditionFalse, 2),
		},
		{
			name:       "Unknown to True emits Ready",
			stored:     readiness(corev1.ConditionUnknown, 1),
			desired:    readiness(corev1.ConditionTrue, 1),
			wantEvents: []string{string(LLMInferenceServiceReadyState)},
		},
		{
			name:       "False to True emits Ready",
			stored:     readiness(corev1.ConditionFalse, 1),
			desired:    readiness(corev1.ConditionTrue, 1),
			wantEvents: []string{string(LLMInferenceServiceReadyState)},
		},
		{
			name:       "True to False emits NotReady",
			stored:     readiness(corev1.ConditionTrue, 1),
			desired:    readiness(corev1.ConditionFalse, 1),
			wantEvents: []string{string(LLMInferenceServiceNotReadyState)},
		},
		{
			name:    "True to Unknown emits nothing",
			stored:  readiness(corev1.ConditionTrue, 1),
			desired: readiness(corev1.ConditionUnknown, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha2.AddToScheme(scheme))
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Status:     tt.stored,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).WithStatusSubresource(llmSvc).Build()
			recorder := record.NewFakeRecorder(10)
			r := &LLMISVCReconciler{Client: fakeClient, EventRecorder: recorder}

			before := &v1alpha2.LLMInferenceService{}
			require.NoError(t, fakeClient.Get(t.Context(), client.ObjectKeyFromObject(llmSvc), before))
			desired := before.DeepCopy()
			desired.Status = tt.desired

			// when
			require.NoError(t, r.updateStatus(t.Context(), desired))

			// then
			close(recorder.Events)
			reasons := make([]string, 0, len(recorder.Events))
			for event := range recorder.Events {
				reasons = append(reasons, strings.Fields(event)[1])
			}
			assert.True(t, slices.Equal(tt.wantEvents, reasons), "want Events %v, got %v", tt.wantEvents, reasons)

			stored := &v1alpha2.LLMInferenceService{}
			require.NoError(t, fakeClient.Get(t.Context(), client.ObjectKeyFromObject(llmSvc), stored))
			assert.Equal(t, tt.desired.ObservedGeneration, stored.Status.ObservedGeneration)
			assert.Equal(t, tt.desired.GetCondition(apis.ConditionReady).Status, stored.Status.GetCondition(apis.ConditionReady).Status)
			// A write of an equal status still bumps resourceVersion and re-enqueues the service.
			wantWrite := !equality.Semantic.DeepEqual(tt.stored, tt.desired)
			assert.Equal(t, wantWrite, stored.ResourceVersion != before.ResourceVersion, "status written")
		})
	}
}
