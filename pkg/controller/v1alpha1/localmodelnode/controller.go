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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups,verbs=get;list
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterstoragecontainers,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
package localmodelnode

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LocalModelNodeReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

const (
	MountPath             = "/mnt/models" // Volume mount path for models, must be the same as the value in the DaemonSet spec
	DownloadContainerName = "kserve-localmodel-download"
	PvcSourceMountName    = "kserve-pvc-source"
)

var (
	defaultJobImage            = "kserve/storage-initializer:latest" // Can be overwritten by the value in the configmap
	FSGroup                    *int64
	jobNamespace               string
	jobTTLSecondsAfterFinished int32 = 3600                   // One hour. Can be overwritten by the value in the configmap
	nodeName                         = os.Getenv("NODE_NAME") // Name of current node, passed as an env variable via downward API
	modelsRootFolder                 = filepath.Join(MountPath, "models")
	fsHelper                   FileSystemInterface
)

func (c *LocalModelNodeReconciler) launchJob(ctx context.Context, localModelNode v1alpha1api.LocalModelNode, modelInfo v1alpha1api.LocalModelInfo) (*batchv1.Job, error) {
	jobName := modelInfo.ModelName + "-" + localModelNode.ObjectMeta.Name
	container, err := c.getContainerSpecForStorageUri(ctx, modelInfo.SourceModelUri)
	if err != nil {
		return nil, err
	}

	container.Args = []string{modelInfo.SourceModelUri, MountPath}
	container.VolumeMounts = []v1.VolumeMount{
		{
			MountPath: MountPath,
			Name:      PvcSourceMountName,
			ReadOnly:  false,
			SubPath:   filepath.Join("models", modelInfo.ModelName),
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: jobName,
			Namespace:    jobNamespace,
			Labels:       map[string]string{"model": modelInfo.ModelName, "node": localModelNode.Name},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &jobTTLSecondsAfterFinished,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					NodeName:      nodeName,
					Containers:    []v1.Container{*container},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{
							Name: PvcSourceMountName,
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: modelInfo.ModelName,
								},
							},
						},
					},
					SecurityContext: &v1.PodSecurityContext{
						FSGroup: FSGroup,
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(&localModelNode, job, c.Scheme); err != nil {
		c.Log.Error(err, "Failed to set controller reference", "name", modelInfo.ModelName)
		return nil, err
	}
	jobs := c.Clientset.BatchV1().Jobs(jobNamespace)
	job, err = jobs.Create(ctx, job, metav1.CreateOptions{})
	c.Log.Info("Creating job", "name", job.Name, "namespace", job.Namespace)
	if err != nil {
		c.Log.Error(err, "Failed to create job.", "name", job.Name)
		return nil, err
	}
	return job, err
}

// Fetches container spec for model download container, use the default KServe image if not found
func (c *LocalModelNodeReconciler) getContainerSpecForStorageUri(ctx context.Context, storageUri string) (*v1.Container, error) {
	storageContainers := &v1alpha1api.ClusterStorageContainerList{}
	if err := c.Client.List(ctx, storageContainers); err != nil {
		return nil, err
	}

	for _, sc := range storageContainers.Items {
		if sc.IsDisabled() {
			continue
		}
		if sc.Spec.WorkloadType != v1alpha1api.LocalModelDownloadJob {
			continue
		}
		supported, err := sc.Spec.IsStorageUriSupported(storageUri)
		if err != nil {
			return nil, fmt.Errorf("error checking storage container %s: %w", sc.Name, err)
		}
		if supported {
			return &sc.Spec.Container, nil
		}
	}

	defaultContainer := &v1.Container{
		Name:                     DownloadContainerName,
		Image:                    defaultJobImage,
		TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
	}
	return defaultContainer, nil
}

