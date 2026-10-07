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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveDeploymentBackedInferenceService(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*deploymentBackedWorkloadFixture)
		wantResult bool
	}{
		{
			name:       "resolves deployment ownership chain",
			wantResult: true,
		},
		{
			name: "rejects replica set UID mismatch",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.replicaSet.UID = "different-replicaset-uid"
			},
		},
		{
			name: "rejects deployment UID mismatch",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.deployment.UID = "different-deployment-uid"
			},
		},
		{
			name: "rejects inference service UID mismatch",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.inferenceService.UID = "different-inferenceservice-uid"
			},
		},
		{
			name: "rejects non controller owner",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.pod.OwnerReferences[0].Controller = nil
			},
		},
		{
			name: "rejects wrong owner API version",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.pod.OwnerReferences[0].APIVersion = "apps/v1beta1"
			},
		},
		{
			name: "rejects missing deployment owner",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.replicaSet.OwnerReferences = nil
			},
		},
		{
			name: "rejects missing inference service owner",
			mutate: func(fixture *deploymentBackedWorkloadFixture) {
				fixture.deployment.OwnerReferences = nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDeploymentBackedWorkloadFixture(t)
			if tc.mutate != nil {
				tc.mutate(fixture)
				fixture.rebuildReader()
			}

			resolved, err := ResolveDeploymentBackedInferenceService(context.Background(), fixture.reader, fixture.pod)
			require.NoError(t, err)
			if tc.wantResult {
				require.Equal(t, fixture.inferenceService, resolved)
			} else {
				require.Nil(t, resolved)
			}
		})
	}
}

func TestResolveDeploymentBackedInferenceServiceReturnsReaderError(t *testing.T) {
	fixture := newDeploymentBackedWorkloadFixture(t)
	fixture.reader = fake.NewClientBuilder().WithScheme(fixture.scheme).Build()

	_, err := ResolveDeploymentBackedInferenceService(context.Background(), fixture.reader, fixture.pod)
	require.Error(t, err)
}
