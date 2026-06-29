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

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var log = logf.Log.WithName("PDBReconciler")

// PDBReconciler reconciles a PodDisruptionBudget for a KServe component.
// PDB is nil when no PodDisruptionBudget is desired; non-nil when one should exist.
// componentName and componentNamespace are always set so that a pre-existing PDB
// can be looked up and deleted even when PDB is nil.
type PDBReconciler struct {
	client             client.Client
	scheme             *runtime.Scheme
	PDB                *policyv1.PodDisruptionBudget
	componentName      string
	componentNamespace string
}

func NewPDBReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
) (*PDBReconciler, error) {
	return &PDBReconciler{
		client:             client,
		scheme:             scheme,
		PDB:                createPDB(componentMeta, componentExt),
		componentName:      componentMeta.Name,
		componentNamespace: componentMeta.Namespace,
	}, nil
}

func createPDB(componentMeta metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec) *policyv1.PodDisruptionBudget {
	if componentExt == nil || componentExt.PodDisruptionBudget == nil {
		return nil
	}

	spec := *componentExt.PodDisruptionBudget
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.InferenceServicePodLabelKey: componentMeta.Labels[constants.InferenceServicePodLabelKey],
			constants.KServiceComponentLabel:      componentMeta.Labels[constants.KServiceComponentLabel],
		},
	}

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentMeta.Name,
			Namespace: componentMeta.Namespace,
		},
		Spec: spec,
	}
}

func (r *PDBReconciler) checkPDBExist(ctx context.Context) (constants.CheckResultType, *policyv1.PodDisruptionBudget, error) {
	existing := &policyv1.PodDisruptionBudget{}
	err := r.client.Get(ctx, types.NamespacedName{
		Namespace: r.componentNamespace,
		Name:      r.componentName,
	}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			if r.PDB == nil {
				return constants.CheckResultSkipped, nil, nil
			}
			return constants.CheckResultCreate, nil, nil
		}
		return constants.CheckResultUnknown, nil, err
	}
	if r.PDB == nil {
		return constants.CheckResultDelete, existing, nil
	}
	if equality.Semantic.DeepEqual(r.PDB.Spec, existing.Spec) {
		return constants.CheckResultExisted, existing, nil
	}
	return constants.CheckResultUpdate, existing, nil
}

// Reconcile creates, updates, or deletes the PodDisruptionBudget to match the desired state.
func (r *PDBReconciler) Reconcile(ctx context.Context) error {
	checkResult, existing, err := r.checkPDBExist(ctx)
	log.Info("PodDisruptionBudget reconcile", "checkResult", checkResult, "err", err)
	if err != nil {
		return err
	}

	switch checkResult {
	case constants.CheckResultCreate:
		return r.client.Create(ctx, r.PDB)
	case constants.CheckResultUpdate:
		existing.Spec = r.PDB.Spec
		return r.client.Update(ctx, existing)
	case constants.CheckResultDelete:
		return r.client.Delete(ctx, existing)
	default:
		return nil
	}
}

func (r *PDBReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	if r.PDB == nil {
		return nil
	}
	return controllerutil.SetControllerReference(owner, r.PDB, scheme)
}
