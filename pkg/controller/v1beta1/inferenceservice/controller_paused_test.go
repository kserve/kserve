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

package inferenceservice

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

const finalizerName = "inferenceservice.finalizers"

// newWriteTrackingReconciler returns a reconciler backed by a fake client that records every write
// it receives, and a Knative discovery endpoint that refuses connections.
func newWriteTrackingReconciler(t *testing.T, isvc *v1beta1.InferenceService) (*InferenceServiceReconciler, client.Client, *[]string) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	var writes []string
	track := func(verb string) { writes = append(writes, verb) }
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(isvc).WithStatusSubresource(&v1beta1.InferenceService{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				track("create")
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				track("update")
				return c.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				track("patch")
				return c.Patch(ctx, obj, patch, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				track(subResource + " update")
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
			SubResourcePatch: func(ctx context.Context, c client.Client, subResource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				track(subResource + " patch")
				return c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	r := &InferenceServiceReconciler{
		Client:       c,
		ClientConfig: &rest.Config{Host: "http://127.0.0.1:1"},
		Clientset: kubernetesfake.NewClientset(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		}),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}
	return r, c, &writes
}

// A Ready InferenceService with auto-update disabled is not reconciled, and its status is left as
// stored unless preReconcilePlatform recorded something.
func TestReconcileSkipsPausedInferenceService(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		statusMode string
		wantWrites []string
	}{
		{
			name:       "finalizer already registered",
			finalizers: []string{finalizerName},
			statusMode: string(constants.Standard),
		},
		{
			name:       "finalizer is registered before the pause takes effect",
			statusMode: string(constants.Standard),
			wantWrites: []string{"patch"},
		},
		{
			name:       "status records the legacy RawDeployment mode",
			finalizers: []string{finalizerName},
			statusMode: string(constants.LegacyRawDeployment),
		},
		{
			name:       "status records the legacy Serverless mode",
			finalizers: []string{finalizerName},
			statusMode: string(constants.LegacyServerless),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "paused",
					Namespace:   "default",
					Finalizers:  tt.finalizers,
					Annotations: map[string]string{constants.DisableAutoUpdateAnnotationKey: "true"},
				},
			}
			isvc.Status.InitializeConditions()
			isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{Status: corev1.ConditionTrue})
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{Status: corev1.ConditionTrue})
			isvc.Status.DeploymentMode = tt.statusMode
			require.True(t, isvc.Status.IsReady())

			r, c, writes := newWriteTrackingReconciler(t, isvc)

			// when
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: types.NamespacedName{Name: isvc.Name, Namespace: isvc.Namespace}})

			// then
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)
			assert.Equal(t, tt.wantWrites, *writes)

			persisted := &v1beta1.InferenceService{}
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(isvc), persisted))
			assert.Equal(t, []string{finalizerName}, persisted.Finalizers)
			assert.True(t, persisted.Status.IsReady())
			assert.Equal(t, tt.statusMode, persisted.Status.DeploymentMode)
		})
	}
}

// The first reconcile of a new InferenceService registers the finalizer, whose Patch response
// replaces the in-memory status. Status must be initialized after it, so an early failure still
// persists the initial conditions.
func TestReconcileInitializesStatusBeforeEarlyReturn(t *testing.T) {
	// given
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "fresh",
			Namespace:   "default",
			Annotations: map[string]string{constants.DeploymentMode: string(constants.Knative)},
		},
	}
	r, c, writes := newWriteTrackingReconciler(t, isvc)

	// when
	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: types.NamespacedName{Name: isvc.Name, Namespace: isvc.Namespace}})

	// then
	require.Error(t, err, "Knative CRD discovery is expected to fail")
	assert.Equal(t, []string{"patch", "status update"}, *writes)

	persisted := &v1beta1.InferenceService{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(isvc), persisted))
	assert.Equal(t, []string{finalizerName}, persisted.Finalizers)
	for _, conditionType := range []apis.ConditionType{apis.ConditionReady, v1beta1.PredictorReady, v1beta1.IngressReady} {
		condition := persisted.Status.GetCondition(conditionType)
		if assert.NotNil(t, condition, "condition %s", conditionType) {
			assert.Equal(t, corev1.ConditionUnknown, condition.Status, "condition %s", conditionType)
		}
	}
}
