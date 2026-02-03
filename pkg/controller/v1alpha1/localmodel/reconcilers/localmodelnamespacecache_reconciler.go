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

// LocalModelNamespaceCacheReconciler reconciles namespace-scoped LocalModelNamespaceCache resources
type LocalModelNamespaceCacheReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// Reconcile
// Step 1 - Checks if the CR is in the deletion process. Deletion completes when all LocalModelNodes have been updated
// Step 2 - Adds this model to LocalModelNode resources in the node group
// Step 3 - Creates PV & PVC for model download (in the same namespace as the CR)
// Step 4 - Creates PV & PVCs for ISVCs in the same namespace using this cached model
func (c *LocalModelNamespaceCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.Log.Info("Reconciling namespace-scoped localmodel", "name", req.Name, "namespace", req.Namespace)
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

	localModel := &v1alpha1.LocalModelNamespaceCache{}
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
		if !utils.Includes(localModel.ObjectMeta.Finalizers, NamespaceCacheFinalizerName) {
			patch := client.MergeFrom(localModel.DeepCopy())
			localModel.ObjectMeta.Finalizers = append(localModel.ObjectMeta.Finalizers, NamespaceCacheFinalizerName)
			if err := c.Patch(ctx, localModel, patch); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		return DeleteModelFromNodes(ctx, c.Client, c.Clientset, c.Log, nil, localModel, nodeGroups)
	}

	// Step 2 - Adds this model to LocalModelNode resources in the node group
	if err := ReconcileLocalModelNode(ctx, c.Client, c.Log, nil, localModel, nodeGroups); err != nil {
		c.Log.Error(err, "failed to reconcile LocalModelNode for namespace cache")
	}

	// Step 3 - Creates PV & PVC for model download (in the CR's namespace)
	// Note: The download PVC name includes "-download" suffix to avoid conflict with serving PVCs
	// since for namespace-scoped caches, both download and serving happen in the same namespace
	for _, nodeGroup := range nodeGroups {
		pvSpec := nodeGroup.Spec.PersistentVolumeSpec
		pv := corev1.PersistentVolume{Spec: pvSpec, ObjectMeta: metav1.ObjectMeta{
			Name: localModel.Name + "-" + nodeGroup.Name + "-" + localModel.Namespace + "-download",
		}}
		if err := CreatePV(ctx, c.Clientset, c.Scheme, c.Log, pv, nil, localModel); err != nil {
			c.Log.Error(err, "Create PV err", "name", pv.Name)
		}

		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: localModel.Name + "-" + nodeGroup.Name + "-download",
			},
			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
		}
		pvc.Spec.VolumeName = pv.Name

		// Download jobs run in the same namespace as the LocalModelNamespaceCache
		if err := CreatePVC(ctx, c.Clientset, c.Scheme, c.Log, pvc, localModel.Namespace, nil, localModel); err != nil {
			c.Log.Error(err, "Create PVC err", "name", pvc.Name)
		}
	}

	if localModelConfig.DisableVolumeManagement {
		return ctrl.Result{}, nil
	}

	// Step 4 - Creates PV & PVCs for ISVCs in the same namespace using this model
	err = ReconcileForIsvcs(ctx, c.Client, c.Clientset, c.Scheme, c.Log, nil, localModel, nodeGroups, defaultNodeGroup)
	return ctrl.Result{}, err
}

// Reconciles corresponding namespace model cache CR when we found an update on an isvc
func (c *LocalModelNamespaceCacheReconciler) isvcFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	isvc := obj.(*v1beta1.InferenceService)
	if isvc.Labels == nil {
		return []reconcile.Request{}
	}
	var modelName string
	var modelNamespace string
	var ok bool
	if modelName, ok = isvc.Labels[constants.LocalModelLabel]; !ok {
		return []reconcile.Request{}
	}
	// Check if it's a namespace-scoped model
	if modelNamespace, ok = isvc.Labels[constants.LocalModelNamespaceLabel]; !ok {
		return []reconcile.Request{}
	}
	// Ensure the ISVC is in the same namespace as the LocalModelNamespaceCache
	if isvc.Namespace != modelNamespace {
		return []reconcile.Request{}
	}

	localModel := &v1alpha1.LocalModelNamespaceCache{}
	if err := c.Get(ctx, types.NamespacedName{Name: modelName, Namespace: modelNamespace}, localModel); err != nil {
		c.Log.Error(err, "error getting namespace localModel", "name", modelName, "namespace", modelNamespace)
		return []reconcile.Request{}
	}

	c.Log.Info("Reconcile namespace localModel from inference services", "name", modelName, "namespace", modelNamespace)

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      modelName,
			Namespace: modelNamespace,
		},
	}}
}

