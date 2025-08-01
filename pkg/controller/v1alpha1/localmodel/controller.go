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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
package localmodel

import (
	"context"
	"maps"
	"reflect"
	"slices"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

type LocalModelReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

var (
	ownerKey         = ".metadata.controller"
	localModelKey    = ".localmodel"
	apiGVStr         = v1alpha1.SchemeGroupVersion.String()
	modelCacheCRName = "LocalModelCache"
	finalizerName    = "localmodel.kserve.io/finalizer"
)

// The localmodel is being deleted
func (c *LocalModelReconciler) deleteModelFromNodes(ctx context.Context, localModel *v1alpha1.LocalModelCache,
	nodeGroups map[string]*v1alpha1.LocalModelNodeGroup,
) (ctrl.Result, error) {
	// finalizer does not exist, nothing to do here!
	if !utils.Includes(localModel.ObjectMeta.Finalizers, finalizerName) {
		return ctrl.Result{}, nil
	}
	c.Log.Info("deleting model", "name", localModel.Name)

	// Todo: Prevent deletion if there are isvcs using this localmodel
	for _, nodeGroup := range nodeGroups {
		readyNodes, notReadyNodes, err := getNodesFromNodeGroup(ctx, nodeGroup, c.Client)
		if err != nil {
			c.Log.Error(err, "getNodesFromNodeGroup node error")
			return ctrl.Result{}, err
		}
		for _, node := range append(readyNodes.Items, notReadyNodes.Items...) {
			localModelNode := &v1alpha1.LocalModelNode{}
			err := c.Client.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
			if err != nil {
				if apierr.IsNotFound(err) {
					c.Log.Info("localmodelNode not found", "node", node.Name)
					continue
				} else {
					c.Log.Error(err, "Failed to get localmodelnode", "name", node.Name)
					return ctrl.Result{}, err
				}
			}

			if err := c.DeleteModelFromNode(ctx, localModelNode, localModel); err != nil {
				c.Log.Error(err, "failed to delete model from localModelNode", "localModelNode", localModelNode.Name)
				return ctrl.Result{}, err
			}
		}
	}

	patch := client.MergeFrom(localModel.DeepCopy())
	localModel.ObjectMeta.Finalizers = utils.RemoveString(localModel.ObjectMeta.Finalizers, finalizerName)
	if err := c.Patch(ctx, localModel, patch); err != nil {
		c.Log.Error(err, "Cannot remove finalizer", "model name", localModel.Name)
		return ctrl.Result{}, err
	}

	// Stop reconciliation as the item is being deleted
	return ctrl.Result{}, nil
}

// Creates a PV and set the localModel as its controller
func (c *LocalModelReconciler) createPV(ctx context.Context, spec corev1.PersistentVolume, localModel *v1alpha1.LocalModelCache) error {
	persistentVolumes := c.Clientset.CoreV1().PersistentVolumes()
	if _, err := persistentVolumes.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			c.Log.Error(err, "Failed to get PV")
			return err
		}
		c.Log.Info("Create PV", "name", spec.Name)
		if err := controllerutil.SetControllerReference(localModel, &spec, c.Scheme); err != nil {
			c.Log.Error(err, "Failed to set controller reference")
			return err
		}
		if _, err := persistentVolumes.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			c.Log.Error(err, "Failed to create PV", "name", spec.Name)
			return err
		}
	}
	return nil
}

// Creates a PVC and sets the localModel as its controller
func (c *LocalModelReconciler) createPVC(ctx context.Context, spec corev1.PersistentVolumeClaim, namespace string, localModel *v1alpha1.LocalModelCache) error {
	persistentVolumeClaims := c.Clientset.CoreV1().PersistentVolumeClaims(namespace)
	if _, err := persistentVolumeClaims.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			c.Log.Error(err, "Failed to get PVC")
			return err
		}
		c.Log.Info("Create PVC", "name", spec.Name, "namespace", namespace)
		if err := controllerutil.SetControllerReference(localModel, &spec, c.Scheme); err != nil {
			c.Log.Error(err, "Set controller reference")
			return err
		}
		if _, err := persistentVolumeClaims.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			c.Log.Error(err, "Failed to create PVC", "name", spec.Name)
			return err
		}
	}
	return nil
}

