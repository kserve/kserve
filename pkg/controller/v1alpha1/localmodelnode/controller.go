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
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterstoragecontainers,verbs=get;list
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
	"os"
	"reflect"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
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

var (
	defaultJobImage    = "kserve/storage-initializer:latest" // Could be overwritten by the value in the configmap
	FSGroup            *int64                                // Could be overwritten by the value in the configmap
	nodeName           = os.Getenv("NODE_NAME")              // Name of current node
	localModelHostPath = "/mnt/models"                       // Host node directory to store local models
)

// Launches a job if not exist, or return the existing job
func (c *LocalModelNodeReconciler) launchDownloadJob(jobName string, namespace string, localModelNode *v1alpha1api.LocalModelNode, modelInfo v1alpha1api.LocalModelInfo, claimName string, node string) (*batchv1.Job, error) {
	container, err := c.getContainerSpecForStorageUri(modelInfo.SourceModelUri)
	if err != nil {
		return nil, err
	}

	return c.launchJob(jobName, *container, namespace, localModelNode, modelInfo, claimName, node)
}

// Launches a job if not exist, or return the existing job
func (c *LocalModelNodeReconciler) launchJob(jobName string, container v1.Container, namespace string, localModelNode *v1alpha1api.LocalModelNode, modelInfo v1alpha1api.LocalModelInfo, claimName string, node string) (*batchv1.Job, error) {
	jobs := c.Clientset.BatchV1().Jobs(namespace)

	job, err := jobs.Get(context.TODO(), jobName, metav1.GetOptions{})

	// In tests, job is an empty struct, using this bool is easier than checking for empty struct
	jobFound := true
	if err != nil {
		if apierr.IsNotFound(err) {
			jobFound = false
		} else {
			c.Log.Error(err, "Failed to get job", "name", jobName)
			return job, err
		}
	}

	container.Name = jobName
	container.Args = []string{modelInfo.SourceModelUri, localModelHostPath}
	container.VolumeMounts = []v1.VolumeMount{
		{
			MountPath: localModelHostPath,
			Name:      "kserve-pvc-source",
			ReadOnly:  false,
			SubPath:   "models/" + modelInfo.ModelName,
		},
	}
	expectedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName + "dryrun",
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					NodeName:      node,
					Containers:    []v1.Container{container},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: claimName,
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
	dryrunJob, err := jobs.Create(context.TODO(), expectedJob, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	if err != nil {
		return nil, err
	}
	if job != nil && reflect.DeepEqual(job.Spec.Template.Spec, dryrunJob.Spec.Template.Spec) {
		return job, nil
	}
	if jobFound {
		bg := metav1.DeletePropagationBackground
		err = jobs.Delete(context.TODO(), job.Name, metav1.DeleteOptions{
			PropagationPolicy: &bg,
		})
		if err != nil {
			c.Log.Error(err, "Failed to delete job.", "name", job.Name)
			return nil, err
		}
	}

	if err := controllerutil.SetControllerReference(localModelNode, expectedJob, c.Scheme); err != nil {
		c.Log.Error(err, "Failed to set controller reference", "name", modelInfo.ModelName)
		return nil, err
	}
	expectedJob.Name = jobName
	job, err = jobs.Create(context.TODO(), expectedJob, metav1.CreateOptions{})
	c.Log.Info("Creating job", "name", job.Name, "namespace", namespace)
	if err != nil {
		c.Log.Error(err, "Failed to create job.", "name", expectedJob.Name)
		return nil, err
	}
	return job, err
}

// Fetches container spec for model download container, use the default KServe image if not found
func (c *LocalModelNodeReconciler) getContainerSpecForStorageUri(storageUri string) (*v1.Container, error) {
	storageContainers := &v1alpha1api.ClusterStorageContainerList{}
	if err := c.Client.List(context.TODO(), storageContainers); err != nil {
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
		Image:                    defaultJobImage,
		TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
	}
	return defaultContainer, nil
}

func (c *LocalModelNodeReconciler) downloadModels(ctx context.Context, localModelNode *v1alpha1api.LocalModelNode, jobNamespace string) error {
	c.Log.Info("Downloading models to", "node", localModelNode.ObjectMeta.Name)

	for _, modelInfo := range localModelNode.Spec.LocalModels {
		if status, ok := localModelNode.Status.ModelStatus[modelInfo.ModelName]; ok {
			if status == v1alpha1api.ModelDownloaded {
				continue
			}
		}
		jobName := modelInfo.ModelName + "-" + localModelNode.ObjectMeta.Name

		job, err := c.launchDownloadJob(jobName, jobNamespace, localModelNode, modelInfo, modelInfo.ModelName, localModelNode.ObjectMeta.Name)
		if err != nil {
			c.Log.Error(err, "Job error", "name", jobName)
			return err
		}

		localModelNode.Status.ModelStatus = map[string]v1alpha1api.ModelStatus{}
		switch {
		case job.Status.Succeeded > 0:
			localModelNode.Status.ModelStatus[modelInfo.ModelName] = v1alpha1api.ModelDownloaded
		case job.Status.Failed > 0:
			localModelNode.Status.ModelStatus[modelInfo.ModelName] = v1alpha1api.ModelDownloadError
		case job.Status.Ready != nil && *job.Status.Ready > 0:
			localModelNode.Status.ModelStatus[modelInfo.ModelName] = v1alpha1api.ModelDownloading
		default:
			localModelNode.Status.ModelStatus[modelInfo.ModelName] = v1alpha1api.ModelDownloadPending
		}
	}

	if err := c.Status().Update(ctx, localModelNode); err != nil {
		c.Log.Error(err, "Update local model cache status error", "name", localModelNode.Name)
		return err
	}

	return nil
}

func (c *LocalModelNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != nodeName {
		c.Log.Info("Skipping LocalModelNode because it is not for current node", "name", req.Name, "current node", nodeName)
		return reconcile.Result{}, nil
	}

	c.Log.Info("Agent reconciling LocalModelNode", "name", req.Name, "node", nodeName)
	// Create Jobs to download models if the model is not present locally.
	// 1. Check if LocalModelNode CR is for current node
	localModelNode := &v1alpha1api.LocalModelNode{}
	if err := c.Get(ctx, req.NamespacedName, localModelNode); err != nil {
		c.Log.Error(err, "Error getting LocalModelNode", "name", req.Name)
		return reconcile.Result{}, err
	}

	// 2. Kick off download jobs for all models in spec
	localModelConfig, err := v1beta1.NewLocalModelConfig(c.Clientset)
	if err != nil {
		c.Log.Error(err, "Failed to get local model config")
		return reconcile.Result{}, err
	}
	defaultJobImage = localModelConfig.DefaultJobImage
	FSGroup = localModelConfig.FSGroup
	if err := c.downloadModels(ctx, localModelNode, localModelConfig.JobNamespace); err != nil {
		c.Log.Error(err, "Model download err")
		return reconcile.Result{}, err
	}

	// Todo: Add logic to delete models
	return reconcile.Result{}, nil
}

func (c *LocalModelNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.LocalModelNode{}).
		Owns(&batchv1.Job{}).
		Complete(c)
}