func (c *LocalModelNodeReconciler) getLatestJob(ctx context.Context, modelName string, nodeName string, excludeSucceeded bool) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	labelSelector := map[string]string{
		"model": modelName,
		"node":  nodeName,
	}
	if err := c.Client.List(ctx, jobList, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector)); err != nil {
		if errors.IsNotFound(err) {
			c.Log.Info("Job not found", "model", modelName)
			return nil, nil
		}
		return nil, err
	}
	c.Log.Info("Found jobs", "model", modelName, "num of jobs", len(jobList.Items))
	var latestJob *batchv1.Job
	for i, job := range jobList.Items {
		if excludeSucceeded && job.Status.Succeeded > 0 {
			continue
		}
		if latestJob == nil || job.CreationTimestamp.After(latestJob.CreationTimestamp.Time) {
			latestJob = &jobList.Items[i]
		}
	}
	return latestJob, nil
}

func getModelStatusFromJobStatus(jobStatus batchv1.JobStatus) v1alpha1api.ModelStatus {
	switch {
	case jobStatus.Succeeded > 0:
		return v1alpha1api.ModelDownloaded
	case jobStatus.Failed > 0:
		return v1alpha1api.ModelDownloadError
	case jobStatus.Ready != nil && *jobStatus.Ready > 0:
		return v1alpha1api.ModelDownloading
	default:
		return v1alpha1api.ModelDownloadPending
	}
}

// Create jobs to download models if the model is not present locally
// Update the status of the LocalModelNode CR
func (c *LocalModelNodeReconciler) downloadModels(ctx context.Context, localModelNode *v1alpha1api.LocalModelNode) error {
	newStatus := map[string]v1alpha1api.ModelStatus{}
	for _, modelInfo := range localModelNode.Spec.LocalModels {
		c.Log.Info("checking model from spec", "model", modelInfo.ModelName)
		var job *batchv1.Job
		folderExists, err := fsHelper.hasModelFolder(modelInfo.ModelName)
		if err != nil {
			c.Log.Error(err, "Failed to check model folder", "model", modelInfo.ModelName)
			return err
		}
		if folderExists {
			c.Log.Info("Model folder found", "model", modelInfo.ModelName)
			// If folder exists and the job has been successfully completed, do nothing
			// If the job is cleaned up, no new job is created because the status is already set to ModelDownloaded
			if status, ok := localModelNode.Status.ModelStatus[modelInfo.ModelName]; ok {
				if status == v1alpha1api.ModelDownloaded {
					newStatus[modelInfo.ModelName] = v1alpha1api.ModelDownloaded
					continue
				}
			}
			job, err = c.getLatestJob(ctx, modelInfo.ModelName, nodeName, false)
			if err != nil {
				c.Log.Error(err, "Failed to getLatestJob", "model", modelInfo.ModelName, "node", nodeName)
				return err
			}
			// If job is not found, create a new one. Because download could be incomplete.
			if job == nil {
				c.Log.Info("Model folder exists, creating download job", "model", modelInfo.ModelName)
				job, err = c.launchJob(ctx, *localModelNode, modelInfo)
				if err != nil {
					c.Log.Error(err, "Failed to create Job", "model", modelInfo.ModelName, "node", nodeName)
					return err
				}
			}
		} else {
			// Folder does not exist
			c.Log.Info("Model folder not found", "model", modelInfo.ModelName)
			job, err = c.getLatestJob(ctx, modelInfo.ModelName, nodeName, true)
			if err != nil {
				c.Log.Error(err, "Failed to getLatestJob", "model", modelInfo.ModelName, "node", nodeName)
				return err
			}
			if job != nil {
				c.Log.Info("model status from latest job", "model", modelInfo.ModelName, "status", getModelStatusFromJobStatus(job.Status))
			}
			// Recreate job if it has been terminated because the model is missing locally
			// If the job has failed, we do not retry here because there are retries on the job.
			// To retry the download, users can manually fix the issue and delete the failed job.
			if job == nil || job.Status.Succeeded > 0 {
				c.Log.Info("Download model", "model", modelInfo.ModelName)
				job, err = c.launchJob(ctx, *localModelNode, modelInfo)
				if err != nil {
					c.Log.Error(err, "Failed to create Job", "model", modelInfo.ModelName, "node", nodeName)
					return err
				}
			}
		}
		newStatus[modelInfo.ModelName] = getModelStatusFromJobStatus(job.Status)

		c.Log.Info("Downloading models:", "model", modelInfo.ModelName,
			"node", localModelNode.ObjectMeta.Name, "status", newStatus[modelInfo.ModelName])
	}

	// Skip update if no changes to status
	if maps.Equal(localModelNode.Status.ModelStatus, newStatus) {
		return nil
	}

	localModelNode.Status.ModelStatus = newStatus
	if err := c.Status().Update(ctx, localModelNode); err != nil {
		c.Log.Error(err, "Update local model cache status error", "name", localModelNode.Name)
		return err
	}
	c.Log.Info("status updated", "name", localModelNode.Name, "num of models in status", len(localModelNode.Status.ModelStatus))

	return nil
}

