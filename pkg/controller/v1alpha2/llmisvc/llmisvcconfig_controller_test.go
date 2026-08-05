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
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

var configFinalizerNameForTest = constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"

// fakeWebhookServer is a minimal webhook.Server stub used to control the
// StartedChecker result in tests without spinning up a real HTTPS listener.
type fakeWebhookServer struct {
	started bool
}

var _ webhook.Server = &fakeWebhookServer{}

func (f *fakeWebhookServer) NeedLeaderElection() bool          { return false }
func (f *fakeWebhookServer) Register(_ string, _ http.Handler) {}
func (f *fakeWebhookServer) Start(_ context.Context) error     { return nil }
func (f *fakeWebhookServer) WebhookMux() *http.ServeMux        { return nil }

func (f *fakeWebhookServer) StartedChecker() healthz.Checker {
	return func(_ *http.Request) error {
		if !f.started {
			return errors.New("webhook server has not been started yet")
		}
		return nil
	}
}

func newConfigReconcilerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	return scheme
}

func newTestLLMISVCConfig(name, namespace string) *v1alpha2.LLMInferenceServiceConfig {
	return &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestLLMISVCConfigReconciler_WebhookNotStarted_RequeuesWithoutMutating(t *testing.T) {
	scheme := newConfigReconcilerTestScheme(t)
	config := newTestLLMISVCConfig("my-config", "test-ns")
	key := types.NamespacedName{Namespace: config.Namespace, Name: config.Name}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(config).Build()
	r := &LLMISVCConfigReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		WebhookServer: &fakeWebhookServer{started: false},
	}

	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	assert.Equal(t, webhookNotReadyRequeueInterval, result.RequeueAfter,
		"expected the requeue delay used when the webhook server has not started")

	current := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, fakeClient.Get(t.Context(), key, current))
	assert.False(t, controllerutil.ContainsFinalizer(current, configFinalizerNameForTest),
		"finalizer must not be added while the webhook server has not started")
}

func TestLLMISVCConfigReconciler_WebhookStarted_AddsFinalizer(t *testing.T) {
	scheme := newConfigReconcilerTestScheme(t)
	config := newTestLLMISVCConfig("my-config", "test-ns")
	key := types.NamespacedName{Namespace: config.Namespace, Name: config.Name}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(config).Build()
	r := &LLMISVCConfigReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		WebhookServer: &fakeWebhookServer{started: true},
	}

	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "no artificial delay once the webhook is ready")

	current := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, fakeClient.Get(t.Context(), key, current))
	assert.True(t, controllerutil.ContainsFinalizer(current, configFinalizerNameForTest),
		"finalizer should be added once the webhook server is ready")
}

func TestLLMISVCConfigReconciler_NilWebhookServer_BehavesAsUngated(t *testing.T) {
	scheme := newConfigReconcilerTestScheme(t)
	config := newTestLLMISVCConfig("my-config", "test-ns")
	key := types.NamespacedName{Namespace: config.Namespace, Name: config.Name}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(config).Build()
	r := &LLMISVCConfigReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		// WebhookServer intentionally left nil, matching callers (e.g. test fixtures)
		// that don't wire one in.
	}

	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "a nil WebhookServer must not gate reconciliation")

	current := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, fakeClient.Get(t.Context(), key, current))
	assert.True(t, controllerutil.ContainsFinalizer(current, configFinalizerNameForTest),
		"finalizer should be added when no webhook readiness gate is configured")
}
