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
	"maps"
	"slices"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	controllerutils "github.com/kserve/kserve/pkg/controller/v1alpha1/utils"
	"github.com/kserve/kserve/pkg/utils"
)

// Common constants used by both reconcilers
var (
	OwnerKey                    = ".metadata.controller"
	LocalModelKey               = ".localmodel"
	LocalModelNamespaceKey      = ".localmodelnamespace"
	APIGVStr                    = v1alpha1.SchemeGroupVersion.String()
	ModelCacheCRName            = "LocalModelCache"
	ModelNamespaceCacheCRName   = "LocalModelNamespaceCache"
	FinalizerName               = "localmodel.kserve.io/finalizer"
	NamespaceCacheFinalizerName = "localmodelnamespacecache.kserve.io/finalizer"
)

// LocalModelParams holds common parameters extracted from either LocalModelCache or LocalModelNamespaceCache
type LocalModelParams struct {
	Name               string
	Namespace          string // Empty for cluster-scoped LocalModelCache
	SourceModelUri     string
	NodeGroups         []string
	ServiceAccountName string
	Storage            *v1alpha1.LocalModelStorageSpec
	Finalizers         []string
	FinalizerName      string
	IsNamespaceScoped  bool
}

// ExtractLocalModelParams extracts common parameters from either LocalModelCache or LocalModelNamespaceCache
// Only one of the parameters should be non-nil
func ExtractLocalModelParams(localModelCache *v1alpha1.LocalModelCache, localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache) LocalModelParams {
	if localModelCache != nil {
		return LocalModelParams{
			Name:               localModelCache.Name,
			Namespace:          "", // Cluster-scoped
			SourceModelUri:     localModelCache.Spec.SourceModelUri,
			NodeGroups:         localModelCache.Spec.NodeGroups,
			ServiceAccountName: localModelCache.Spec.ServiceAccountName,
			Storage:            localModelCache.Spec.Storage,
			Finalizers:         localModelCache.ObjectMeta.Finalizers,
			FinalizerName:      FinalizerName,
			IsNamespaceScoped:  false,
		}
	}
	if localModelNamespaceCache != nil {
		return LocalModelParams{
			Name:               localModelNamespaceCache.Name,
			Namespace:          localModelNamespaceCache.Namespace,
			SourceModelUri:     localModelNamespaceCache.Spec.SourceModelUri,
			NodeGroups:         localModelNamespaceCache.Spec.NodeGroups,
			ServiceAccountName: localModelNamespaceCache.Spec.ServiceAccountName,
			Storage:            localModelNamespaceCache.Spec.Storage,
			Finalizers:         localModelNamespaceCache.ObjectMeta.Finalizers,
			FinalizerName:      NamespaceCacheFinalizerName,
			IsNamespaceScoped:  true,
		}
	}
	return LocalModelParams{}
}

// CreateLocalModelInfo creates a LocalModelInfo from either LocalModelCache or LocalModelNamespaceCache
func CreateLocalModelInfo(localModelCache *v1alpha1.LocalModelCache, localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache) v1alpha1.LocalModelInfo {
	params := ExtractLocalModelParams(localModelCache, localModelNamespaceCache)
	return v1alpha1.LocalModelInfo{
		ModelName:          params.Name,
		SourceModelUri:     params.SourceModelUri,
		Namespace:          params.Namespace,
		ServiceAccountName: params.ServiceAccountName,
		Storage:            params.Storage,
	}
}

// IsNodeReady checks if a node is in ready state
func IsNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// GetNodesFromNodeGroup returns a list of ready nodes, and not ready nodes that matches the node selector in the node group
func GetNodesFromNodeGroup(ctx context.Context, nodeGroup *v1alpha1.LocalModelNodeGroup, c client.Client) (*corev1.NodeList, *corev1.NodeList, error) {
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
			if IsNodeReady(node) {
				readyNodes.Items = append(readyNodes.Items, node)
			} else {
				notReadyNodes.Items = append(notReadyNodes.Items, node)
			}
		}
	}
	return readyNodes, notReadyNodes, nil
}

