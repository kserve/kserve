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
	batchv1 "k8s.io/api/batch/v1"
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
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	controllerutils "github.com/kserve/kserve/pkg/controller/v1alpha1/utils"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/localmodelcache"
	"github.com/kserve/kserve/pkg/utils"
)

// LocalModelNamespaceCacheReconciler reconciles namespace-scoped LocalModelNamespaceCache resources
type LocalModelNamespaceCacheReconciler struct {
	client.Client
	APIReader                client.Reader
	Clientset                *kubernetes.Clientset
	Log                      logr.Logger
	Scheme                   *runtime.Scheme
	CredentialBuilder        *credentials.CredentialBuilder
	llmInferenceServiceCRDUp bool
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

	// Shared-PVC mode takes a dedicated branch before any node-group resolution or per-node
	// PV/PVC/LocalModelNode fan-out.
	if localModel.Spec.SharedPVCMode() {
		return c.reconcileSharedPVC(ctx, localModel, isvcConfigMap)
	}

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
	if localModel.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !utils.Includes(localModel.Finalizers, NamespaceCacheFinalizerName) {
			patch := client.MergeFrom(localModel.DeepCopy())
			localModel.Finalizers = append(localModel.Finalizers, NamespaceCacheFinalizerName)
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

	// Step 3 - Creates PV & PVC for model download (in jobNamespace, same as cluster-scoped)
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
				Name: localModel.Name + "-" + nodeGroup.Name + "-" + localModel.Namespace + "-download",
			},
			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
		}
		pvc.Spec.VolumeName = pv.Name

		if err := CreatePVC(ctx, c.Clientset, c.Scheme, c.Log, pvc, localModelConfig.JobNamespace, nil, localModel); err != nil {
			c.Log.Error(err, "Create PVC err", "name", pvc.Name)
		}
	}

	if localModelConfig.DisableVolumeManagement {
		return ctrl.Result{}, nil
	}

	// Step 4 - Creates PV & PVCs for ISVCs in the same namespace using this model
	err = ReconcileForIsvcs(ctx, c.Client, c.Clientset, c.Scheme, c.Log, nil, localModel, nodeGroups, defaultNodeGroup, c.llmInferenceServiceCRDUp)
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

// Reconciles corresponding namespace model cache CR when we found an update on an LLMInferenceService
func (c *LocalModelNamespaceCacheReconciler) llmIsvcFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	llmSvc := obj.(*v1alpha2.LLMInferenceService)
	cacheNames := localmodelcache.LLMISVCNamespaceCacheNames(llmSvc.Namespace, llmSvc.Labels, llmSvc.Annotations)
	if len(cacheNames) == 0 {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(cacheNames))
	for _, modelName := range cacheNames {
		localModel := &v1alpha1.LocalModelNamespaceCache{}
		if err := c.Get(ctx, types.NamespacedName{Name: modelName, Namespace: llmSvc.Namespace}, localModel); err != nil {
			c.Log.Error(err, "error getting namespace localModel", "name", modelName, "namespace", llmSvc.Namespace)
			continue
		}
		c.Log.Info("Reconcile namespace localModel from LLM inference services", "name", modelName, "namespace", llmSvc.Namespace)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: modelName, Namespace: llmSvc.Namespace},
		})
	}
	return requests
}

// Given a node object, checks if it matches any node group CR, then reconcile all namespace local models that has this node group.
func (c *LocalModelNamespaceCacheReconciler) nodeFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	node := obj.(*corev1.Node)
	requests := []reconcile.Request{}
	models := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := c.List(ctx, models); err != nil {
		c.Log.Error(err, "list namespace models error when reconciling nodes")
		return []reconcile.Request{}
	}

	for _, model := range models.Items {
		// Shared-PVC caches have no node groups and ignore node events.
		if model.Spec.SharedPVCMode() || len(model.Spec.NodeGroups) == 0 {
			continue
		}
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

// Given a PVC, reconcile shared-PVC caches in the same namespace that reference it by name.
// PVCs are watched via a map (never owned), so their creation, binding, expansion, and capacity
// changes trigger reconciliation without the controller taking ownership of user-provided claims.
func (c *LocalModelNamespaceCacheReconciler) pvcFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	pvc := obj.(*corev1.PersistentVolumeClaim)
	models := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := c.List(ctx, models, client.InNamespace(pvc.Namespace)); err != nil {
		c.Log.Error(err, "list namespace models error when reconciling PVCs")
		return []reconcile.Request{}
	}
	requests := []reconcile.Request{}
	for i := range models.Items {
		model := &models.Items[i]
		if model.Spec.SharedPVCMode() && *model.Spec.PVCRef == pvc.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: model.Name, Namespace: model.Namespace},
			})
		}
	}
	return requests
}