// ReconcileForIsvcs Get all isvcs with model cache enabled, create pvs and pvcs.
func (c *LocalModelReconciler) ReconcileForIsvcs(ctx context.Context, localModel *v1alpha1.LocalModelCache,
	localModelNodeGroups map[string]*v1alpha1.LocalModelNodeGroup, defaultNodeGroup *v1alpha1.LocalModelNodeGroup, jobNamespace string,
) error {
	isvcs := &v1beta1.InferenceServiceList{}
	if err := c.Client.List(ctx, isvcs, client.MatchingFields{localModelKey: localModel.Name}); err != nil {
		c.Log.Error(err, "List isvc error")
		return err
	}
	isvcNames := []v1alpha1.NamespacedName{}
	// namespaces with isvcs deployed and their node groups
	namespaceToNodeGroups := make(map[string]map[string]*v1alpha1.LocalModelNodeGroup)
	for _, isvc := range isvcs.Items {
		isvcNames = append(isvcNames, v1alpha1.NamespacedName{Name: isvc.Name, Namespace: isvc.Namespace})
		// isvc has nodegroup annotation
		if isvcNodeGroup, ok := isvc.ObjectMeta.Annotations[constants.NodeGroupAnnotationKey]; ok {
			if nodeGroup, ok := localModelNodeGroups[isvcNodeGroup]; ok {
				if _, ok := namespaceToNodeGroups[isvc.Namespace]; !ok {
					namespaceToNodeGroups[isvc.Namespace] = map[string]*v1alpha1.LocalModelNodeGroup{}
				}
				namespaceToNodeGroups[isvc.Namespace][nodeGroup.Name] = nodeGroup
			} else {
				c.Log.Info("Didn't find isvc node group in model cache node groups", "isvc name", isvc.Name, "isvc node group", isvcNodeGroup, "model cache node groups", slices.Collect(maps.Keys(localModelNodeGroups)))
			}
			// isvc does not have nodegroup annotation. Use default nodegroup
		} else if _, ok := namespaceToNodeGroups[isvc.Namespace]; !ok {
			c.Log.Info("Isvc does not have node group annotation", "isvc name", isvc.Name, "nodegroup annotation", constants.NodeGroupAnnotationKey)
			namespaceToNodeGroups[isvc.Namespace] = map[string]*v1alpha1.LocalModelNodeGroup{defaultNodeGroup.Name: defaultNodeGroup}
		} else {
			namespaceToNodeGroups[isvc.Namespace][defaultNodeGroup.Name] = defaultNodeGroup
		}
	}
	localModel.Status.InferenceServices = isvcNames
	if err := c.Status().Update(ctx, localModel); err != nil {
		c.Log.Error(err, "cannot update status", "name", localModel.Name)
	}

	for namespace, nodeGroups := range namespaceToNodeGroups {
		for nodeGroupName, nodeGroup := range nodeGroups {
			pvcName := localModel.Name + "-" + nodeGroupName
			pv := corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: pvcName + "-" + namespace,
				},
				Spec: nodeGroup.Spec.PersistentVolumeSpec,
			}
			if err := c.createPV(ctx, pv, localModel); err != nil {
				c.Log.Error(err, "Create PV err", "name", pv.Name)
			}

			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
			}
			pvc.Spec.VolumeName = pv.Name
			if err := c.createPVC(ctx, pvc, namespace, localModel); err != nil {
				c.Log.Error(err, "Create PVC err", "name", pvc.Name)
			}
		}
	}
	return nil
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
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !utils.Includes(localModel.ObjectMeta.Finalizers, finalizerName) {
			patch := client.MergeFrom(localModel.DeepCopy())
			localModel.ObjectMeta.Finalizers = append(localModel.ObjectMeta.Finalizers, finalizerName)
			if err := c.Patch(ctx, localModel, patch); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		return c.deleteModelFromNodes(ctx, localModel, nodeGroups)
	}

	// Step 2 - Adds this model to LocalModelNode resources in the node group
	if err := c.ReconcileLocalModelNode(ctx, localModel, nodeGroups); err != nil {
		c.Log.Error(err, "failed to reconcile LocalModelNode")
	}

	// Step 3 - Creates PV & PVC for model download
	for _, nodeGroup := range nodeGroups {
		pvSpec := nodeGroup.Spec.PersistentVolumeSpec
		pv := corev1.PersistentVolume{Spec: pvSpec, ObjectMeta: metav1.ObjectMeta{
			Name: localModel.Name + "-" + nodeGroup.Name + "-download",
		}}
		if err := c.createPV(ctx, pv, localModel); err != nil {
			c.Log.Error(err, "Create PV err", "name", pv.Name)
		}

		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: localModel.Name + "-" + nodeGroup.Name,
			},
			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
		}
		pvc.Spec.VolumeName = pv.Name

		if err := c.createPVC(ctx, pvc, localModelConfig.JobNamespace, localModel); err != nil {
			c.Log.Error(err, "Create PVC err", "name", pv.Name)
		}
	}

	if localModelConfig.DisableVolumeManagement {
		return ctrl.Result{}, nil
	}

	// Step 4 - Creates PV & PVCs for namespaces with isvcs using this model
	err = c.ReconcileForIsvcs(ctx, localModel, nodeGroups, defaultNodeGroup, localModelConfig.JobNamespace)
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