// Given a node object, checks if it matches any node group CR, then reconcile all namespace local models that has this node group.
func (c *LocalModelNamespaceCacheReconciler) nodeFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	node := obj.(*corev1.Node)
	requests := []reconcile.Request{}
	models := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := c.Client.List(ctx, models); err != nil {
		c.Log.Error(err, "list namespace models error when reconciling nodes")
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
			c.Log.Info("new node for namespace model", "name", model.Name, "namespace", model.Namespace)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      model.Name,
					Namespace: model.Namespace,
				},
			})
		}
	}
	return requests
}

// Given a LocalModelNode object, reconcile all namespace-scoped LocalModelNamespaceCache CRs that are referenced in it.
func (c *LocalModelNamespaceCacheReconciler) localmodelNodeFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	localmodelNode := obj.(*v1alpha1.LocalModelNode)
	requests := []reconcile.Request{}
	for _, modelInfo := range localmodelNode.Spec.LocalModels {
		// Only handle namespace-scoped LocalModelNamespaceCache (non-empty namespace)
		if modelInfo.Namespace != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      modelInfo.ModelName,
					Namespace: modelInfo.Namespace,
				},
			})
		}
	}
	return requests
}

func (c *LocalModelNamespaceCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	// Index for namespace-scoped models - index by name AND namespace label
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.InferenceService{}, LocalModelNamespaceKey, func(rawObj client.Object) []string {
		isvc := rawObj.(*v1beta1.InferenceService)
		// Only index if both labels exist and namespace matches
		modelName, hasModel := isvc.GetLabels()[constants.LocalModelLabel]
		modelNamespace, hasNamespace := isvc.GetLabels()[constants.LocalModelNamespaceLabel]
		if hasModel && hasNamespace && isvc.Namespace == modelNamespace {
			return []string{modelName}
		}
		return nil
	}); err != nil {
		return err
	}

	isvcPredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Check if namespace label changed
			oldNsLabel := e.ObjectOld.GetLabels()[constants.LocalModelNamespaceLabel]
			newNsLabel := e.ObjectNew.GetLabels()[constants.LocalModelNamespaceLabel]
			return oldNsLabel != newNsLabel
		},
		CreateFunc: func(e event.CreateEvent) bool {
			if _, ok := e.Object.GetLabels()[constants.LocalModelNamespaceLabel]; !ok {
				return false
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if _, ok := e.Object.GetLabels()[constants.LocalModelNamespaceLabel]; !ok {
				return false
			}
			return true
		},
	}

	nodePredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectNew.(*corev1.Node)
			newNode := e.ObjectNew.(*corev1.Node)
			return !IsNodeReady(*oldNode) && IsNodeReady(*newNode)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	localModelNodePredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*v1alpha1.LocalModelNode)
			newNode := e.ObjectNew.(*v1alpha1.LocalModelNode)
			return !reflect.DeepEqual(oldNode.Status, newNode.Status)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LocalModelNamespaceCache{}).
		Owns(&corev1.PersistentVolume{}).
		Owns(&corev1.PersistentVolumeClaim{})

	if !localModelConfig.DisableVolumeManagement {
		controllerBuilder.Watches(&v1beta1.InferenceService{}, handler.EnqueueRequestsFromMapFunc(c.isvcFuncNamespaceCache), builder.WithPredicates(isvcPredicates))
	}

	return controllerBuilder.
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(c.nodeFuncNamespaceCache), builder.WithPredicates(nodePredicates)).
		Watches(&v1alpha1.LocalModelNode{}, handler.EnqueueRequestsFromMapFunc(c.localmodelNodeFuncNamespaceCache), builder.WithPredicates(localModelNodePredicates)).
		Complete(c)
}
