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

package pdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = policyv1.AddToScheme(s)
	_ = v1beta1.AddToScheme(s)
	return s
}

var (
	minAvailable1   = intstr.FromInt32(1)
	maxUnavailable1 = intstr.FromInt32(1)

	testMeta = metav1.ObjectMeta{
		Name:      "sklearn-iris-predictor",
		Namespace: "default",
		Labels: map[string]string{
			constants.InferenceServicePodLabelKey: "sklearn-iris",
			constants.KServiceComponentLabel:      "predictor",
		},
	}
)

// trackingClient wraps a fake client and records the last mutating action taken,
// following the same pattern as the HPA reconciler tests.
type trackingClient struct {
	client.Client
	actualAction *string
}

func (c *trackingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	*c.actualAction = "create"
	return c.Client.Create(ctx, obj, opts...)
}

func (c *trackingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	*c.actualAction = "update"
	return c.Client.Update(ctx, obj, opts...)
}

func (c *trackingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	*c.actualAction = "delete"
	return c.Client.Delete(ctx, obj, opts...)
}

func TestCreatePDB(t *testing.T) {
	tests := []struct {
		name         string
		componentExt *v1beta1.ComponentExtensionSpec
		wantNil      bool
		wantSpec     *policyv1.PodDisruptionBudgetSpec
	}{
		{
			name:         "nil componentExt returns nil PDB",
			componentExt: nil,
			wantNil:      true,
		},
		{
			name:         "nil PodDisruptionBudget field returns nil PDB",
			componentExt: &v1beta1.ComponentExtensionSpec{},
			wantNil:      true,
		},
		{
			name: "minAvailable set",
			componentExt: &v1beta1.ComponentExtensionSpec{
				PodDisruptionBudget: &policyv1.PodDisruptionBudgetSpec{
					MinAvailable: &minAvailable1,
				},
			},
			wantNil: false,
			wantSpec: &policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &minAvailable1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.InferenceServicePodLabelKey: "sklearn-iris",
						constants.KServiceComponentLabel:      "predictor",
					},
				},
			},
		},
		{
			name: "maxUnavailable set",
			componentExt: &v1beta1.ComponentExtensionSpec{
				PodDisruptionBudget: &policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable1,
				},
			},
			wantNil: false,
			wantSpec: &policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailable1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.InferenceServicePodLabelKey: "sklearn-iris",
						constants.KServiceComponentLabel:      "predictor",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createPDB(testMeta, tt.componentExt)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, testMeta.Name, got.Name)
			assert.Equal(t, testMeta.Namespace, got.Namespace)
			assert.Equal(t, *tt.wantSpec, got.Spec)
		})
	}
}

