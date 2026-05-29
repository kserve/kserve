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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func newRolloutTestReconciler(objs ...runtime.Object) *LLMISVCReconciler {
	scheme := runtime.NewScheme()
	_ = v1alpha2.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		b = b.WithRuntimeObjects(o)
	}
	return &LLMISVCReconciler{Client: b.Build()}
}

func deploymentWithAvailable(name string, available corev1.ConditionStatus) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: available},
			},
		},
	}
}

func llmSvc(ns, name string, withPrefill bool) *v1alpha2.LLMInferenceService {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if withPrefill {
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{Template: &corev1.PodSpec{}}
	}
	return svc
}

func TestIsWorkloadRolling(t *testing.T) {
	const ns = "default"
	const svcName = "my-llm"
	mainName := svcName + "-kserve"
	prefillName := svcName + "-kserve-prefill"

	tests := []struct {
		name        string
		deployments []*appsv1.Deployment
		withPrefill bool
		want        bool
	}{
		{
			name:        "fully available — not rolling",
			deployments: []*appsv1.Deployment{deploymentWithAvailable(mainName, corev1.ConditionTrue)},
			want:        false,
		},
		{
			name:        "Available=False — not yet ready",
			deployments: []*appsv1.Deployment{deploymentWithAvailable(mainName, corev1.ConditionFalse)},
			want:        true,
		},
		{
			name: "no Available condition yet (brand new deployment)",
			deployments: []*appsv1.Deployment{{
				ObjectMeta: metav1.ObjectMeta{Name: mainName, Namespace: "default"},
			}},
			want: true,
		},
		{
			name: "main available, prefill not yet ready",
			deployments: []*appsv1.Deployment{
				deploymentWithAvailable(mainName, corev1.ConditionTrue),
				deploymentWithAvailable(prefillName, corev1.ConditionFalse),
			},
			withPrefill: true,
			want:        true,
		},
		{
			name: "both main and prefill available",
			deployments: []*appsv1.Deployment{
				deploymentWithAvailable(mainName, corev1.ConditionTrue),
				deploymentWithAvailable(prefillName, corev1.ConditionTrue),
			},
			withPrefill: true,
			want:        false,
		},
		{
			name:        "deployment not found — treated as not rolling",
			deployments: []*appsv1.Deployment{},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, len(tt.deployments))
			for i, d := range tt.deployments {
				objs[i] = d
			}
			r := newRolloutTestReconciler(objs...)
			svc := llmSvc(ns, svcName, tt.withPrefill)

			got, err := r.isWorkloadRolling(t.Context(), svc)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
