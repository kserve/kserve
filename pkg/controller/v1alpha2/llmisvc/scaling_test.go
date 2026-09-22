/*
Copyright 2025 The KServe Authors.

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
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func deploymentScaleTargetRef(name string) autoscalingv2.CrossVersionObjectReference {
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       name,
	}
}

func lwsScaleTargetRef(name string) autoscalingv2.CrossVersionObjectReference {
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: lwsapi.GroupVersion.String(),
		Kind:       "LeaderWorkerSet",
		Name:       name,
	}
}

// newTestLLMISVC creates a minimal LLMInferenceService for testing.
func newTestLLMISVC(name, namespace string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.kserve.io/v1alpha2",
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid-1234",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"},
				Name: ptr.To("meta-llama/Llama-3.1-8B"),
			},
		},
	}
}

func TestPropagateScalingStatusWVAUnsupported(t *testing.T) {
	svc := newTestLLMISVC("my-model", "prod")
	var gotReason, gotMessage string
	readyCalled, unsetCalled := false, false

	err := (&LLMISVCReconciler{}).propagateScalingStatus(
		context.Background(),
		svc,
		&v1alpha2.ScalingSpec{WVA: &v1alpha2.WVASpec{}},
		"my-model-kserve-keda",
		func() { readyCalled = true },
		func(reason, message string, _ ...interface{}) {
			gotReason = reason
			gotMessage = message
		},
		func() { unsetCalled = true },
	)

	require.NoError(t, err)
	assert.False(t, readyCalled)
	assert.False(t, unsetCalled)
	assert.Equal(t, "WVAUnsupported", gotReason)
	assert.Contains(t, gotMessage, "WVA autoscaling is no longer supported")
}

func TestExpectedDirectScaledObject(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		scaleTargetRef autoscalingv2.CrossVersionObjectReference
		soName         string
		validate       func(t *testing.T, so *kedav1alpha1.ScaledObject)
	}{
		{
			name:   "uses user-defined triggers",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 5,
				KEDA: &v1alpha2.DirectKEDAScalingSpec{
					KEDAScalingSpec: v1alpha2.KEDAScalingSpec{
						PollingInterval: ptr.To(int32(30)),
					},
					Triggers: []kedav1alpha1.ScaleTriggers{
						{
							Type: "cpu",
							Metadata: map[string]string{
								"value": "80",
							},
						},
					},
				},
			},
			scaleTargetRef: deploymentScaleTargetRef("my-model-kserve"),
			soName:         "my-model-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.Spec.Triggers, 1)
				assert.Equal(t, "cpu", so.Spec.Triggers[0].Type)
				assert.Equal(t, "80", so.Spec.Triggers[0].Metadata["value"])
				assert.NotEqual(t, "prometheus", so.Spec.Triggers[0].Type)
				assert.Equal(t, int32(30), *so.Spec.PollingInterval)
				assert.Equal(t, int32(1), *so.Spec.MinReplicaCount)
				assert.Equal(t, int32(5), *so.Spec.MaxReplicaCount)
				assert.Empty(t, so.Annotations)
			},
		},
		{
			name:   "scale target ref points to deployment",
			llmSvc: newTestLLMISVC("sim-llama", "default"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 3,
				KEDA: &v1alpha2.DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "prometheus", Metadata: map[string]string{"serverAddress": "http://prom:9090", "query": "up", "threshold": "1"}},
					},
				},
			},
			scaleTargetRef: deploymentScaleTargetRef("sim-llama-kserve"),
			soName:         "sim-llama-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "apps/v1", so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "sim-llama-kserve", so.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:   "scale target ref points to LeaderWorkerSet for multi-node",
			llmSvc: newTestLLMISVC("sim-llama", "default"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 3,
				KEDA: &v1alpha2.DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "cpu", Metadata: map[string]string{"value": "80"}},
					},
				},
			},
			scaleTargetRef: lwsScaleTargetRef("sim-llama-kserve-mn"),
			soName:         "sim-llama-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.ScaleTargetRef)
				assert.Equal(t, lwsapi.GroupVersion.String(), so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "LeaderWorkerSet", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "sim-llama-kserve-mn", so.Spec.ScaleTargetRef.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := expectedDirectScaledObject(tt.llmSvc, tt.scaling, tt.scaleTargetRef, tt.soName)
			assert.Equal(t, tt.soName, so.Name)
			assert.Equal(t, tt.llmSvc.Namespace, so.Namespace)
			tt.validate(t, so)
		})
	}
}

func TestSemanticScaledObjectIsEqual(t *testing.T) {
	base := func() *kedav1alpha1.ScaledObject {
		return &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: kedav1alpha1.ScaledObjectSpec{
				MaxReplicaCount: ptr.To(int32(5)),
				Triggers: []kedav1alpha1.ScaleTriggers{
					{Type: "prometheus", Metadata: map[string]string{"query": "up"}},
				},
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticScaledObjectIsEqual(base(), base()))
	})

	t.Run("different trigger returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.Triggers[0].Metadata["query"] = "down"
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("removed optional field in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.MaxReplicaCount = nil
		assert.False(t, semanticScaledObjectIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticScaledObjectIsEqual(base(), curr))
	})
}

func TestPreserveKEDAManagedMetadata(t *testing.T) {
	hook := PreserveKEDAManagedMetadata()

	// Extract the AfterDryRunFunc from the UpdateOption by applying it to updateOptions.
	opts := &updateOptions[*kedav1alpha1.ScaledObject]{}
	hook(opts)
	require.Len(t, opts.afterDryRunFns, 1)
	fn := opts.afterDryRunFns[0]

	t.Run("copies KEDA label from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"app":                                    "test",
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, "test", expected.Labels["app"])
	})

	t.Run("no-op when curr has no KEDA label", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, map[string]string{"app": "test"}, expected.Labels)
	})

	t.Run("initializes nil labels map when KEDA label present on curr", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
	})

	t.Run("copies finalizers from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"finalizer.keda.sh"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})

	t.Run("no-op when curr has no finalizers", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{}
		fn(expected, expected.DeepCopy(), curr)
		assert.Empty(t, expected.Finalizers)
	})

	t.Run("preserves both KEDA label and finalizers together", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Labels:     map[string]string{kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so"},
				Finalizers: []string{"finalizer.keda.sh"},
			},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})
}

func TestNamingHelpers(t *testing.T) {
	svc := newTestLLMISVC("sim-llama", "llm-d-dev")

	t.Run("main deployment name (standard)", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve", mainDeploymentName(svc))
	})

	t.Run("prefill deployment name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill", prefillDeploymentName(svc))
	})

	t.Run("main HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-hpa", mainHPAName(svc))
	})

	t.Run("prefill HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-hpa", prefillHPAName(svc))
	})

	t.Run("main ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-keda", mainScaledObjectName(svc))
	})

	t.Run("prefill ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-keda", prefillScaledObjectName(svc))
	})

	t.Run("main LWS name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-mn", mainLWSName(svc))
	})

	t.Run("prefill LWS name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-mn-prefill", prefillLWSName(svc))
	})
}

func TestScaleTargetRefHelpers(t *testing.T) {
	t.Run("mainScaleTargetRef returns Deployment when no worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		ref := mainScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, mainDeploymentName(svc), ref.Name)
	})

	t.Run("mainScaleTargetRef returns LWS when worker is set", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Worker = &corev1.PodSpec{}
		ref := mainScaleTargetRef(svc)
		assert.Equal(t, lwsapi.GroupVersion.String(), ref.APIVersion)
		assert.Equal(t, "LeaderWorkerSet", ref.Kind)
		assert.Equal(t, mainLWSName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns Deployment when no prefill worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, prefillDeploymentName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns LWS when prefill worker is set", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
			Worker: &corev1.PodSpec{},
		}
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, lwsapi.GroupVersion.String(), ref.APIVersion)
		assert.Equal(t, "LeaderWorkerSet", ref.Kind)
		assert.Equal(t, prefillLWSName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns Deployment when prefill has no worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, prefillDeploymentName(svc), ref.Name)
	})
}

func newReconcilerWithHPA(hpa *autoscalingv2.HorizontalPodAutoscaler) *LLMISVCReconciler {
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)
	cb := fake.NewClientBuilder().WithScheme(scheme)
	if hpa != nil {
		cb = cb.WithObjects(hpa)
	}
	return &LLMISVCReconciler{
		Client:        cb.Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func newReconcilerWithScaledObject(so *kedav1alpha1.ScaledObject) *LLMISVCReconciler {
	scheme := runtime.NewScheme()
	_ = kedav1alpha1.AddToScheme(scheme)
	cb := fake.NewClientBuilder().WithScheme(scheme)
	if so != nil {
		cb = cb.WithObjects(so)
	}
	return &LLMISVCReconciler{
		Client:        cb.Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func TestPropagateScaledObjectStatus(t *testing.T) {
	expectedSO := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
	}

	tests := []struct {
		name         string
		so           *kedav1alpha1.ScaledObject
		wantReady    bool
		wantNotReady bool
		wantReason   string
		wantErr      bool
	}{
		{
			name: "Ready=True -> ready",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Ready=False -> not ready with reason",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: "TriggerError", Message: "prometheus query failed"},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "TriggerError",
		},
		{
			name: "Ready=Unknown -> not ready ScaledObjectProgressing",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionUnknown},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
		{
			name: "Paused=True with Ready=True -> ready (pause not surfaced yet)",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionTrue, Reason: "ScaledObjectPaused", Message: "ScaledObject is paused"},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Fallback=True with Ready=True -> ready (soft warning)",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionTrue, Message: "using fallback replicas"},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "no conditions (nil) -> not ready ScaledObjectProgressing",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
			},
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
		{
			name:         "ScaledObject not found -> not ready ScaledObjectProgressing (cache lag)",
			so:           nil,
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcilerWithScaledObject(tt.so)
			var readyCalled, notReadyCalled bool
			var notReadyReason string

			err := r.propagateScaledObjectStatus(context.Background(), expectedSO,
				func() { readyCalled = true },
				func(reason, msg string, a ...interface{}) {
					notReadyCalled = true
					notReadyReason = reason
				},
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReady, readyCalled, "ready callback")
			assert.Equal(t, tt.wantNotReady, notReadyCalled, "notReady callback")
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, notReadyReason)
			}
		})
	}
}