// Given a node object, checks if it matches any node group CR, then reconcile all local models that has this node group to create download jobs.
func (c *LocalModelReconciler) localmodelNodeFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	localmodelNode := obj.(*v1alpha1.LocalModelNode)
	requests := []reconcile.Request{}
	for _, modelInfo := range localmodelNode.Spec.LocalModels {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: modelInfo.ModelName,
			},
		})
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
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.PersistentVolumeClaim{}, ownerKey, func(rawObj client.Object) []string {
		pvc := rawObj.(*corev1.PersistentVolumeClaim)
		owner := metav1.GetControllerOf(pvc)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != modelCacheCRName {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.InferenceService{}, localModelKey, func(rawObj client.Object) []string {
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
			return !isNodeReady(*oldNode) && isNodeReady(*newNode)
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

func isNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// Returns a list of ready nodes, and not ready nodes that matches the node selector in the node group
func getNodesFromNodeGroup(ctx context.Context, nodeGroup *v1alpha1.LocalModelNodeGroup, c client.Client) (*corev1.NodeList, *corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	readyNodes := &corev1.NodeList{}
	notReadyNodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		return nil, nil, err
	}
	for _, node := range nodes.Items {
		matches, err := controllerutils.CheckNodeAffinity(&nodeGroup.Spec.PersistentVolumeSpec, node)
		if err != nil {
			return nil, nil, err
		}
		if matches {
			if isNodeReady(node) {
				readyNodes.Items = append(readyNodes.Items, node)
			} else {
				notReadyNodes.Items = append(notReadyNodes.Items, node)
			}
		}
	}
	return readyNodes, notReadyNodes, nil
}

// DeleteModelFromNode deletes the source model from the localmodelnode
func (c *LocalModelReconciler) DeleteModelFromNode(ctx context.Context, localmodelNode *v1alpha1.LocalModelNode, localModel *v1alpha1.LocalModelCache) error {
	var patch client.Patch
	for i, modelInfo := range localmodelNode.Spec.LocalModels {
		if modelInfo.ModelName == localModel.Name {
			patch = client.MergeFrom(localmodelNode.DeepCopy())
			localmodelNode.Spec.LocalModels = append(localmodelNode.Spec.LocalModels[:i], localmodelNode.Spec.LocalModels[i+1:]...)
			if err := c.Client.Patch(ctx, localmodelNode, patch); err != nil {
				c.Log.Error(err, "Update localmodelnode", "name", localmodelNode.Name)
				return err
			}
			break
		}
	}
	return nil
}

// UpdateLocalModelNode updates the source model uri of the localmodelnode from the localmodel
func (c *LocalModelReconciler) UpdateLocalModelNode(ctx context.Context, localmodelNode *v1alpha1.LocalModelNode, localModel *v1alpha1.LocalModelCache) error {
	var patch client.Patch
	updated := false
	for i, modelInfo := range localmodelNode.Spec.LocalModels {
		if modelInfo.ModelName == localModel.Name {
			if modelInfo.SourceModelUri == localModel.Spec.SourceModelUri {
				return nil
			}
			// Update the source model uri
			c.Log.Info("Unexpected update to sourceModelURI", "node", localmodelNode.Name, "model", localModel.Name)
			updated = true
			patch = client.MergeFrom(localmodelNode.DeepCopy())
			localmodelNode.Spec.LocalModels[i].SourceModelUri = localModel.Spec.SourceModelUri
			break
		}
	}
	if !updated {
		patch = client.MergeFrom(localmodelNode.DeepCopy())
		localmodelNode.Spec.LocalModels = append(localmodelNode.Spec.LocalModels, v1alpha1.LocalModelInfo{ModelName: localModel.Name, SourceModelUri: localModel.Spec.SourceModelUri})
	}
	if err := c.Client.Patch(ctx, localmodelNode, patch); err != nil {
		c.Log.Error(err, "Update localmodelnode", "name", localmodelNode.Name)
		return err
	}
	return nil
}

func nodeStatusFromLocalModelStatus(modelStatus v1alpha1.ModelStatus) v1alpha1.NodeStatus {
	switch modelStatus {
	case v1alpha1.ModelDownloadPending:
		return v1alpha1.NodeDownloadPending
	case v1alpha1.ModelDownloading:
		return v1alpha1.NodeDownloading
	case v1alpha1.ModelDownloadError:
		return v1alpha1.NodeDownloadError
	case v1alpha1.ModelDownloaded:
		return v1alpha1.NodeDownloaded
	}
	return v1alpha1.NodeDownloadPending
}

// ReconcileLocalModelNode creates updates localmodelnode for each node in the node group. It adds and removes localmodels from the localmodelnode and updates the status on the localmodel from the localmodelnode.
func (c *LocalModelReconciler) ReconcileLocalModelNode(ctx context.Context, localModel *v1alpha1.LocalModelCache, nodeGroups map[string]*v1alpha1.LocalModelNodeGroup) error {
	for _, nodeGroup := range nodeGroups {
		readyNodes, notReadyNodes, err := getNodesFromNodeGroup(ctx, nodeGroup, c.Client)
		if err != nil {
			c.Log.Error(err, "getNodesFromNodeGroup node error")
			return err
		}
		if localModel.Status.NodeStatus == nil {
			localModel.Status.NodeStatus = make(map[string]v1alpha1.NodeStatus)
		}
		for _, node := range notReadyNodes.Items {
			if _, ok := localModel.Status.NodeStatus[node.Name]; !ok {
				localModel.Status.NodeStatus[node.Name] = v1alpha1.NodeNotReady
			}
		}
		for _, node := range readyNodes.Items {
			localModelNode := &v1alpha1.LocalModelNode{}
			err := c.Client.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
			found := true
			if err != nil {
				if apierr.IsNotFound(err) {
					found = false
					c.Log.Info("localmodelNode not found")
				} else {
					c.Log.Error(err, "Failed to get localmodelnode", "name", node.Name)
					return err
				}
			}
			if !found {
				localModelNode = &v1alpha1.LocalModelNode{
					ObjectMeta: metav1.ObjectMeta{
						Name: node.Name,
					},
					Spec: v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{{ModelName: localModel.Name, SourceModelUri: localModel.Spec.SourceModelUri}}},
				}
				if err := c.Client.Create(ctx, localModelNode); err != nil {
					c.Log.Error(err, "Create localmodelnode", "name", node.Name)
					return err
				}
			} else {
				if err := c.UpdateLocalModelNode(ctx, localModelNode, localModel); err != nil {
					return err
				}
			}
			modelStatus := localModelNode.Status.ModelStatus[localModel.Name]
			localModel.Status.NodeStatus[node.Name] = nodeStatusFromLocalModelStatus(modelStatus)
		}

		successfulNodes := 0
		failedNodes := 0
		for _, status := range localModel.Status.NodeStatus {
			switch status {
			case v1alpha1.NodeDownloaded:
				successfulNodes += 1
			case v1alpha1.NodeDownloadError:
				failedNodes += 1
			}
		}
		localModel.Status.ModelCopies = &v1alpha1.ModelCopies{Total: len(localModel.Status.NodeStatus), Available: successfulNodes, Failed: failedNodes}
		if err := c.Status().Update(ctx, localModel); err != nil {
			c.Log.Error(err, "cannot update model status from node", "name", localModel.Name)
		}
	}
	return nil
}