// StorageSpecEqual compares two LocalModelStorageSpec for equality
func StorageSpecEqual(a, b *v1alpha1.LocalModelStorageSpec) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Compare StorageKey
	if (a.StorageKey == nil) != (b.StorageKey == nil) {
		return false
	}
	if a.StorageKey != nil && *a.StorageKey != *b.StorageKey {
		return false
	}
	// Compare Parameters
	if (a.Parameters == nil) != (b.Parameters == nil) {
		return false
	}
	if a.Parameters != nil && !maps.Equal(*a.Parameters, *b.Parameters) {
		return false
	}
	return true
}

// NodeStatusFromLocalModelStatus converts a ModelStatus to NodeStatus
func NodeStatusFromLocalModelStatus(modelStatus v1alpha1.ModelStatus) v1alpha1.NodeStatus {
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

// DeleteModelFromNodes deletes the model from all nodes in the node groups
// Only one of localModelCache or localModelNamespaceCache should be non-nil
// For namespace-scoped models, also explicitly deletes PVs since they cannot have owner references
func DeleteModelFromNodes(
	ctx context.Context,
	c client.Client,
	clientset *kubernetes.Clientset,
	log logr.Logger,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
	nodeGroups map[string]*v1alpha1.LocalModelNodeGroup,
) (ctrl.Result, error) {
	params := ExtractLocalModelParams(localModelCache, localModelNamespaceCache)

	// finalizer does not exist, nothing to do here!
	if !utils.Includes(params.Finalizers, params.FinalizerName) {
		return ctrl.Result{}, nil
	}
	log.Info("deleting model", "name", params.Name, "namespace", params.Namespace)

	for nodeGroupName, nodeGroup := range nodeGroups {
		readyNodes, notReadyNodes, err := GetNodesFromNodeGroup(ctx, nodeGroup, c)
		if err != nil {
			log.Error(err, "getNodesFromNodeGroup node error")
			return ctrl.Result{}, err
		}
		for _, node := range append(readyNodes.Items, notReadyNodes.Items...) {
			localModelNode := &v1alpha1.LocalModelNode{}
			err := c.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
			if err != nil {
				if apierr.IsNotFound(err) {
					log.Info("localmodelNode not found", "node", node.Name)
					continue
				} else {
					log.Error(err, "Failed to get localmodelnode", "name", node.Name)
					return ctrl.Result{}, err
				}
			}

			if err := DeleteModelFromNode(ctx, c, log, localModelNode, params.Name, params.Namespace); err != nil {
				log.Error(err, "failed to delete model from localModelNode", "localModelNode", localModelNode.Name)
				return ctrl.Result{}, err
			}
		}

		// For namespace-scoped LocalModelNamespaceCache, PVs cannot have owner references
		// (Kubernetes limitation: cluster-scoped resources cannot be owned by namespace-scoped resources)
		// So we must explicitly delete them here
		if params.IsNamespaceScoped {
			// Delete download PV: {modelName}-{nodeGroup}-{namespace}-download
			downloadPVName := params.Name + "-" + nodeGroupName + "-" + params.Namespace + "-download"
			if err := DeletePV(ctx, clientset, log, downloadPVName); err != nil {
				log.Error(err, "failed to delete download PV", "name", downloadPVName)
				// Continue with cleanup, don't return error for PV deletion failures
			}

			// Delete serving PV: {modelName}-{nodeGroup}-{namespace}
			servingPVName := params.Name + "-" + nodeGroupName + "-" + params.Namespace
			if err := DeletePV(ctx, clientset, log, servingPVName); err != nil {
				log.Error(err, "failed to delete serving PV", "name", servingPVName)
				// Continue with cleanup, don't return error for PV deletion failures
			}
		}
	}

	// Remove finalizer
	if localModelCache != nil {
		patch := client.MergeFrom(localModelCache.DeepCopy())
		localModelCache.ObjectMeta.Finalizers = utils.RemoveString(localModelCache.ObjectMeta.Finalizers, params.FinalizerName)
		if err := c.Patch(ctx, localModelCache, patch); err != nil {
			log.Error(err, "Cannot remove finalizer", "model name", params.Name)
			return ctrl.Result{}, err
		}
	} else if localModelNamespaceCache != nil {
		patch := client.MergeFrom(localModelNamespaceCache.DeepCopy())
		localModelNamespaceCache.ObjectMeta.Finalizers = utils.RemoveString(localModelNamespaceCache.ObjectMeta.Finalizers, params.FinalizerName)
		if err := c.Patch(ctx, localModelNamespaceCache, patch); err != nil {
			log.Error(err, "Cannot remove finalizer", "model name", params.Name)
			return ctrl.Result{}, err
		}
	}

	// Stop reconciliation as the item is being deleted
	return ctrl.Result{}, nil
}

// DeleteModelFromNode deletes a model entry from a LocalModelNode
func DeleteModelFromNode(ctx context.Context, c client.Client, log logr.Logger, localModelNode *v1alpha1.LocalModelNode, modelName, namespace string) error {
	var patch client.Patch
	for i, modelInfo := range localModelNode.Spec.LocalModels {
		// Match by name and namespace
		if modelInfo.ModelName == modelName && modelInfo.Namespace == namespace {
			patch = client.MergeFrom(localModelNode.DeepCopy())
			localModelNode.Spec.LocalModels = append(localModelNode.Spec.LocalModels[:i], localModelNode.Spec.LocalModels[i+1:]...)
			if err := c.Patch(ctx, localModelNode, patch); err != nil {
				log.Error(err, "Update localmodelnode", "name", localModelNode.Name)
				return err
			}
			break
		}
	}
	return nil
}

// DeletePV deletes a PersistentVolume by name
func DeletePV(ctx context.Context, clientset *kubernetes.Clientset, log logr.Logger, name string) error {
	persistentVolumes := clientset.CoreV1().PersistentVolumes()
	if _, err := persistentVolumes.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if apierr.IsNotFound(err) {
			// PV doesn't exist, nothing to delete
			return nil
		}
		log.Error(err, "Failed to get PV for deletion", "name", name)
		return err
	}
	log.Info("Deleting PV", "name", name)
	if err := persistentVolumes.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierr.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to delete PV", "name", name)
		return err
	}
	return nil
}

