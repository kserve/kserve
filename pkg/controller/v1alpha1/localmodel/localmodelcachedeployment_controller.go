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

package localmodel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// LocalModelCacheDeploymentReconciler reconciles a LocalModelCacheDeployment object
type LocalModelCacheDeploymentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcachedeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcachedeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete

func (r *LocalModelCacheDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("localModelDeployment", req.Name)

	// Fetch the LocalModelCacheDeployment
	localModelDeployment := &v1alpha1.LocalModelCacheDeployment{}
	if err := r.Get(ctx, req.NamespacedName, localModelDeployment); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Calculate revision name from spec hash
	lmcName := fmt.Sprintf("%s-%s", localModelDeployment.Name, computeSpecHash(localModelDeployment.Spec))
	existingLmc := &v1alpha1.LocalModelCache{}
	err := r.Get(ctx, client.ObjectKey{Name: lmcName}, existingLmc)

	if errors.IsNotFound(err) {
		// Create new LocalModelCache
		lmc := &v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name: lmcName,
				Labels: map[string]string{
					constants.LocalModelCacheDeploymentLabel: localModelDeployment.Name,
					constants.LocalModelCacheRevisionLabel:   fmt.Sprintf("%d", localModelDeployment.Generation),
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(localModelDeployment, v1alpha1.SchemeGroupVersion.WithKind(constants.LocalModelCacheDeploymentKind)),
				},
			},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: localModelDeployment.Spec.SourceModelUri,
				ModelSize:      localModelDeployment.Spec.ModelSize,
				NodeGroups:     localModelDeployment.Spec.NodeGroups,
			},
		}

		if err := r.Create(ctx, lmc); err != nil {
			log.Error(err, "Failed to create LocalModelCache", "name", lmcName)
			return ctrl.Result{}, err
		}
		log.Info("Created LocalModelCache", "name", lmcName)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	localModelDeployment.Status.CurrentRevision = lmcName
	localModelDeployment.Status.ObservedGeneration = localModelDeployment.Generation

	// Update revision list
	if err := r.updateRevisionList(ctx, localModelDeployment); err != nil {
		log.Error(err, "Failed to update revision list")
		return ctrl.Result{}, err
	}

	// Clean up old revisions
	if err := r.cleanupOldRevisions(ctx, localModelDeployment); err != nil {
		log.Error(err, "Failed to cleanup old revisions")
		return ctrl.Result{}, err
	}

	if err := r.Status().Update(ctx, localModelDeployment); err != nil {
		log.Error(err, "Failed to update LocalModelCacheDeployment status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *LocalModelCacheDeploymentReconciler) updateRevisionList(ctx context.Context, localModelDeployment *v1alpha1.LocalModelCacheDeployment) error {
	// List all LocalModelCaches owned by this LocalModelCacheDeployment
	lmcList := &v1alpha1.LocalModelCacheList{}
	if err := r.List(ctx, lmcList, client.MatchingLabels{
		constants.LocalModelCacheDeploymentLabel: localModelDeployment.Name,
	}); err != nil {
		return err
	}

	revisions := []v1alpha1.LocalModelCacheDeploymentRevision{}
	for _, lmc := range lmcList.Items {
		var revNum int32
		if revLabel, ok := lmc.Labels[constants.LocalModelCacheRevisionLabel]; ok {
			if _, err := fmt.Sscanf(revLabel, "%d", &revNum); err != nil {
				r.Log.Error(err, "Failed to parse revision label", "name", lmc.Name, "label", revLabel)
				continue
			}
		}
		rev := v1alpha1.LocalModelCacheDeploymentRevision{
			Name:     lmc.Name,
			Revision: revNum,
		}
		revisions = append(revisions, rev)
	}
	localModelDeployment.Status.Revisions = revisions
	return nil
}

func (r *LocalModelCacheDeploymentReconciler) cleanupOldRevisions(ctx context.Context, deployment *v1alpha1.LocalModelCacheDeployment) error {
	limit := int32(10)
	if deployment.Spec.RevisionHistoryLimit != nil {
		limit = *deployment.Spec.RevisionHistoryLimit
	}

	lmcList := &v1alpha1.LocalModelCacheList{}
	if err := r.List(ctx, lmcList, client.MatchingLabels{
		constants.LocalModelCacheDeploymentLabel: deployment.Name,
	}); err != nil {
		return err
	}

	// +1 for the current revision
	if int32(len(lmcList.Items)) <= limit+1 {
		return nil
	}

	// Sort by creation timestamp (oldest first)
	items := lmcList.Items
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].CreationTimestamp.Before(&items[i].CreationTimestamp) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	// Delete oldest revisions, skip current
	toDelete := int32(len(items)) - (limit + 1)
	for i := int32(0); i < toDelete; i++ {
		if items[i].Name == deployment.Status.CurrentRevision {
			continue
		}
		r.Log.Info("Deleting old revision", "name", items[i].Name)
		if err := r.Delete(ctx, &items[i]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func computeSpecHash(spec v1alpha1.LocalModelCacheDeploymentSpec) string {
	// Exclude RevisionHistoryLimit from hash — changing it should not create a new revision
	hashSpec := struct {
		SourceModelUri string
		ModelSize      string
		NodeGroups     []string
	}{
		SourceModelUri: spec.SourceModelUri,
		ModelSize:      spec.ModelSize.String(),
		NodeGroups:     spec.NodeGroups,
	}
	data, _ := json.Marshal(hashSpec)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:4])
}

func (r *LocalModelCacheDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LocalModelCacheDeployment{}).
		Owns(&v1alpha1.LocalModelCache{}).
		Complete(r)
}