func TestCheckPDBExist(t *testing.T) {
	s := scheme()

	existing := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMeta.Name,
			Namespace: testMeta.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable1,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.InferenceServicePodLabelKey: "sklearn-iris",
					constants.KServiceComponentLabel:      "predictor",
				},
			},
		},
	}

	tests := []struct {
		name       string
		existing   *policyv1.PodDisruptionBudget
		desired    *policyv1.PodDisruptionBudget
		wantResult constants.CheckResultType
	}{
		{
			name:       "returns Skipped when not found and no desired PDB",
			existing:   nil,
			desired:    nil,
			wantResult: constants.CheckResultSkipped,
		},
		{
			name:       "returns Create when not found and selector set",
			existing:   nil,
			desired:    existing.DeepCopy(),
			wantResult: constants.CheckResultCreate,
		},
		{
			name:       "returns Existed when spec matches",
			existing:   existing.DeepCopy(),
			desired:    existing.DeepCopy(),
			wantResult: constants.CheckResultExisted,
		},
		{
			name:       "returns Delete when existing present and desired is nil",
			existing:   existing.DeepCopy(),
			desired:    nil,
			wantResult: constants.CheckResultDelete,
		},
		{
			name:     "returns Update when spec differs",
			existing: existing.DeepCopy(),
			desired: func() *policyv1.PodDisruptionBudget {
				d := existing.DeepCopy()
				updated := intstr.FromInt32(2)
				d.Spec.MinAvailable = &updated
				return d
			}(),
			wantResult: constants.CheckResultUpdate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(s)
			if tt.existing != nil {
				builder = builder.WithObjects(tt.existing)
			}
			c := builder.Build()

			r := &PDBReconciler{
				client:             c,
				PDB:                tt.desired,
				componentName:      testMeta.Name,
				componentNamespace: testMeta.Namespace,
			}
			result, _, err := r.checkPDBExist(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestReconcile(t *testing.T) {
	s := scheme()

	pdbSpec := &policyv1.PodDisruptionBudgetSpec{
		MinAvailable: &minAvailable1,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.InferenceServicePodLabelKey: "sklearn-iris",
				constants.KServiceComponentLabel:      "predictor",
			},
		},
	}
	existingPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: testMeta.Name, Namespace: testMeta.Namespace},
		Spec:       *pdbSpec,
	}
	desiredPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: testMeta.Name, Namespace: testMeta.Namespace},
		Spec:       *pdbSpec,
	}

	tests := []struct {
		name           string
		desired        *policyv1.PodDisruptionBudget
		existing       *policyv1.PodDisruptionBudget
		expectedAction string
	}{
		{
			name:           "no-op when no desired PDB and none exists",
			desired:        nil,
			existing:       nil,
			expectedAction: "",
		},
		{
			name:           "creates PDB when not found",
			desired:        desiredPDB,
			existing:       nil,
			expectedAction: "create",
		},
		{
			name:           "no-op when PDB up to date",
			desired:        desiredPDB,
			existing:       existingPDB,
			expectedAction: "",
		},
		{
			name: "updates PDB when spec differs",
			desired: func() *policyv1.PodDisruptionBudget {
				updated := intstr.FromInt32(2)
				return &policyv1.PodDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{Name: testMeta.Name, Namespace: testMeta.Namespace},
					Spec: policyv1.PodDisruptionBudgetSpec{
						MinAvailable: &updated,
						Selector:     pdbSpec.Selector,
					},
				}
			}(),
			existing:       existingPDB,
			expectedAction: "update",
		},
		{
			name:           "deletes existing PDB when removed from spec",
			desired:        nil,
			existing:       existingPDB,
			expectedAction: "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(s)
			if tt.existing != nil {
				clientBuilder = clientBuilder.WithObjects(tt.existing)
			}
			fakeClient := clientBuilder.Build()

			var actualAction string
			tracker := &trackingClient{Client: fakeClient, actualAction: &actualAction}

			r := &PDBReconciler{
				client:             tracker,
				PDB:                tt.desired,
				componentName:      testMeta.Name,
				componentNamespace: testMeta.Namespace,
			}
			err := r.Reconcile(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAction, actualAction)

			// Verify actual cluster state after reconcile
			resultPDB := &policyv1.PodDisruptionBudget{}
			getErr := fakeClient.Get(context.Background(),
				types.NamespacedName{Name: testMeta.Name, Namespace: testMeta.Namespace}, resultPDB)

			switch tt.expectedAction {
			case "create", "update":
				require.NoError(t, getErr, "expected PDB to exist after %s", tt.expectedAction)
				assert.Equal(t, tt.desired.Spec.MinAvailable, resultPDB.Spec.MinAvailable)
				assert.Equal(t, tt.desired.Spec.MaxUnavailable, resultPDB.Spec.MaxUnavailable)
			case "delete":
				assert.True(t, apierr.IsNotFound(getErr), "expected PDB to be deleted")
			}
		})
	}
}

func TestSetControllerReferences(t *testing.T) {
	s := scheme()
	_ = v1beta1.AddToScheme(s)

	owner := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sklearn-iris",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	t.Run("nil PDB returns nil", func(t *testing.T) {
		r := &PDBReconciler{PDB: nil}
		err := r.SetControllerReferences(owner, s)
		assert.NoError(t, err)
	})

	t.Run("valid PDB sets owner reference", func(t *testing.T) {
		pdbObj := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sklearn-iris-predictor",
				Namespace: "default",
			},
		}
		r := &PDBReconciler{PDB: pdbObj}
		err := r.SetControllerReferences(owner, s)
		require.NoError(t, err)
		require.Len(t, pdbObj.OwnerReferences, 1)
		assert.Equal(t, "sklearn-iris", pdbObj.OwnerReferences[0].Name)
	})
}
