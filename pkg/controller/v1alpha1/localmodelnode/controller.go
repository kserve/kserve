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
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterstoragecontainers,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
package localmodelnode

import (
	"context"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LocalModelNodeReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

var (

//ownerKey         = ".metadata.controller"
//localModelKey    = ".localmodel"
//apiGVStr         = v1alpha1api.SchemeGroupVersion.String()
//modelCacheCRName = "ClusterLocalModel"
//finalizerName    = "localmodel.kserve.io/finalizer"
//defaultJobImage = "kserve/storage-initializer:latest" // Could be overwritten by the value in the configmap
//FSGroup         *int64                                // Could be overwritten by the value in the configmap

)

// // The localmodel is being deleted
//
//	func (c *LocalModelNodeReconciler) deleteModelFromNodes(localModel *v1alpha1.ClusterLocalModel, jobNamespace string) (ctrl.Result, error) {
//		// finalizer does not exists, nothing to do here!
//		if !utils.Includes(localModel.ObjectMeta.Finalizers, finalizerName) {
//			return ctrl.Result{}, nil
//		}
//		c.Log.Info("deleting model", "name", localModel.Name)
//
//		// Todo: Prevent deletion if there are isvcs using this localmodel
//
//		allDone := true
//		for node := range localModel.Status.NodeStatus {
//			jobName := localModel.Name + "-" + node + "-delete"
//
//			job, err := c.launchDeletionJob(jobName, jobNamespace, localModel, localModel.Spec.SourceModelUri, localModel.Name, node)
//			if err != nil {
//				c.Log.Error(err, "Deletion Job err", "name", jobName)
//				return ctrl.Result{}, err
//			}
//			switch {
//			case job.Status.Succeeded > 0:
//				c.Log.Info("Deletion Job succeeded", "name", jobName)
//				localModel.Status.NodeStatus[node] = v1alpha1api.NodeDeleted
//			case job.Status.Failed > 0:
//				allDone = false
//				localModel.Status.NodeStatus[node] = v1alpha1api.NodeDeletionError
//			default:
//				allDone = false
//				localModel.Status.NodeStatus[node] = v1alpha1api.NodeDeleting
//			}
//		}
//		if allDone {
//			patch := client.MergeFrom(localModel.DeepCopy())
//			// remove our finalizer from the list and update it.
//			localModel.ObjectMeta.Finalizers = utils.RemoveString(localModel.ObjectMeta.Finalizers, finalizerName)
//			if err := c.Patch(context.Background(), localModel, patch); err != nil {
//				c.Log.Error(err, "Cannot remove finalizer", "model name", localModel.Name)
//				return ctrl.Result{}, err
//			}
//		}
//		if err := c.Status().Update(context.Background(), localModel); err != nil {
//			return ctrl.Result{}, err
//		}
//
//		// Stop reconciliation as the item is being deleted
//		return ctrl.Result{}, nil
//	}
//
// // Creates a PV and set the localModel as its controller
//
//	func (c *LocalModelNodeReconciler) createPV(spec v1.PersistentVolume, localModel *v1alpha1.ClusterLocalModel) error {
//		persistentVolumes := c.Clientset.CoreV1().PersistentVolumes()
//		if _, err := persistentVolumes.Get(context.TODO(), spec.Name, metav1.GetOptions{}); err != nil {
//			if !apierr.IsNotFound(err) {
//				c.Log.Error(err, "Failed to get PV")
//				return err
//			}
//			c.Log.Info("Create PV", "name", spec.Name)
//			if err := controllerutil.SetControllerReference(localModel, &spec, c.Scheme); err != nil {
//				c.Log.Error(err, "Failed to set controller reference")
//				return err
//			}
//			if _, err := persistentVolumes.Create(context.TODO(), &spec, metav1.CreateOptions{}); err != nil {
//				c.Log.Error(err, "Failed to create PV", "name", spec.Name)
//				return err
//			}
//		}
//		return nil
//	}
//
// // Creates a PVC and sets the localModel as its controller
//
//	func (c *LocalModelNodeReconciler) createPVC(spec v1.PersistentVolumeClaim, namespace string, localModel *v1alpha1.ClusterLocalModel) error {
//		persistentVolumeClaims := c.Clientset.CoreV1().PersistentVolumeClaims(namespace)
//		if _, err := persistentVolumeClaims.Get(context.TODO(), spec.Name, metav1.GetOptions{}); err != nil {
//			if !apierr.IsNotFound(err) {
//				c.Log.Error(err, "Failed to get PVC")
//				return err
//			}
//			c.Log.Info("Create PVC", "name", spec.Name, "namespace", namespace)
//			if err := controllerutil.SetControllerReference(localModel, &spec, c.Scheme); err != nil {
//				c.Log.Error(err, "Set controller reference")
//				return err
//			}
//			if _, err := persistentVolumeClaims.Create(context.TODO(), &spec, metav1.CreateOptions{}); err != nil {
//				c.Log.Error(err, "Failed to create PVC", "name", spec.Name)
//				return err
//			}
//		}
//		return nil
//	}

//func (c *LocalModelNodeReconciler) DownloadModel(ctx context.Context, localModel *v1alpha1api.ClusterLocalModel, nodeGroup *v1alpha1api.LocalModelNodeGroup, pvc v1.PersistentVolumeClaim, jobNamespace string) error {
//	readyNodes, notReadyNodes, err := getNodesFromNodeGroup(nodeGroup, c.Client)
//	if err != nil {
//		return err
//	}
//	c.Log.Info("Downloading to nodes", "node count", len(readyNodes.Items))
//
//	if localModel.Status.NodeStatus == nil {
//		localModel.Status.NodeStatus = make(map[string]v1alpha1api.NodeStatus)
//	}
//	for _, node := range notReadyNodes.Items {
//		if _, ok := localModel.Status.NodeStatus[node.Name]; !ok {
//			localModel.Status.NodeStatus[node.Name] = v1alpha1api.NodeNotReady
//		}
//	}
//	for _, node := range readyNodes.Items {
//		if status, ok := localModel.Status.NodeStatus[node.Name]; ok {
//			if status == v1alpha1api.NodeDownloaded {
//				continue
//			}
//		}
//		jobName := localModel.Name + "-" + node.Name
//		c.Log.Info("Launch download job", "name", jobName)
//		job, err := c.launchDownloadJob(jobName, jobNamespace, localModel, localModel.Spec.SourceModelUri, pvc.Name, node.Name)
//		if err != nil {
//			c.Log.Error(err, "Job error", "name", jobName)
//			return err
//		}
//		switch {
//		case job.Status.Succeeded > 0:
//			localModel.Status.NodeStatus[node.Name] = v1alpha1api.NodeDownloaded
//		case job.Status.Failed > 0:
//			localModel.Status.NodeStatus[node.Name] = v1alpha1api.NodeDownloadError
//		case job.Status.Ready != nil && *job.Status.Ready > 0:
//			localModel.Status.NodeStatus[node.Name] = v1alpha1api.NodeDownloading
//		default:
//			localModel.Status.NodeStatus[node.Name] = v1alpha1api.NodeDownloadPending
//		}
//	}
//
//	successfulNodes := 0
//	failedNodes := 0
//	for _, status := range localModel.Status.NodeStatus {
//		switch status {
//		case v1alpha1api.NodeDownloaded:
//			successfulNodes += 1
//		case v1alpha1api.NodeDownloadError:
//			failedNodes += 1
//		}
//	}
//	localModel.Status.ModelCopies = &v1alpha1api.ModelCopies{Total: len(localModel.Status.NodeStatus), Available: successfulNodes, Failed: failedNodes}
//	c.Log.Info("Update model cache status", "name", localModel.Name)
//	if err := c.Status().Update(context.Background(), localModel); err != nil {
//		c.Log.Error(err, "Update model cache status error", "name", localModel.Name)
//		return err
//	}
//	return nil
//}

//// Get all isvcs with model cache enabled, create pvs and pvcs, remove pvs and pvcs in namespaces without isvcs.
//func (c *LocalModelNodeReconciler) ReconcileForIsvcs(ctx context.Context, localModel *v1alpha1api.ClusterLocalModel, nodeGroup *v1alpha1api.LocalModelNodeGroup, jobNamespace string) error {
//	isvcs := &v1beta1.InferenceServiceList{}
//	if err := c.Client.List(context.TODO(), isvcs, client.MatchingFields{localModelKey: localModel.Name}); err != nil {
//		c.Log.Error(err, "List isvc error")
//		return err
//	}
//	isvcNames := []v1alpha1.NamespacedName{}
//	// namespaces with isvcs deployed
//	namespaces := make(map[string]struct{})
//	for _, isvc := range isvcs.Items {
//		isvcNames = append(isvcNames, v1alpha1.NamespacedName{Name: isvc.Name, Namespace: isvc.Namespace})
//		namespaces[isvc.Namespace] = struct{}{}
//	}
//	localModel.Status.InferenceServices = isvcNames
//	if err := c.Status().Update(context.TODO(), localModel); err != nil {
//		c.Log.Error(err, "cannot update status", "name", localModel.Name)
//	}
//
//	// Remove PVs and PVCs if the namespace does not have isvcs
//	pvcs := v1.PersistentVolumeClaimList{}
//	if err := c.List(ctx, &pvcs, client.MatchingFields{ownerKey: localModel.Name}); err != nil {
//		c.Log.Error(err, "unable to list PVCs", "name", localModel.Name)
//		return err
//	}
//	for _, pvc := range pvcs.Items {
//		if _, ok := namespaces[pvc.Namespace]; !ok {
//			if pvc.Namespace == jobNamespace {
//				// Keep PVCs in modelCacheNamespace as they don't have a corresponding inference service
//				continue
//			}
//			c.Log.Info("deleting pvc ", "name", pvc.Name, "namespace", pvc.Namespace)
//			persistentVolumeClaims := c.Clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace)
//			if err := persistentVolumeClaims.Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{}); err != nil {
//				c.Log.Error(err, "deleting PVC ", "name", pvc.Name, "namespace", pvc.Namespace)
//			}
//			c.Log.Info("deleting pv", "name", pvc.Name+"-"+pvc.Namespace)
//			persistentVolumes := c.Clientset.CoreV1().PersistentVolumes()
//			if err := persistentVolumes.Delete(context.TODO(), pvc.Name+"-"+pvc.Namespace, metav1.DeleteOptions{}); err != nil {
//				c.Log.Error(err, "deleting PV err")
//			}
//		}
//	}
//
//	for namespace := range namespaces {
//		pv := v1.PersistentVolume{
//			ObjectMeta: metav1.ObjectMeta{
//				Name: localModel.Name + "-" + namespace,
//			},
//			Spec: nodeGroup.Spec.PersistentVolumeSpec,
//		}
//		if err := c.createPV(pv, localModel); err != nil {
//			c.Log.Error(err, "Create PV err", "name", pv.Name)
//		}
//
//		pvc := v1.PersistentVolumeClaim{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      localModel.Name,
//				Namespace: namespace,
//			},
//			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
//		}
//		pvc.Spec.VolumeName = pv.Name
//		if err := c.createPVC(pvc, namespace, localModel); err != nil {
//			c.Log.Error(err, "Create PVC err", "name", pvc.Name)
//		}
//	}
//	return nil
//}

// Step 1 - Checks if the CR is in the deletion process, if so, it creates deletion jobs to delete models on all nodes.
// Step 2 - Creates PV & PVC for model download
// Step 3 - Creates Jobs on all nodes to download models
// Step 4 - Creates PV & PVCs for namespaces with isvcs using this cached model
func (c *LocalModelNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.Log.Info("Reconciling localmodelnode", "name", req.Name)
	return reconcile.Result{}, nil
}

//localModelConfig, err := v1beta1.NewLocalModelConfig(c.Clientset)
//if err != nil {
//	c.Log.Error(err, "Failed to get local model config")
//	return reconcile.Result{}, err
//}
//defaultJobImage = localModelConfig.DefaultJobImage
//FSGroup = localModelConfig.FSGroup
//
//localModel := &v1alpha1api.ClusterLocalModel{}
//if err := c.Get(ctx, req.NamespacedName, localModel); err != nil {
//	return reconcile.Result{}, err
//}
//
//nodeGroup := &v1alpha1api.LocalModelNodeGroup{}
//nodeGroupNamespacedName := types.NamespacedName{Name: localModel.Spec.NodeGroup}
//if err := c.Get(ctx, nodeGroupNamespacedName, nodeGroup); err != nil {
//	return reconcile.Result{}, err
//}
//
//// Step 1 - Checks if the CR is in the deletion process, if so, creates deletion jobs to delete models on all nodes.
//if localModel.ObjectMeta.DeletionTimestamp.IsZero() {
//	// The object is not being deleted, so if it does not have our finalizer,
//	// then lets add the finalizer and update the object. This is equivalent
//	// registering our finalizer.
//	if !utils.Includes(localModel.ObjectMeta.Finalizers, finalizerName) {
//		patch := client.MergeFrom(localModel.DeepCopy())
//		localModel.ObjectMeta.Finalizers = append(localModel.ObjectMeta.Finalizers, finalizerName)
//		if err := c.Patch(context.Background(), localModel, patch); err != nil {
//			return ctrl.Result{}, err
//		}
//	}
//} else {
//	return c.deleteModelFromNodes(localModel, localModelConfig.JobNamespace)
//}
//
//// Step 2 - Creates PV & PVC for model download
//pvSpec := nodeGroup.Spec.PersistentVolumeSpec
//pv := v1.PersistentVolume{Spec: pvSpec, ObjectMeta: metav1.ObjectMeta{
//	Name: localModel.Name + "-download",
//}}
//if err := c.createPV(pv, localModel); err != nil {
//	c.Log.Error(err, "Create PV err", "name", pv.Name)
//}
//
//pvc := v1.PersistentVolumeClaim{
//	ObjectMeta: metav1.ObjectMeta{
//		Name: localModel.Name,
//	},
//	Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
//}
//pvc.Spec.VolumeName = pv.Name
//
//if err := c.createPVC(pvc, localModelConfig.JobNamespace, localModel); err != nil {
//	c.Log.Error(err, "Create PVC err", "name", pv.Name)
//}
//
//// Step 3 - Creates Jobs on all nodes to download models
//if err := c.DownloadModel(ctx, localModel, nodeGroup, pvc, localModelConfig.JobNamespace); err != nil {
//	c.Log.Error(err, "Model download err", "model", localModel.Name)
//}
//
//// Step 4 - Creates PV & PVCs for namespaces with isvcs using this model
//err = c.ReconcileForIsvcs(ctx, localModel, nodeGroup, localModelConfig.JobNamespace)
//return ctrl.Result{}, err
//}

//// Reconciles corresponding model cache CR when we found an update on an isvc
//func (c *LocalModelNodeReconciler) isvcFunc(ctx context.Context, obj client.Object) []reconcile.Request {
//	isvc := obj.(*v1beta1.InferenceService)
//	if isvc.Labels == nil {
//		return []reconcile.Request{}
//	}
//	var modelName string
//	var ok bool
//	if modelName, ok = isvc.Labels[constants.LocalModelLabel]; !ok {
//		return []reconcile.Request{}
//	}
//	localModel := &v1alpha1api.ClusterLocalModel{}
//	if err := c.Get(ctx, types.NamespacedName{Name: modelName}, localModel); err != nil {
//		c.Log.Error(err, "error getting localModel", "name", modelName)
//		return []reconcile.Request{}
//	}
//
//	c.Log.Info("Reconcile localModel from inference services", "name", modelName)
//
//	return []reconcile.Request{{
//		NamespacedName: types.NamespacedName{
//			Name: modelName,
//		}}}
//}

//// Given a node object, checks if it matches any node group CR, then reconcile all local models that has this node group to create download jobs.
//func (c *LocalModelNodeReconciler) nodeFunc(ctx context.Context, obj client.Object) []reconcile.Request {
//	node := obj.(*v1.Node)
//	requests := []reconcile.Request{}
//	models := &v1alpha1.ClusterLocalModelList{}
//	if err := c.Client.List(context.TODO(), models); err != nil {
//		return []reconcile.Request{}
//	}
//
//	for _, model := range models.Items {
//		nodeGroup := &v1alpha1api.LocalModelNodeGroup{}
//		nodeGroupNamespacedName := types.NamespacedName{Name: model.Spec.NodeGroup}
//		if err := c.Get(ctx, nodeGroupNamespacedName, nodeGroup); err != nil {
//			c.Log.Info("get nodegroup failed", "name", model.Spec.NodeGroup)
//			continue
//		}
//		matches, err := checkNodeAffinity(&nodeGroup.Spec.PersistentVolumeSpec, *node)
//		if err != nil {
//			c.Log.Error(err, "checkNodeAffinity error", "node", node.Name)
//		}
//		if matches {
//			c.Log.Info("new node for model", "name", model.Name)
//			requests = append(requests, reconcile.Request{
//				NamespacedName: types.NamespacedName{
//					Name: model.Name,
//				}})
//		}
//	}
//	return requests
//}

func (c *LocalModelNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.PersistentVolumeClaim{}, ownerKey, func(rawObj client.Object) []string {
	//	pvc := rawObj.(*v1.PersistentVolumeClaim)
	//	owner := metav1.GetControllerOf(pvc)
	//	if owner == nil {
	//		return nil
	//	}
	//	if owner.APIVersion != apiGVStr || owner.Kind != modelCacheCRName {
	//		return nil
	//	}
	//
	//	return []string{owner.Name}
	//}); err != nil {
	//	return err
	//}
	//
	//if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.InferenceService{}, localModelKey, func(rawObj client.Object) []string {
	//	isvc := rawObj.(*v1beta1.InferenceService)
	//	if model, ok := isvc.GetLabels()[constants.LocalModelLabel]; ok {
	//		return []string{model}
	//	}
	//	return nil
	//}); err != nil {
	//	return err
	//}
	//
	//isvcPredicates := predicate.Funcs{
	//	UpdateFunc: func(e event.UpdateEvent) bool {
	//		return e.ObjectOld.GetLabels()[constants.LocalModelLabel] != e.ObjectNew.GetLabels()[constants.LocalModelLabel]
	//	},
	//	CreateFunc: func(e event.CreateEvent) bool {
	//		if _, ok := e.Object.GetLabels()[constants.LocalModelLabel]; !ok {
	//			return false
	//		}
	//		return true
	//	},
	//	DeleteFunc: func(e event.DeleteEvent) bool {
	//		if _, ok := e.Object.GetLabels()[constants.LocalModelLabel]; !ok {
	//			return false
	//		}
	//		return true
	//	},
	//}
	//
	//nodePredicates := predicate.Funcs{
	//	UpdateFunc: func(e event.UpdateEvent) bool {
	//		// Only reconciles the local model crs when the node becomes ready from not ready
	//		// Todo: add tests
	//		oldNode := e.ObjectNew.(*v1.Node)
	//		newNode := e.ObjectNew.(*v1.Node)
	//		return !isNodeReady(*oldNode) && isNodeReady(*newNode)
	//	},
	//	CreateFunc: func(e event.CreateEvent) bool {
	//		// Do nothing here, generates local model cr reconcile requests in nodeFunc
	//		return true
	//	},
	//	DeleteFunc: func(e event.DeleteEvent) bool {
	//		return false
	//	},
	//}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.LocalModelNode{}).
		// Ownes Jobs, PersistentVolumes and PersistentVolumeClaims that is created by this local model controller
		//Owns(&batchv1.Job{}).
		//Owns(&v1.PersistentVolume{}).
		//Owns(&v1.PersistentVolumeClaim{}).
		// Creates or deletes pv/pvcs when isvcs got created or deleted
		//Watches(&v1beta1.InferenceService{}, handler.EnqueueRequestsFromMapFunc(c.isvcFunc), builder.WithPredicates(isvcPredicates)).
		// Downloads models to new nodes
		//Watches(&v1.Node{}, handler.EnqueueRequestsFromMapFunc(c.nodeFunc), builder.WithPredicates(nodePredicates)).
		Complete(c)
}

//	func (c *LocalModelNodeReconciler) launchDeletionJob(jobName string, namespace string, localModel *v1alpha1api.ClusterLocalModel, storageUri string, claimName string, node string) (*batchv1.Job, error) {
//		container, err := c.getContainerSpecForStorageUri(storageUri)
//		if err != nil {
//			return nil, err
//		}
//		container.Command = []string{"/bin/sh", "-c", "rm -rf /mnt/models/*"}
//		container.Args = nil
//		return c.launchJob(jobName, *container, namespace, localModel, storageUri, claimName, node)
//	}

// Launches a job if not exist, or return the existing job
//func (c *LocalModelNodeReconciler) launchDownloadJob(jobName string, namespace string, localModel *v1alpha1api.ClusterLocalModel, storageUri string, claimName string, node string) (*batchv1.Job, error) {
//	container, err := c.getContainerSpecForStorageUri(storageUri)
//	if err != nil {
//		return nil, err
//	}
//
//	return c.launchJob(jobName, *container, namespace, localModel, storageUri, claimName, node)
//}

// Launches a job if not exist, or return the existing job
//func (c *LocalModelNodeReconciler) launchJob(jobName string, container v1.Container, namespace string, localModel *v1alpha1api.ClusterLocalModel, storageUri string, claimName string, node string) (*batchv1.Job, error) {
//	jobs := c.Clientset.BatchV1().Jobs(namespace)
//
//	job, err := jobs.Get(context.TODO(), jobName, metav1.GetOptions{})
//
//	// In tests, job is an empty struct, using this bool is easier than checking for empty struct
//	jobFound := true
//	if err != nil {
//		if apierr.IsNotFound(err) {
//			jobFound = false
//		} else {
//			c.Log.Error(err, "Failed to get job", "name", jobName)
//			return job, err
//		}
//	}
//
//	container.Name = jobName
//	container.Args = []string{storageUri, "/mnt/models"}
//	container.VolumeMounts = []v1.VolumeMount{
//		{
//			MountPath: "/mnt/models",
//			Name:      "kserve-pvc-source",
//			ReadOnly:  false,
//			SubPath:   "models/" + localModel.Name,
//		},
//	}
//	expectedJob := &batchv1.Job{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      jobName + "dryrun",
//			Namespace: namespace,
//		},
//		Spec: batchv1.JobSpec{
//			Template: v1.PodTemplateSpec{
//				Spec: v1.PodSpec{
//					NodeName:      node,
//					Containers:    []v1.Container{container},
//					RestartPolicy: v1.RestartPolicyNever,
//					Volumes: []v1.Volume{
//						{
//							Name: "kserve-pvc-source",
//							VolumeSource: v1.VolumeSource{
//								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
//									ClaimName: claimName,
//								},
//							},
//						},
//					},
//					SecurityContext: &v1.PodSecurityContext{
//						FSGroup: FSGroup,
//					},
//				},
//			},
//		},
//	}
//	dryrunJob, err := jobs.Create(context.TODO(), expectedJob, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
//	if err != nil {
//		return nil, err
//	}
//	if job != nil && reflect.DeepEqual(job.Spec.Template.Spec, dryrunJob.Spec.Template.Spec) {
//		return job, nil
//	}
//
//	if jobFound {
//		bg := metav1.DeletePropagationBackground
//		err = jobs.Delete(context.TODO(), job.Name, metav1.DeleteOptions{
//			PropagationPolicy: &bg,
//		})
//		if err != nil {
//			c.Log.Error(err, "Failed to delete job.", "name", job.Name)
//			return nil, err
//		}
//	}
//
//	if err := controllerutil.SetControllerReference(localModel, expectedJob, c.Scheme); err != nil {
//		c.Log.Error(err, "Failed to set controller reference", "name", localModel.Name)
//		return nil, err
//	}
//	expectedJob.Name = jobName
//	job, err = jobs.Create(context.TODO(), expectedJob, metav1.CreateOptions{})
//	c.Log.Info("Creating job", "name", job.Name, "namespace", namespace)
//	if err != nil {
//		c.Log.Error(err, "Failed to create job.", "name", expectedJob.Name)
//		return nil, err
//	}
//	return job, err
//}

//func isNodeReady(node v1.Node) bool {
//	for _, condition := range node.Status.Conditions {
//		if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
//			return true
//		}
//	}
//	return false
//}

// Returns true if the node matches the node affinity specified in the PV Spec
//func checkNodeAffinity(pvSpec *v1.PersistentVolumeSpec, node v1.Node) (bool, error) {
//	if pvSpec.NodeAffinity == nil || pvSpec.NodeAffinity.Required == nil {
//		return false, nil
//	}
//
//	terms := pvSpec.NodeAffinity.Required
//	if matches, err := corev1.MatchNodeSelectorTerms(&node, terms); err != nil {
//		return matches, nil
//	} else {
//		return matches, err
//	}
//}

// Returns a list of ready nodes, and not ready nodes that matches the node selector in the node group
//func getNodesFromNodeGroup(nodeGroup *v1alpha1api.LocalModelNodeGroup, c client.Client) (*v1.NodeList, *v1.NodeList, error) {
//	nodes := &v1.NodeList{}
//	readyNodes := &v1.NodeList{}
//	notReadyNodes := &v1.NodeList{}
//	if err := c.List(context.TODO(), nodes); err != nil {
//		return nil, nil, err
//	}
//	for _, node := range nodes.Items {
//		matches, err := checkNodeAffinity(&nodeGroup.Spec.PersistentVolumeSpec, node)
//		if err != nil {
//			return nil, nil, err
//		}
//		if matches {
//			if isNodeReady(node) {
//				readyNodes.Items = append(readyNodes.Items, node)
//			} else {
//				notReadyNodes.Items = append(notReadyNodes.Items, node)
//			}
//		}
//	}
//	return readyNodes, notReadyNodes, nil
//}

// Fetches container spec for model download container, use the default KServe image if not found
//func (c *LocalModelNodeReconciler) getContainerSpecForStorageUri(storageUri string) (*v1.Container, error) {
//	storageContainers := &v1alpha1.ClusterStorageContainerList{}
//	if err := c.Client.List(context.TODO(), storageContainers); err != nil {
//		return nil, err
//	}
//
//	for _, sc := range storageContainers.Items {
//		if sc.IsDisabled() {
//			continue
//		}
//		if sc.Spec.WorkloadType != v1alpha1.LocalModelDownloadJob {
//			continue
//		}
//		supported, err := sc.Spec.IsStorageUriSupported(storageUri)
//		if err != nil {
//			return nil, fmt.Errorf("error checking storage container %s: %w", sc.Name, err)
//		}
//		if supported {
//			return &sc.Spec.Container, nil
//		}
//	}
//
//	defaultContainer := &v1.Container{
//		Image:                    defaultJobImage,
//		TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
//	}
//	return defaultContainer, nil
//}
