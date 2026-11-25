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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestSchedulerGenerationTarget(t *testing.T) {
	manual := v1alpha2.SchedulerGenerationsSpec{Promotion: v1alpha2.SchedulerGenerationPromotionManual}
	tests := []struct {
		name       string
		choice     schedulerGenerationChoice
		want       string
		pinnedRuns bool
	}{
		{
			name:   "first generation serves at once: nothing else could",
			choice: schedulerGenerationChoice{current: "new", running: sets.New("new")},
			want:   "new", pinnedRuns: true,
		},
		{
			name:   "automatic: the serving generation stays while the new one is not available",
			choice: schedulerGenerationChoice{current: "new", serving: "old", running: sets.New("new", "old")},
			want:   "old", pinnedRuns: true,
		},
		{
			name:   "automatic: the new generation serves once available",
			choice: schedulerGenerationChoice{current: "new", currentAvailable: true, serving: "old", running: sets.New("new", "old")},
			want:   "new", pinnedRuns: true,
		},
		{
			name:   "manual: the serving generation stays even when the new one is available",
			choice: schedulerGenerationChoice{current: "new", currentAvailable: true, serving: "old", running: sets.New("new", "old"), spec: manual},
			want:   "old", pinnedRuns: true,
		},
		{
			name: "manual: serving promotes",
			choice: schedulerGenerationChoice{
				current: "new", serving: "old", running: sets.New("new", "old"),
				spec: v1alpha2.SchedulerGenerationsSpec{Promotion: v1alpha2.SchedulerGenerationPromotionManual, Serving: "new"},
			},
			want: "new", pinnedRuns: true,
		},
		{
			name: "serving rolls back to a retained generation while the spec is unchanged",
			choice: schedulerGenerationChoice{
				current: "new", currentAvailable: true, serving: "new", running: sets.New("new", "old"),
				spec: v1alpha2.SchedulerGenerationsSpec{Serving: "old"},
			},
			want: "old", pinnedRuns: true,
		},
		{
			name: "serving names a generation that does not run: the Service stays",
			choice: schedulerGenerationChoice{
				current: "new", currentAvailable: true, serving: "old", running: sets.New("new", "old"),
				spec: v1alpha2.SchedulerGenerationsSpec{Serving: "gone"},
			},
			want: "old", pinnedRuns: false,
		},
		{
			name:   "the serving generation was deleted: the current one serves",
			choice: schedulerGenerationChoice{current: "new", serving: "gone", running: sets.New("new")},
			want:   "new", pinnedRuns: true,
		},
		{
			name:   "from another strategy: the scheduler Deployment serves until the first generation is available",
			choice: schedulerGenerationChoice{current: "new", legacyRunning: true, running: sets.New("new")},
			want:   "", pinnedRuns: true,
		},
		{
			name:   "from another strategy: the first generation serves once available",
			choice: schedulerGenerationChoice{current: "new", currentAvailable: true, legacyRunning: true, running: sets.New("new")},
			want:   "new", pinnedRuns: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, pinnedRuns := tt.choice.target()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.pinnedRuns, pinnedRuns)
		})
	}
}

func TestNextSchedulerGenerationHistory(t *testing.T) {
	assert.Equal(t, []string{"b", "a"}, nextSchedulerGenerationHistory([]string{"a"}, "b", "c"), "the generation that served goes first")
	assert.Equal(t, []string{"a"}, nextSchedulerGenerationHistory([]string{"a"}, "c", "c"), "no move, no change")
	assert.Equal(t, []string{"c"}, nextSchedulerGenerationHistory([]string{"b", "c"}, "", "b"), "the target leaves the history")
	assert.Equal(t, []string{"a"}, nextSchedulerGenerationHistory([]string{"a", "b"}, "a", "b"), "a rollback swaps the two")

	long := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	assert.Len(t, nextSchedulerGenerationHistory(long, "0", "x"), maxSchedulerGenerationHistory)
}

func TestRetainedSchedulerGenerations(t *testing.T) {
	running := sets.New("cur", "srv", "h1", "h2", "h3", "orphan")
	choice := schedulerGenerationChoice{current: "cur", running: running}

	assert.Equal(t, sets.New("cur", "srv", "h1"), retainedSchedulerGenerations(choice, "srv", []string{"h1", "h2", "h3"}), "one previous generation by default")

	choice.spec.RetainedGenerations = ptr.To[int32](0)
	assert.Equal(t, sets.New("cur", "srv"), retainedSchedulerGenerations(choice, "srv", []string{"h1"}))

	choice.spec.RetainedGenerations = ptr.To[int32](2)
	assert.Equal(t, sets.New("cur", "srv", "h2", "h3"), retainedSchedulerGenerations(choice, "srv", []string{"gone", "h2", "h3", "h1"}), "generations that no longer run are skipped")

	choice.spec.RetainedGenerations = nil
	choice.spec.Serving = "orphan"
	assert.True(t, retainedSchedulerGenerations(choice, "orphan", nil).Has("orphan"))
}