// DeletePVC deletes a PersistentVolumeClaim by name and namespace
func DeletePVC(ctx context.Context, clientset *kubernetes.Clientset, log logr.Logger, name, namespace string) error {
	persistentVolumeClaims := clientset.CoreV1().PersistentVolumeClaims(namespace)
	if _, err := persistentVolumeClaims.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if apierr.IsNotFound(err) {
			// PVC doesn't exist, nothing to delete
			return nil
		}
		log.Error(err, "Failed to get PVC for deletion", "name", name, "namespace", namespace)
		return err
	}
	log.Info("Deleting PVC", "name", name, "namespace", namespace)
	if err := persistentVolumeClaims.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierr.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to delete PVC", "name", name, "namespace", namespace)
		return err
	}
	return nil
}

// CreatePV creates a PersistentVolume and sets the owner reference
// Only one of localModelCache or localModelNamespaceCache should be non-nil
// Note: For namespace-scoped LocalModelNamespaceCache, owner reference is NOT set because
// PVs are cluster-scoped and cannot be owned by namespace-scoped resources
func CreatePV(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	scheme *runtime.Scheme,
	log logr.Logger,
	spec corev1.PersistentVolume,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
) error {
	persistentVolumes := clientset.CoreV1().PersistentVolumes()
	if _, err := persistentVolumes.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			log.Error(err, "Failed to get PV")
			return err
		}
		log.Info("Create PV", "name", spec.Name)

		// Set controller reference only for cluster-scoped LocalModelCache
		// PVs are cluster-scoped, so they cannot be owned by namespace-scoped resources
		if localModelCache != nil {
			if err := controllerutil.SetControllerReference(localModelCache, &spec, scheme); err != nil {
				log.Error(err, "Failed to set controller reference")
				return err
			}
		}
		// For LocalModelNamespaceCache, we don't set owner reference since PV is cluster-scoped

		if _, err := persistentVolumes.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			log.Error(err, "Failed to create PV", "name", spec.Name)
			return err
		}
	}
	return nil
}