// Delete models that are not in the spec
func (c *LocalModelNodeReconciler) deleteModels(localModelNode v1alpha1api.LocalModelNode) error {
	// 1. Scan model dir and get a list of existing folders representing downloaded models
	foldersToRemove := map[string]struct{}{}
	entries, err := fsHelper.getModelFolders()
	if err != nil {
		c.Log.Error(err, "Failed to list model folder")
	}
	for _, entry := range entries {
		// Models could only exist in sub dir
		if entry.IsDir() {
			foldersToRemove[entry.Name()] = struct{}{}
		}
	}

	// 2. Compare with list of models from LocalModelNode CR
	for _, localModelInfo := range localModelNode.Spec.LocalModels {
		// Remove expected models from local model set
		delete(foldersToRemove, localModelInfo.ModelName)
	}
	// 3. Models not in LocalModelNode CR spec should be deleted
	if len(foldersToRemove) != 0 {
		c.Log.Info("Found model(s) to remove", "num of models", len(foldersToRemove))
		for modelName := range foldersToRemove {
			c.Log.Info("Removing model", "model", modelName)
			if err := fsHelper.removeModel(modelName); err != nil {
				c.Log.Error(err, "Failed to remove model directory", "model", modelName)
			}
		}
	}
	return nil
}

func (c *LocalModelNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != nodeName {
		c.Log.Info("Skipping LocalModelNode because it is not for current node", "name", req.Name, "current node", nodeName)
		return reconcile.Result{}, nil
	}

	c.Log.Info("Agent reconciling LocalModelNode", "name", req.Name, "node", nodeName)

	// fsHelper is a global variable to allow mocking in tests
	if fsHelper == nil {
		fsHelper = NewFileSystemHelper(modelsRootFolder)
	}

	// Create Jobs to download models if the model is not present locally.
	// 1. Check if LocalModelNode CR is for current node
	localModelNode := v1alpha1api.LocalModelNode{}
	if err := c.Get(ctx, req.NamespacedName, &localModelNode); err != nil {
		c.Log.Error(err, "Error getting LocalModelNode", "name", req.Name)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Kick off download jobs for all models in spec
	localModelConfig, err := v1beta1.NewLocalModelConfig(c.Clientset)
	if err != nil {
		c.Log.Error(err, "Failed to get local model config")
		return reconcile.Result{}, err
	}
	defaultJobImage = localModelConfig.DefaultJobImage
	jobNamespace = localModelConfig.JobNamespace
	FSGroup = localModelConfig.FSGroup
	if localModelConfig.JobTTLSecondsAfterFinished != nil {
		jobTTLSecondsAfterFinished = *localModelConfig.JobTTLSecondsAfterFinished
	}
	if err := c.downloadModels(ctx, &localModelNode); err != nil {
		c.Log.Error(err, "Model download err")
		return reconcile.Result{}, err
	}

	// 3. Delete models that are not in the spec. This function does not modify the resource.
	if err := c.deleteModels(localModelNode); err != nil {
		c.Log.Error(err, "Model deletion err")
		return reconcile.Result{}, err
	}
	// Requeue to check local folders periodically
	// Todo: Make it configurable
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}

func (c *LocalModelNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.LocalModelNode{}).
		Owns(&batchv1.Job{}).
		Complete(c)
}