func TestAsSchedulerGeneration(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "ns"}}
	newDeployment := func(args ...string) *appsv1.Deployment {
		labels := map[string]string{"app.kubernetes.io/name": "llm"}
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: schedulerDeploymentName(svc), Labels: labels},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "llm"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "llm"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Args: args}}},
				},
			},
		}
	}

	d := newDeployment("--pool-name=llm-inference-pool", "--ha-enable-leader-election")
	hash, err := schedulerGenerationHash(&d.Spec.Template)
	require.NoError(t, err)
	again, err := schedulerGenerationHash(&newDeployment("--pool-name=llm-inference-pool", "--ha-enable-leader-election").Spec.Template)
	require.NoError(t, err)
	assert.Equal(t, hash, again, "the same template is the same generation")
	other, err := schedulerGenerationHash(&newDeployment("--pool-name=llm-inference-pool").Spec.Template)
	require.NoError(t, err)
	assert.NotEqual(t, hash, other)

	asSchedulerGeneration(svc, d, hash, v1alpha2.SchedulerGenerationsSpec{MinReadySeconds: 20})
	assert.Equal(t, "llm-kserve-router-scheduler-"+hash, d.Name)
	assert.Equal(t, hash, d.Labels[SchedulerGenerationLabelKey])
	assert.Equal(t, hash, d.Spec.Selector.MatchLabels[SchedulerGenerationLabelKey])
	assert.Equal(t, hash, d.Spec.Template.Labels[SchedulerGenerationLabelKey])
	assert.Equal(t, int32(20), d.Spec.MinReadySeconds)
	assert.Contains(t, d.Spec.Template.Spec.Containers[0].Args, "--ha-lease-name=llm-epp-"+hash+".llm-d.ai",
		"each leader-elected generation elects its own leader")
	assert.True(t, deploymentRunsLeaderElection(d))

	activeActive := newDeployment("--pool-name=llm-inference-pool")
	asSchedulerGeneration(svc, activeActive, other, v1alpha2.SchedulerGenerationsSpec{})
	assert.Equal(t, []string{"--pool-name=llm-inference-pool"}, activeActive.Spec.Template.Spec.Containers[0].Args)

	ownLease := newDeployment("--ha-enable-leader-election", "--ha-lease-name", "mine")
	asSchedulerGeneration(svc, ownLease, hash, v1alpha2.SchedulerGenerationsSpec{})
	assert.Equal(t, []string{"--ha-enable-leader-election", "--ha-lease-name", "mine"}, ownLease.Spec.Template.Spec.Containers[0].Args)
}

func TestSchedulerGenerationAvailable(t *testing.T) {
	deployment := func(replicas, updated, available int32, args ...string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Generation: 2},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(replicas),
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Args: args}}}},
			},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: updated, AvailableReplicas: available},
		}
	}
	assert.True(t, schedulerGenerationAvailable(deployment(2, 2, 2)))
	assert.False(t, schedulerGenerationAvailable(deployment(2, 2, 1)), "every active-active replica has to be available")
	assert.True(t, schedulerGenerationAvailable(deployment(2, 2, 1, "--ha-enable-leader-election")), "a leader-elected generation is available with its leader")
	assert.False(t, schedulerGenerationAvailable(deployment(2, 2, 0, "--ha-enable-leader-election")))

	stale := deployment(2, 2, 2)
	stale.Status.ObservedGeneration = 1
	assert.False(t, schedulerGenerationAvailable(stale), "status of an older spec")
}

func TestSchedulerArgEnablesLeaderElection(t *testing.T) {
	for arg, want := range map[string]bool{
		"--ha-enable-leader-election":       true,
		"-ha-enable-leader-election":        true,
		"--ha-enable-leader-election=true":  true,
		"--ha-enable-leader-election=1":     true,
		"--ha-enable-leader-election=false": false,
		"--ha-enable-leader-election=0":     false,
		"ha-enable-leader-election":         false,
		"--ha-enable-leader-election-other": false,
		"--pool-name":                       false,
	} {
		assert.Equal(t, want, schedulerArgEnablesLeaderElection(arg), arg)
	}
}