// CreatePVC creates a PersistentVolumeClaim and sets the owner reference
// Only one of localModelCache or localModelNamespaceCache should be non-nil
func CreatePVC(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	scheme *runtime.Scheme,
	log logr.Logger,
	spec corev1.PersistentVolumeClaim,
	namespace string,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
) error {
	persistentVolumeClaims := clientset.CoreV1().PersistentVolumeClaims(namespace)
	if _, err := persistentVolumeClaims.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			log.Error(err, "Failed to get PVC")
			return err
		}
		log.Info("Create PVC", "name", spec.Name, "namespace", namespace)

		// Set namespace on spec for owner reference check
		spec.Namespace = namespace

		// Set controller reference based on which type is provided
		// For LocalModelCache (cluster-scoped), we can set owner reference
		// For LocalModelNamespaceCache, only set owner reference if the PVC is in the same namespace
		if localModelCache != nil {
			if err := controllerutil.SetControllerReference(localModelCache, &spec, scheme); err != nil {
				log.Error(err, "Set controller reference")
				return err
			}
		} else if localModelNamespaceCache != nil {
			// Only set owner reference if PVC is in the same namespace as the cache
			if namespace == localModelNamespaceCache.Namespace {
				if err := controllerutil.SetControllerReference(localModelNamespaceCache, &spec, scheme); err != nil {
					log.Error(err, "Set controller reference")
					return err
				}
			}
		}

		if _, err := persistentVolumeClaims.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			log.Error(err, "Failed to create PVC", "name", spec.Name)
			return err
		}
	}
	return nil
}

// ReconcileForIsvcs reconciles PVs and PVCs for InferenceServices using this cached model
// Only one of localModelCache or localModelNamespaceCache should be non-nil
// For LocalModelCache: lists all ISVCs matching the model globally
// For LocalModelNamespaceCache: lists ISVCs only in the same namespace
// Also cleans up unused serving PVs/PVCs when ISVCs are removed
func ReconcileForIsvcs(
	ctx context.Context,
	c client.Client,
	clientset *kubernetes.Clientset,
	scheme *runtime.Scheme,
	log logr.Logger,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
	localModelNodeGroups map[string]*v1alpha1.LocalModelNodeGroup,
	defaultNodeGroup *v1alpha1.LocalModelNodeGroup,
) error {
	params := ExtractLocalModelParams(localModelCache, localModelNamespaceCache)

	isvcs := &v1beta1.InferenceServiceList{}

	// List ISVCs based on scope
	if params.IsNamespaceScoped {
		// For namespace-scoped, only list ISVCs in the same namespace
		if err := c.List(ctx, isvcs,
			client.InNamespace(params.Namespace),
			client.MatchingFields{LocalModelNamespaceKey: params.Name}); err != nil {
			log.Error(err, "List isvc error")
			return err
		}
	} else {
		// For cluster-scoped, list all ISVCs matching the model
		if err := c.List(ctx, isvcs, client.MatchingFields{LocalModelKey: params.Name}); err != nil {
			log.Error(err, "List isvc error")
			return err
		}
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
				log.Info("Didn't find isvc node group in model cache node groups", "isvc name", isvc.Name, "isvc node group", isvcNodeGroup, "model cache node groups", slices.Collect(maps.Keys(localModelNodeGroups)))
			}
			// isvc does not have nodegroup annotation. Use default nodegroup
		} else if _, ok := namespaceToNodeGroups[isvc.Namespace]; !ok {
			log.Info("Isvc does not have node group annotation", "isvc name", isvc.Name, "nodegroup annotation", constants.NodeGroupAnnotationKey)
			namespaceToNodeGroups[isvc.Namespace] = map[string]*v1alpha1.LocalModelNodeGroup{defaultNodeGroup.Name: defaultNodeGroup}
		} else {
			namespaceToNodeGroups[isvc.Namespace][defaultNodeGroup.Name] = defaultNodeGroup
		}
	}

	// Get the previous list of namespaces from status to detect removed ISVCs
	var previousNamespaces map[string]bool
	if localModelCache != nil && localModelCache.Status.InferenceServices != nil {
		previousNamespaces = make(map[string]bool)
		for _, isvc := range localModelCache.Status.InferenceServices {
			previousNamespaces[isvc.Namespace] = true
		}
	} else if localModelNamespaceCache != nil && localModelNamespaceCache.Status.InferenceServices != nil {
		previousNamespaces = make(map[string]bool)
		for _, isvc := range localModelNamespaceCache.Status.InferenceServices {
			previousNamespaces[isvc.Namespace] = true
		}
	}

	// Update status with ISVC names
	if localModelCache != nil {
		localModelCache.Status.InferenceServices = isvcNames
		if err := c.Status().Update(ctx, localModelCache); err != nil {
			log.Error(err, "cannot update status", "name", params.Name)
		}
	} else if localModelNamespaceCache != nil {
		localModelNamespaceCache.Status.InferenceServices = isvcNames
		if err := c.Status().Update(ctx, localModelNamespaceCache); err != nil {
			log.Error(err, "cannot update status", "name", params.Name)
		}
	}

	// Determine current namespaces with ISVCs
	currentNamespaces := make(map[string]bool)
	for namespace := range namespaceToNodeGroups {
		currentNamespaces[namespace] = true
	}

	// Clean up serving PVs/PVCs for namespaces that no longer have ISVCs using this model
	for namespace := range previousNamespaces {
		if !currentNamespaces[namespace] {
			// This namespace no longer has ISVCs using this model, clean up
			for nodeGroupName := range localModelNodeGroups {
				pvcName := params.Name + "-" + nodeGroupName
				pvName := pvcName + "-" + namespace

				// For namespace-scoped models, we need to explicitly delete PVs
				// For cluster-scoped models, PVs have owner references and will be garbage collected
				// but we still delete them here for immediate cleanup
				if params.IsNamespaceScoped {
					if err := DeletePV(ctx, clientset, log, pvName); err != nil {
						log.Error(err, "failed to delete serving PV for removed ISVC namespace", "name", pvName)
					}
				}

				// Delete PVC - for cluster-scoped models, PVCs have owner references
				// For namespace-scoped models in other namespaces, they may not have owner references
				if err := DeletePVC(ctx, clientset, log, pvcName, namespace); err != nil {
					log.Error(err, "failed to delete serving PVC for removed ISVC namespace", "name", pvcName, "namespace", namespace)
				}
			}
		}
	}

	// Create PVs and PVCs for each namespace
	for namespace, nodeGroups := range namespaceToNodeGroups {
		for nodeGroupName, nodeGroup := range nodeGroups {
			pvcName := params.Name + "-" + nodeGroupName
			pv := corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: pvcName + "-" + namespace,
				},
				Spec: nodeGroup.Spec.PersistentVolumeSpec,
			}
			if err := CreatePV(ctx, clientset, scheme, log, pv, localModelCache, localModelNamespaceCache); err != nil {
				log.Error(err, "Create PV err", "name", pv.Name)
			}

			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
			}
			pvc.Spec.VolumeName = pv.Name
			if err := CreatePVC(ctx, clientset, scheme, log, pvc, namespace, localModelCache, localModelNamespaceCache); err != nil {
				log.Error(err, "Create PVC err", "name", pvc.Name)
			}
		}
	}
	return nil
}

