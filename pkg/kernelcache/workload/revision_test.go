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

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

func TestResolveDeploymentBackedInferenceServiceRevision(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*deploymentBackedWorkloadFixture)
		wantID     string
		wantError  string
		wantResult bool
	}{
		{
			name:       "resolves matching pod and replica set revision",
			wantID:     "abc123",
			wantResult: true,
		},
		{
			name: "rejects mismatched pod and replica set revisions",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = "different"
			},
			wantError: "does not match ReplicaSet revision",
		},
		{
			name: "uses replica set revision when pod revision is missing",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				delete(fixture.pod.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
			},
			wantID:     "abc123",
			wantResult: true,
		},
		{
			name: "returns no result when both revisions are missing",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				delete(fixture.pod.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
				delete(fixture.replicaSet.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDeploymentBackedWorkloadFixture(t)
			if tc.mutate != nil {
				tc.mutate(fixture)
				fixture.rebuildReader()
			}

			revision, err := ResolveDeploymentBackedInferenceServiceRevision(context.Background(), fixture.reader, fixture.pod)
			if tc.wantError != "" {
				require.EqualError(t, err, "pod revision \"different\" "+tc.wantError+" \"abc123\"")
				require.Nil(t, revision)
				return
			}
			require.NoError(t, err)
			if !tc.wantResult {
				require.Nil(t, revision)
				return
			}
			require.NotNil(t, revision)
			require.Equal(t, fixture.inferenceService, revision.Source)
			require.Equal(t, fixture.replicaSet, revision.ReplicaSet)
			require.Equal(t, tc.wantID, revision.RevisionID)
		})
	}
}
