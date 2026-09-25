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
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

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

	changed := deepCopyRevisionRoles(roles)
	changed[1].DeploymentTemplate.Spec.Containers[0].Image = "vllm:v2"
	changedRevision, err := computeWorkloadRevision(changed)
	require.NoError(t, err)
	assert.NotEqual(t, revision, changedRevision)

	withRevisionLabel := deepCopyRevisionRoles(roles)
	withRevisionLabel[0].DeploymentTemplate.Labels[constants.LLMInferenceServiceRevisionLabelKey] = "previous"
	withRevisionLabel[1].DeploymentTemplate.Labels = map[string]string{
		constants.LLMInferenceServiceRevisionLabelKey: "previous",
	}
	ignoredLabelRevision, err := computeWorkloadRevision(withRevisionLabel)
	require.NoError(t, err)
	assert.Equal(t, revision, ignoredLabelRevision, "the generated label must not feed back into its own hash")
}

func TestWorkloadRevisionEnabled(t *testing.T) {
	revisionLabel := map[string]string{
		constants.LLMInferenceServiceRevisionLabelKey: "",
	}
	tests := []struct {
		name string
		svc  *v1alpha2.LLMInferenceService
		want bool
	}{
		{name: "nil service"},
		{
			name: "non-disaggregated service",
			svc: &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{Labels: revisionLabel},
			}},
		},
		{
			name: "neither role opts in",
			svc: &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
				Prefill: &v1alpha2.WorkloadSpec{},
			}},
		},
		{
			name: "decode only",
			svc: &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{Labels: revisionLabel},
				Prefill:      &v1alpha2.WorkloadSpec{},
			}},
		},
		{
			name: "prefill only",
			svc: &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
				Prefill: &v1alpha2.WorkloadSpec{Labels: revisionLabel},
			}},
		},
		{
			name: "both roles opt in",
			svc: &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{Labels: revisionLabel},
				Prefill:      &v1alpha2.WorkloadSpec{Labels: revisionLabel},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, workloadRevisionEnabled(tt.svc))
		})
	}
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
	svcWithoutOptIn := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "revision-test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Prefill: &v1alpha2.WorkloadSpec{},
		},
	}
	disabledCfg := &Config{}
	require.NoError(t, r.reconcileWorkloadRevision(context.Background(), svcWithoutOptIn, disabledCfg))
	assert.Empty(t, disabledCfg.WorkloadRevision,
		"services using configs without the revision placeholder must not roll out on controller upgrade")

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
	svc.Spec.Labels = map[string]string{constants.LLMInferenceServiceRevisionLabelKey: ""}
	svc.Spec.Prefill.Labels = map[string]string{constants.LLMInferenceServiceRevisionLabelKey: ""}
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

	roles := []workloadRevisionRole{
		leaderWorkerRevisionRole(constants.LLMDRoleDecode, &decode.Spec.LeaderWorkerTemplate),
		leaderWorkerRevisionRole(constants.LLMDRolePrefill, &prefillLWS.Spec.LeaderWorkerTemplate),
	}
	revision, err := computeWorkloadRevision(roles)
	require.NoError(t, err)

	policyOnlyChange := decode.Spec.LeaderWorkerTemplate.DeepCopy()
	policyOnlyChange.RestartPolicy = ""
	subGroupSize := int32(1)
	policyOnlyChange.SubGroupPolicy = &lwsapi.SubGroupPolicy{SubGroupSize: &subGroupSize}
	policyRoles := deepCopyRevisionRoles(roles)
	policyRoles[0] = leaderWorkerRevisionRole(constants.LLMDRoleDecode, policyOnlyChange)
	policyOnlyRevision, err := computeWorkloadRevision(policyRoles)
	require.NoError(t, err)
	assert.Equal(t, revision, policyOnlyRevision)

	sizeChange := deepCopyRevisionRoles(roles)
	newSize := *sizeChange[0].Size + 1
	sizeChange[0].Size = &newSize
	sizeRevision, err := computeWorkloadRevision(sizeChange)
	require.NoError(t, err)
	assert.NotEqual(t, revision, sizeRevision, "worker-group size changes must create a new revision")
}