// UpdateLocalModelNode updates or adds a model to a LocalModelNode
// Only one of localModelCache or localModelNamespaceCache should be non-nil
func UpdateLocalModelNode(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	localModelNode *v1alpha1.LocalModelNode,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
) error {
	params := ExtractLocalModelParams(localModelCache, localModelNamespaceCache)
	newModelInfo := CreateLocalModelInfo(localModelCache, localModelNamespaceCache)

	var patch client.Patch
	updated := false
	for i, modelInfo := range localModelNode.Spec.LocalModels {
		// Match by name and namespace
		if modelInfo.ModelName == params.Name && modelInfo.Namespace == params.Namespace {
			// Check if any field has changed
			needsUpdate := modelInfo.SourceModelUri != params.SourceModelUri ||
				modelInfo.ServiceAccountName != params.ServiceAccountName ||
				!StorageSpecEqual(modelInfo.Storage, params.Storage)
			if !needsUpdate {
				return nil
			}
			// Update the local model info
			log.Info("Updating localModelInfo", "node", localModelNode.Name, "model", params.Name, "namespace", params.Namespace)
			updated = true
			patch = client.MergeFrom(localModelNode.DeepCopy())
			localModelNode.Spec.LocalModels[i] = newModelInfo
			break
		}
	}
	if !updated {
		patch = client.MergeFrom(localModelNode.DeepCopy())
		localModelNode.Spec.LocalModels = append(localModelNode.Spec.LocalModels, newModelInfo)
	}
	if err := c.Patch(ctx, localModelNode, patch); err != nil {
		log.Error(err, "Update localmodelnode", "name", localModelNode.Name)
		return err
	}
	return nil
}