// Given a deleted shared-PVC cache, reconcile caches that were contending for the same
// destination so the next deterministic owner can start its import.
func (c *LocalModelNamespaceCacheReconciler) deletedCacheFuncNamespaceCache(ctx context.Context, obj client.Object) []reconcile.Request {
	deleted := obj.(*v1alpha1.LocalModelNamespaceCache)
	if !deleted.Spec.SharedPVCMode() {
		return nil
	}

	reader := c.APIReader
	if reader == nil {
		reader = c.Client
	}
	caches := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := reader.List(ctx, caches, client.InNamespace(deleted.Namespace)); err != nil {
		c.Log.Error(err, "list namespace models error when reconciling deleted cache")
		return nil
	}

	storageKey := v1alpha1.GetStorageKey(deleted.Spec.SourceModelUri)
	requests := []reconcile.Request{}
	for i := range caches.Items {
		cache := &caches.Items[i]
		if !cache.Spec.SharedPVCMode() || *cache.Spec.PVCRef != *deleted.Spec.PVCRef {
			continue
		}
		if v1alpha1.GetStorageKey(cache.Spec.SourceModelUri) != storageKey {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
		})
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
	if c.APIReader == nil {
		c.APIReader = mgr.GetAPIReader()
	}
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
		modelName, hasModel := isvc.GetLabels()[constants.LocalModelLabel]
		modelNamespace, hasNamespace := isvc.GetLabels()[constants.LocalModelNamespaceLabel]
		if hasModel && hasNamespace && isvc.Namespace == modelNamespace {
			return []string{modelName}
		}
		return nil
	}); err != nil {
		return err
	}

	hasLLMISvcCRD, err := hasLLMInferenceServiceCRD(mgr)
	if err != nil {
		return err
	}
	c.llmInferenceServiceCRDUp = hasLLMISvcCRD
	if hasLLMISvcCRD {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha2.LLMInferenceService{}, LocalModelNamespaceKey, func(rawObj client.Object) []string {
			llmSvc := rawObj.(*v1alpha2.LLMInferenceService)
			return localmodelcache.LLMISVCNamespaceCacheNames(llmSvc.Namespace, llmSvc.Labels, llmSvc.Annotations)
		}); err != nil {
			return err
		}
	} else {
		c.Log.Info("LLMInferenceService CRD not installed; skipping LocalModelNamespaceCache LLMInferenceService index and watch setup")
	}

	isvcPredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
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

	nodePredicates := NodeReadyPredicate()

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

	cacheDeletePredicates := predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LocalModelNamespaceCache{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		// Shared-PVC import Jobs are owned by the cache, so their status changes trigger reconciliation.
		Owns(&batchv1.Job{})

	llmIsvcPredicates := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*v1alpha2.LLMInferenceService)
			new := e.ObjectNew.(*v1alpha2.LLMInferenceService)
			return !localmodelcache.CacheNamesEqual(
				localmodelcache.LLMISVCNamespaceCacheNames(old.Namespace, old.Labels, old.Annotations),
				localmodelcache.LLMISVCNamespaceCacheNames(new.Namespace, new.Labels, new.Annotations),
			)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			llmSvc := e.Object.(*v1alpha2.LLMInferenceService)
			return len(localmodelcache.LLMISVCNamespaceCacheNames(llmSvc.Namespace, llmSvc.Labels, llmSvc.Annotations)) > 0
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			llmSvc := e.Object.(*v1alpha2.LLMInferenceService)
			return len(localmodelcache.LLMISVCNamespaceCacheNames(llmSvc.Namespace, llmSvc.Labels, llmSvc.Annotations)) > 0
		},
	}

	if !localModelConfig.DisableVolumeManagement {
		controllerBuilder.Watches(&v1beta1.InferenceService{}, handler.EnqueueRequestsFromMapFunc(c.isvcFuncNamespaceCache), builder.WithPredicates(isvcPredicates))
		if hasLLMISvcCRD {
			controllerBuilder.Watches(&v1alpha2.LLMInferenceService{}, handler.EnqueueRequestsFromMapFunc(c.llmIsvcFuncNamespaceCache), builder.WithPredicates(llmIsvcPredicates))
		}
	}

	return controllerBuilder.
		Watches(&v1alpha1.LocalModelNamespaceCache{}, handler.EnqueueRequestsFromMapFunc(c.deletedCacheFuncNamespaceCache), builder.WithPredicates(cacheDeletePredicates)).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(c.nodeFuncNamespaceCache), builder.WithPredicates(nodePredicates)).
		Watches(&v1alpha1.LocalModelNode{}, handler.EnqueueRequestsFromMapFunc(c.localmodelNodeFuncNamespaceCache), builder.WithPredicates(localModelNodePredicates)).
		Watches(&corev1.PersistentVolumeClaim{}, handler.EnqueueRequestsFromMapFunc(c.pvcFuncNamespaceCache)).
		Complete(c)
}
