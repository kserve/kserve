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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func newLLMISVCReconcilerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	return scheme
}

func TestLLMISVCReconciler_WebhookNotStarted_RequeuesWithoutGet(t *testing.T) {
	scheme := newLLMISVCReconcilerTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		IsWebhookReady: func() error {
			return errors.New("webhook server has not been started yet")
		},
	}

	// Even a request for a name that doesn't exist should short-circuit on the
	// gate before ever attempting the Get, since the gate is checked first.
	key := types.NamespacedName{Namespace: "test-ns", Name: "does-not-exist"}
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	assert.Equal(t, webhookNotReadyRequeueInterval, result.RequeueAfter,
		"expected the requeue delay used when the webhook server has not started")
}

func TestLLMISVCReconciler_WebhookStarted_ProceedsPastGate(t *testing.T) {
	scheme := newLLMISVCReconcilerTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &LLMISVCReconciler{
		Client:         fakeClient,
		EventRecorder:  record.NewFakeRecorder(10),
		IsWebhookReady: func() error { return nil },
	}

	// The target object does not exist, so once the gate passes, Reconcile
	// should hit NotFound on the Get and return a plain no-op result -
	// proving it did not take the "not ready" early-return path.
	key := types.NamespacedName{Namespace: "test-ns", Name: "does-not-exist"}
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter,
		"no artificial webhook-not-ready delay once the webhook is ready")
}