// ReconcileLocalModelNode creates/updates LocalModelNode for each node in the node groups
// Only one of localModelCache or localModelNamespaceCache should be non-nil
func ReconcileLocalModelNode(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	localModelCache *v1alpha1.LocalModelCache,
	localModelNamespaceCache *v1alpha1.LocalModelNamespaceCache,
	nodeGroups map[string]*v1alpha1.LocalModelNodeGroup,
) error {
	params := ExtractLocalModelParams(localModelCache, localModelNamespaceCache)
	modelInfo := CreateLocalModelInfo(localModelCache, localModelNamespaceCache)
	statusKey := modelInfo.GetStatusKey()

	// Get or initialize NodeStatus map
	var nodeStatus map[string]v1alpha1.NodeStatus
	if localModelCache != nil {
		if localModelCache.Status.NodeStatus == nil {
			localModelCache.Status.NodeStatus = make(map[string]v1alpha1.NodeStatus)
		}
		nodeStatus = localModelCache.Status.NodeStatus
	} else if localModelNamespaceCache != nil {
		if localModelNamespaceCache.Status.NodeStatus == nil {
			localModelNamespaceCache.Status.NodeStatus = make(map[string]v1alpha1.NodeStatus)
		}
		nodeStatus = localModelNamespaceCache.Status.NodeStatus
	}

	for _, nodeGroup := range nodeGroups {
		readyNodes, notReadyNodes, err := GetNodesFromNodeGroup(ctx, nodeGroup, c)
		if err != nil {
			log.Error(err, "getNodesFromNodeGroup node error")
			return err
		}

		for _, node := range notReadyNodes.Items {
			if _, ok := nodeStatus[node.Name]; !ok {
				nodeStatus[node.Name] = v1alpha1.NodeNotReady
			}
		}

		for _, node := range readyNodes.Items {
			localModelNode := &v1alpha1.LocalModelNode{}
			err := c.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
			found := true
			if err != nil {
				if apierr.IsNotFound(err) {
					found = false
					log.Info("localmodelNode not found")
				} else {
					log.Error(err, "Failed to get localmodelnode", "name", node.Name)
					return err
				}
			}
			if !found {
				localModelNode = &v1alpha1.LocalModelNode{
					ObjectMeta: metav1.ObjectMeta{
						Name: node.Name,
					},
					Spec: v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{modelInfo}},
				}
				if err := c.Create(ctx, localModelNode); err != nil {
					log.Error(err, "Create localmodelnode", "name", node.Name)
					return err
				}
			} else {
				if err := UpdateLocalModelNode(ctx, c, log, localModelNode, localModelCache, localModelNamespaceCache); err != nil {
					return err
				}
			}
			// Use status key to look up status
			modelStatus := localModelNode.Status.ModelStatus[statusKey]
			nodeStatus[node.Name] = NodeStatusFromLocalModelStatus(modelStatus)
		}

		successfulNodes := 0
		failedNodes := 0
		for _, status := range nodeStatus {
			switch status {
			case v1alpha1.NodeDownloaded:
				successfulNodes += 1
			case v1alpha1.NodeDownloadError:
				failedNodes += 1
			}
		}

		// Update status
		modelCopies := &v1alpha1.ModelCopies{Total: len(nodeStatus), Available: successfulNodes, Failed: failedNodes}
		if localModelCache != nil {
			localModelCache.Status.ModelCopies = modelCopies
			if err := c.Status().Update(ctx, localModelCache); err != nil {
				log.Error(err, "cannot update model status from node", "name", params.Name)
			}
		} else if localModelNamespaceCache != nil {
			localModelNamespaceCache.Status.ModelCopies = modelCopies
			if err := c.Status().Update(ctx, localModelNamespaceCache); err != nil {
				log.Error(err, "cannot update model status from node", "name", params.Name, "namespace", params.Namespace)
			}
		}
	}
	return nil
}
