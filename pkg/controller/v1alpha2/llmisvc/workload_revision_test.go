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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
)

func TestComputeWorkloadRevision(t *testing.T) {
	roles := []workloadRevisionRole{
		{
			Name: "decode",
			DeploymentTemplate: &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"team": "inference"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "vllm:v1"}}},
			},
		},
		{
			Name: "prefill",
			DeploymentTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "vllm:v1"}}},
			},
		},
	}

	revision, err := computeWorkloadRevision(roles)
	require.NoError(t, err)
	assert.Len(t, revision, workloadRevisionLength)

	sameRevision, err := computeWorkloadRevision(roles)
	require.NoError(t, err)
	assert.Equal(t, revision, sameRevision)

	changed := deepCopyWorkloadRevisionRoles(roles)
	changed[1].DeploymentTemplate.Spec.Containers[0].Image = "vllm:v2"
	changedRevision, err := computeWorkloadRevision(changed)
	require.NoError(t, err)
	assert.NotEqual(t, revision, changedRevision)

	withRevisionLabel := deepCopyWorkloadRevisionRoles(roles)
	withRevisionLabel[0].DeploymentTemplate.Labels[constants.LLMInferenceServiceRevisionLabelKey] = "previous"
	withRevisionLabel[1].DeploymentTemplate.Labels = map[string]string{
		constants.LLMInferenceServiceRevisionLabelKey: "previous",
	}
	ignoredLabelRevision, err := computeWorkloadRevision(withRevisionLabel)
	require.NoError(t, err)
	assert.Equal(t, revision, ignoredLabelRevision, "the generated label must not feed back into its own hash")
}

func TestApplyWorkloadRevisionDoesNotMutateSharedLabels(t *testing.T) {
	sharedLabels := map[string]string{"app": "pd"}
	selectorLabels := sharedLabels
	template := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: sharedLabels},
	}

	applyWorkloadRevision(template, &Config{WorkloadRevision: "revision-2"})

	assert.Equal(t, "revision-2", template.Labels[constants.LLMInferenceServiceRevisionLabelKey])
	assert.NotContains(t, selectorLabels, constants.LLMInferenceServiceRevisionLabelKey,
		"the generated revision must not leak into an immutable workload selector")
}

func TestSingleNodeWorkloadsUseSharedRevision(t *testing.T) {
	r := &LLMISVCReconciler{
		Client:    selectorTestClient(t),
		Clientset: k8sfake.NewSimpleClientset(),
	}
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "revision-test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Labels: map[string]string{constants.LLMInferenceServiceRevisionLabelKey: "user-value"},
			},
			Prefill: &v1alpha2.WorkloadSpec{
				Labels: map[string]string{constants.LLMInferenceServiceRevisionLabelKey: "different-user-value"},
			},
		},
	}
	cfg := &Config{}
	require.NoError(t, r.reconcileWorkloadRevision(context.Background(), svc, cfg))
	require.NotEmpty(t, cfg.WorkloadRevision)
	generatedRevision := cfg.WorkloadRevision

	decode, err := r.expectedSingleNodeMainDeployment(context.Background(), svc, cfg)
	require.NoError(t, err)
	prefill, err := r.expectedPrefillMainDeployment(context.Background(), svc, cfg)
	require.NoError(t, err)

	assert.Equal(t, generatedRevision, decode.Spec.Template.Labels[constants.LLMInferenceServiceRevisionLabelKey])
	assert.Equal(t, generatedRevision, prefill.Spec.Template.Labels[constants.LLMInferenceServiceRevisionLabelKey])
	assert.NotContains(t, decode.Spec.Selector.MatchLabels, constants.LLMInferenceServiceRevisionLabelKey)
	assert.NotContains(t, prefill.Spec.Selector.MatchLabels, constants.LLMInferenceServiceRevisionLabelKey)

	decodeReplicas, prefillReplicas := int32(10), int32(3)
	svc.Spec.Replicas = &decodeReplicas
	svc.Spec.Prefill.Replicas = &prefillReplicas
	scaledCfg := &Config{}
	require.NoError(t, r.reconcileWorkloadRevision(context.Background(), svc, scaledCfg))
	assert.Equal(t, generatedRevision, scaledCfg.WorkloadRevision, "replica changes must not create a new revision")
}

func TestMultiNodeWorkloadsUseSharedRevision(t *testing.T) {
	r := &LLMISVCReconciler{Client: newFakeClientWithLWSNoMatch(t)}
	svc := newLLMInferenceServiceWithMultiNode("revision-test", "default")
	prefill := svc.Spec.WorkloadSpec
	prefill.Template = svc.Spec.Template.DeepCopy()
	prefill.Worker = svc.Spec.Worker.DeepCopy()
	svc.Spec.Prefill = &prefill
	cfg := &Config{CredentialConfig: &credentials.CredentialConfig{}}

	require.NoError(t, r.reconcileWorkloadRevision(context.Background(), svc, cfg))
	require.NotEmpty(t, cfg.WorkloadRevision)

	decode, err := r.expectedMainMultiNodeLWS(context.Background(), svc, cfg)
	require.NoError(t, err)
	prefillLWS, err := r.expectedPrefillMultiNodeLWS(context.Background(), svc, cfg)
	require.NoError(t, err)

	for _, template := range []*corev1.PodTemplateSpec{
		decode.Spec.LeaderWorkerTemplate.LeaderTemplate,
		&decode.Spec.LeaderWorkerTemplate.WorkerTemplate,
		prefillLWS.Spec.LeaderWorkerTemplate.LeaderTemplate,
		&prefillLWS.Spec.LeaderWorkerTemplate.WorkerTemplate,
	} {
		require.NotNil(t, template)
		assert.Equal(t, cfg.WorkloadRevision, template.Labels[constants.LLMInferenceServiceRevisionLabelKey])
	}
}

func deepCopyWorkloadRevisionRoles(in []workloadRevisionRole) []workloadRevisionRole {
	out := make([]workloadRevisionRole, len(in))
	for i := range in {
		out[i].Name = in[i].Name
		if in[i].DeploymentTemplate != nil {
			out[i].DeploymentTemplate = in[i].DeploymentTemplate.DeepCopy()
		}
		if in[i].LeaderWorkerTemplate != nil {
			out[i].LeaderWorkerTemplate = in[i].LeaderWorkerTemplate.DeepCopy()
		}
	}
	return out
}
