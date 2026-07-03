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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kserveapiv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestResolveRuntimeSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, kserveapiv1alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	csr := &kserveapiv1alpha1.ClusterServingRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "kserve-llm-sglang"},
		Spec: kserveapiv1alpha1.ServingRuntimeSpec{
			ServingRuntimePodSpec: kserveapiv1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "lmsysorg/sglang:v0.5.14"},
				},
			},
		},
	}
	sr := &kserveapiv1alpha1.ServingRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "kserve-llm-sglang", Namespace: "team-a"},
		Spec: kserveapiv1alpha1.ServingRuntimeSpec{
			ServingRuntimePodSpec: kserveapiv1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "team-a/sglang:custom"},
				},
			},
		},
	}

	tests := []struct {
		name      string
		runtime   *string
		namespace string
		objects   []runtime.Object
		wantNil   bool
		wantImage string
	}{
		{
			name:      "nil runtime returns nil spec",
			runtime:   nil,
			namespace: "default",
			objects:   []runtime.Object{csr},
			wantNil:   true,
		},
		{
			name:      "empty runtime returns nil spec",
			runtime:   ptr.To(""),
			namespace: "default",
			objects:   []runtime.Object{csr},
			wantNil:   true,
		},
		{
			name:      "unknown runtime falls through silently",
			runtime:   ptr.To("does-not-exist"),
			namespace: "default",
			objects:   []runtime.Object{csr},
			wantNil:   true,
		},
		{
			name:      "ClusterServingRuntime resolves when no namespaced SR",
			runtime:   ptr.To("kserve-llm-sglang"),
			namespace: "default",
			objects:   []runtime.Object{csr},
			wantImage: "lmsysorg/sglang:v0.5.14",
		},
		{
			name:      "namespaced ServingRuntime wins over ClusterServingRuntime with same name",
			runtime:   ptr.To("kserve-llm-sglang"),
			namespace: "team-a",
			objects:   []runtime.Object{csr, sr},
			wantImage: "team-a/sglang:custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()
			r := &LLMISVCReconciler{Client: c}
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: tt.namespace},
				Spec:       v1alpha2.LLMInferenceServiceSpec{Runtime: tt.runtime},
			}

			spec, err := r.ResolveRuntimeSpec(t.Context(), llmSvc)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, spec)
				return
			}

			require.NotNil(t, spec)
			require.NotNil(t, spec.Template)
			require.Len(t, spec.Template.Containers, 1)
			assert.Equal(t, "main", spec.Template.Containers[0].Name)
			assert.Equal(t, tt.wantImage, spec.Template.Containers[0].Image)
		})
	}
}
