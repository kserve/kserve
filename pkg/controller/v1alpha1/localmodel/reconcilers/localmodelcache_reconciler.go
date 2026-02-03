/*
Copyright 2024 The KServe Authors.

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

package reconcilers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	controllerutils "github.com/kserve/kserve/pkg/controller/v1alpha1/utils"
	"github.com/kserve/kserve/pkg/utils"
)

// LocalModelReconciler reconciles cluster-scoped LocalModelCache resources
type LocalModelReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// Reconcile
// Step 1 - Checks if the CR is in the deletion process. Deletion completes when all LocalModelNodes have been updated
// Step 2 - Adds this model to LocalModelNode resources in the node group
// Step 3 - Creates PV & PVC for model download
// Step 4 - Creates PV & PVCs for namespaces with isvcs using this cached model
func (c *LocalModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.Log.Info("Reconciling localmodel", "name", req.Name)
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, c.Clientset)
	if err != nil {
		c.Log.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return reconcile.Result{}, err
	}
	localModelConfig, err := v1beta1.NewLocalModelConfig(isvcConfigMap)
	if err != nil {
		c.Log.Error(err, "Failed to get local model config")
		return reconcile.Result{}, err
	}

	localModel := &v1alpha1.LocalModelCache{}
	if err := c.Get(ctx, req.NamespacedName, localModel); err != nil {
		// Ignore not-found errors, we can get them on deleted requests.
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Get all node groups of the local model
	defaultNodeGroup := &v1alpha1.LocalModelNodeGroup{}
	nodeGroups := map[string]*v1alpha1.LocalModelNodeGroup{}
	for idx, nodeGroupName := range localModel.Spec.NodeGroups {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		nodeGroupNamespacedName := types.NamespacedName{Name: nodeGroupName}
		if err := c.Get(ctx, nodeGroupNamespacedName, nodeGroup); err != nil {
			return reconcile.Result{}, err
		}
		nodeGroups[nodeGroupName] = nodeGroup
		if idx == 0 {
			defaultNodeGroup = nodeGroup
		}
	}

	// Step 1 - Checks if the CR is in the deletion process
	if localModel.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !utils.Includes(localModel.ObjectMeta.Finalizers, FinalizerName) {
			patch := client.MergeFrom(localModel.DeepCopy())
			localModel.ObjectMeta.Finalizers = append(localModel.ObjectMeta.Finalizers, FinalizerName)
			if err := c.Patch(ctx, localModel, patch); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		return DeleteModelFromNodes(ctx, c.Client, c.Clientset, c.Log, localModel, nil, nodeGroups)
	}

	// Step 2 - Adds this model to LocalModelNode resources in the node group
	if err := ReconcileLocalModelNode(ctx, c.Client, c.Log, localModel, nil, nodeGroups); err != nil {
		c.Log.Error(err, "failed to reconcile LocalModelNode")
	}

	// Step 3 - Creates PV & PVC for model download
	for _, nodeGroup := range nodeGroups {
		pvSpec := nodeGroup.Spec.PersistentVolumeSpec
		pv := corev1.PersistentVolume{Spec: pvSpec, ObjectMeta: metav1.ObjectMeta{
			Name: localModel.Name + "-" + nodeGroup.Name + "-download",
		}}
		if err := CreatePV(ctx, c.Clientset, c.Scheme, c.Log, pv, localModel, nil); err != nil {
			c.Log.Error(err, "Create PV err", "name", pv.Name)
		}

		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: localModel.Name + "-" + nodeGroup.Name,
			},
			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
		}
		pvc.Spec.VolumeName = pv.Name

		if err := CreatePVC(ctx, c.Clientset, c.Scheme, c.Log, pvc, localModelConfig.JobNamespace, localModel, nil); err != nil {
			c.Log.Error(err, "Create PVC err", "name", pv.Name)
		}
	}

	if localModelConfig.DisableVolumeManagement {
		return ctrl.Result{}, nil
	}

	// Step 4 - Creates PV & PVCs for namespaces with isvcs using this model
	err = ReconcileForIsvcs(ctx, c.Client, c.Clientset, c.Scheme, c.Log, localModel, nil, nodeGroups, defaultNodeGroup)
	return ctrl.Result{}, err
}

// Reconciles corresponding model cache CR when we found an update on an isvc
func (c *LocalModelReconciler) isvcFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	isvc := obj.(*v1beta1.InferenceService)
	if isvc.Labels == nil {
		return []reconcile.Request{}
	}
	var modelName string
	var ok bool
	if modelName, ok = isvc.Labels[constants.LocalModelLabel]; !ok {
		return []reconcile.Request{}
	}
	localModel := &v1alpha1.LocalModelCache{}
	if err := c.Get(ctx, types.NamespacedName{Name: modelName}, localModel); err != nil {
		c.Log.Error(err, "error getting localModel", "name", modelName)
		return []reconcile.Request{}
	}

	c.Log.Info("Reconcile localModel from inference services", "name", modelName)

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name: modelName,
		},
	}}
}

// Given a node object, checks if it matches any node group CR, then reconcile all local models that has this node group to create download jobs.
func (c *LocalModelReconciler) nodeFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	node := obj.(*corev1.Node)
	requests := []reconcile.Request{}
	models := &v1alpha1.LocalModelCacheList{}
	if err := c.Client.List(ctx, models); err != nil {
		c.Log.Error(err, "list models error when reconciling nodes")
		return []reconcile.Request{}
	}

	for _, model := range models.Items {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		nodeGroupNamespacedName := types.NamespacedName{Name: model.Spec.NodeGroups[0]}
		if err := c.Get(ctx, nodeGroupNamespacedName, nodeGroup); err != nil {
			c.Log.Info("get nodegroup failed", "name", model.Spec.NodeGroups[0])
			continue
		}
		matches, err := controllerutils.CheckNodeAffinity(&nodeGroup.Spec.PersistentVolumeSpec, *node)
		if err != nil {
			c.Log.Error(err, "checkNodeAffinity error", "node", node.Name)
		}
		if matches {
			c.Log.Info("new node for model", "name", model.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: model.Name,
				},
			})
		}
	}
	return requests
}

// Given a LocalModelNode object, reconcile all cluster-scoped LocalModelCache CRs that are referenced in it.
func (c *LocalModelReconciler) localmodelNodeFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	localmodelNode := obj.(*v1alpha1.LocalModelNode)
	requests := []reconcile.Request{}
	for _, modelInfo := range localmodelNode.Spec.LocalModels {
		// Only handle cluster-scoped LocalModelCache (empty namespace)
		if modelInfo.Namespace == "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: modelInfo.ModelName,
				},
			})
		}
	}
	return requests
}

func (c *LocalModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(context.Background(), c.Clientset)
	if err != nil {
		c.Log.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return err
	}
	localModelConfig, err := v1beta1.NewLocalModelConfig(isvcConfigMap)
	if err != nil {
		c.Log.Error(err, "Failed to get local model config during controller manager setup")
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.PersistentVolumeClaim{}, OwnerKey, func(rawObj client.Object) []string {
		pvc := rawObj.(*corev1.PersistentVolumeClaim)
		owner := metav1.GetControllerOf(pvc)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGVStr || owner.Kind != ModelCacheCRName {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.InferenceService{}, LocalModelKey, func(rawObj client.Object) []string {
		isvc := rawObj.(*v1beta1.InferenceService)
		if model, ok := isvc.GetLabels()[constants.LocalModelLabel]; ok {
			return []string{model}
		}
		return nil
	}); err != nil {
		return err
	}

	isvcPredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetLabels()[constants.LocalModelLabel] != e.ObjectNew.GetLabels()[constants.LocalModelLabel]
		},
		CreateFunc: func(e event.CreateEvent) bool {
			if _, ok := e.Object.GetLabels()[constants.LocalModelLabel]; !ok {
				return false
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if _, ok := e.Object.GetLabels()[constants.LocalModelLabel]; !ok {
				return false
			}
			return true
		},
	}

	nodePredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Only reconciles the local model crs when the node becomes ready from not ready
			// Todo: add tests
			oldNode := e.ObjectNew.(*corev1.Node)
			newNode := e.ObjectNew.(*corev1.Node)
			return !IsNodeReady(*oldNode) && IsNodeReady(*newNode)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// Do nothing here, generates local model cr reconcile requests in nodeFunc
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	// Define predicates to filter events based on changes to the status field
	localModelNodePredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*v1alpha1.LocalModelNode)
			newNode := e.ObjectNew.(*v1alpha1.LocalModelNode)
			return !reflect.DeepEqual(oldNode.Status, newNode.Status)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// Do nothing on create
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do nothing on delete
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Do nothing on generic events
			return false
		},
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LocalModelCache{}).
		// Ownes PersistentVolumes and PersistentVolumeClaims that is created by this local model controller
		Owns(&corev1.PersistentVolume{}).
		Owns(&corev1.PersistentVolumeClaim{})

	if !localModelConfig.DisableVolumeManagement {
		controllerBuilder.Watches(&v1beta1.InferenceService{}, handler.EnqueueRequestsFromMapFunc(c.isvcFunc), builder.WithPredicates(isvcPredicates))
	}

	return controllerBuilder.
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(c.nodeFunc), builder.WithPredicates(nodePredicates)).
		// Updates model status when localmodelnode status changes
		Watches(&v1alpha1.LocalModelNode{}, handler.EnqueueRequestsFromMapFunc(c.localmodelNodeFunc), builder.WithPredicates(localModelNodePredicates)).
		Complete(c)
}
