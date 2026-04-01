//go:build distro

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

package localmodelnode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const MountPath = "/var/lib/kserve"

var validMCSLevel = regexp.MustCompile(`^s\d+(-s\d+)?(:(c\d{1,4})(,c\d{1,4})*)?$`)

func enhanceDownloadJob(ctx context.Context, c *LocalModelNodeReconciler, job *batchv1.Job, storageKey string) error {
	containers := job.Spec.Template.Spec.Containers
	if len(containers) == 0 || len(containers[0].VolumeMounts) == 0 || len(containers[0].Args) == 0 {
		return errors.New("download job spec is missing required containers, volume mounts, or args")
	}
	container := &job.Spec.Template.Spec.Containers[0]
	container.VolumeMounts[0].SubPath = ""
	container.Args = []string{container.Args[0], filepath.Join(MountPath, "models", storageKey)}

	podSecurityContext := &corev1.PodSecurityContext{}
	if FSGroup != nil {
		podSecurityContext.RunAsUser = FSGroup
		podSecurityContext.RunAsGroup = FSGroup
		podSecurityContext.FSGroup = FSGroup
	}
	mcsLevel, err := c.resolveMCSLevel(ctx, jobNamespace)
	if err != nil {
		return fmt.Errorf("failed to resolve MCS level: %w", err)
	}
	podSecurityContext.SELinuxOptions = &corev1.SELinuxOptions{
		Level: mcsLevel,
	}
	job.Spec.Template.Spec.SecurityContext = podSecurityContext
	job.Spec.Template.Spec.ServiceAccountName = "kserve-localmodelnode-agent"
	return nil
}

func ensureModelRootFolderExistsAndIsWritable(ctx context.Context, c *LocalModelNodeReconciler,
	localModelConfig *v1beta1.LocalModelConfig,
) (*ensureModelRootFolderResult, error) {
	// Create model root folder — tolerate permission errors
	if err := fsHelper.ensureModelRootFolderExists(); err != nil {
		if os.IsPermission(err) {
			c.Log.Info("Model root folder not writable, will launch permission fix job", "path", modelsRootFolder, "error", err)
		} else {
			return nil, fmt.Errorf("failed to ensure model root folder: %w", err)
		}
	}

	// If already writable, nothing to do
	if isModelRootWritable() {
		return &ensureModelRootFolderResult{Continue: true}, nil
	}

	c.Log.Info("Model root directory is not writable, launching permission fix job", "path", modelsRootFolder)

	// Load OpenShift config for permission fix image
	openshiftConfig, err := v1beta1.NewOpenShiftConfig(c.IsvcConfigMap)
	if err != nil {
		c.Log.Error(err, "Failed to get OpenShift config")
		return nil, err
	}

	permissionFixImage := openshiftConfig.ModelcachePermissionFixImage
	if permissionFixImage == "" {
		return nil, errors.New("modelcachePermissionFixImage not configured in inferenceservice-config")
	}
	mcsLevel, err := c.resolveMCSLevel(ctx, localModelConfig.JobNamespace)
	if err != nil {
		c.Log.Error(err, "Invalid MCS level")
		return nil, err
	}

	// Fetch the LocalModelNode to set as owner of the permission fix job
	lmn := &v1alpha1.LocalModelNode{}
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, lmn); err != nil {
		return nil, fmt.Errorf("failed to get LocalModelNode for owner reference: %w", err)
	}

	if err := c.launchPermissionFixJob(ctx, mcsLevel, permissionFixImage, lmn); err != nil {
		c.Log.Error(err, "Failed to launch permission fix job")
		return nil, err
	}
	return &ensureModelRootFolderResult{Result: ctrl.Result{RequeueAfter: 10 * time.Second}}, nil
}

func getProcessMCSLevel() string {
	data, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		return ""
	}
	parts := strings.SplitN(strings.Trim(string(data), "\x00 \n\r"), ":", 4)
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

func (c *LocalModelNodeReconciler) resolveMCSLevel(ctx context.Context, namespace string) (string, error) {
	mcsLevel := getProcessMCSLevel()
	if mcsLevel != "" {
		if !validMCSLevel.MatchString(mcsLevel) {
			return "", fmt.Errorf("invalid MCS level from process: %q", mcsLevel)
		}
		c.Log.Info("Read MCS level from agent process", "mcsLevel", mcsLevel)
		return mcsLevel, nil
	}

	ns, err := c.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		c.Log.Info("Could not get namespace for MCS annotation", "namespace", namespace, "error", err)
		return "", nil
	}
	if mcs, ok := ns.Annotations["openshift.io/sa.scc.mcs"]; ok {
		mcs = strings.TrimSpace(mcs)
		if !validMCSLevel.MatchString(mcs) {
			return "", fmt.Errorf("invalid MCS level from namespace annotation: %q", mcs)
		}
		c.Log.Info("Using namespace MCS level", "namespace", namespace, "mcsLevel", mcs)
		return mcs, nil
	}
	return "s0", nil
}

func (c *LocalModelNodeReconciler) launchPermissionFixJob(ctx context.Context, mcsLevel string, permissionFixImage string, owner *v1alpha1.LocalModelNode) error {
	jobName := "fix-permissions-" + nodeName

	existingJobs := &batchv1.JobList{}
	fixLabels := map[string]string{
		"fix-permissions": "true",
		"node":            nodeName,
	}
	if err := c.List(ctx, existingJobs, client.InNamespace(jobNamespace), client.MatchingLabels(fixLabels)); err != nil {
		return err
	}
	if len(existingJobs.Items) > 0 {
		job := &existingJobs.Items[0]
		if job.Status.Failed > 0 {
			c.Log.Error(fmt.Errorf("permission fix job %s failed", job.Name),
				"Ensure the service account has 'use' permission on kserve-localmodel-permissions-scc")
			_ = c.Clientset.BatchV1().Jobs(jobNamespace).Delete(ctx, job.Name, metav1.DeleteOptions{
				PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
			})
			return fmt.Errorf("permission fix job %s failed, will retry", job.Name)
		}
		c.Log.Info("Permission fix job already exists", "node", nodeName, "job", job.Name)
		return nil
	}

	pvcName := "kserve-localmodelnode-pvc"
	rootUser := int64(0)
	permFixTTL := int32(60)

	var uid, gid int64
	if FSGroup != nil {
		uid = *FSGroup
		gid = *FSGroup
	} else {
		uid = int64(os.Getuid())
		gid = int64(os.Getgid())
	}

	initSecurityContext := &corev1.SecurityContext{
		RunAsUser:                &rootUser,
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"CHOWN", "DAC_OVERRIDE", "FOWNER"},
		},
	}

	env := []corev1.EnvVar{
		{Name: "FIX_UID", Value: strconv.FormatInt(uid, 10)},
		{Name: "FIX_GID", Value: strconv.FormatInt(gid, 10)},
		{Name: "TARGET", Value: MountPath},
	}

	// Script is a Go constant, never interpolated with user data.
	// SECURITY: double-quoting on variable expansions is critical —
	// it prevents word splitting and shell injection. Do not remove the quotes.
	script := `set -eu; chown -R "$FIX_UID:$FIX_GID" "$TARGET" && chcon -R -t container_file_t "$TARGET"`
	if mcsLevel != "" {
		env = append(env, corev1.EnvVar{Name: "MCS_LEVEL", Value: mcsLevel})
		script = `set -eu; chown -R "$FIX_UID:$FIX_GID" "$TARGET" && chcon -R -t container_file_t -l "$MCS_LEVEL" "$TARGET"`
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: jobName,
			Namespace:    jobNamespace,
			Labels:       fixLabels,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &permFixTTL,
			BackoffLimit:            ptr.To(int32(0)),
			ActiveDeadlineSeconds:   ptr.To(int64(120)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "kserve-localmodel-permfix",
					NodeName:           nodeName,
					RestartPolicy:      corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						SELinuxOptions: &corev1.SELinuxOptions{
							Type:  "spc_t",
							Level: mcsLevel,
						},
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "fix-permissions",
							Image:           permissionFixImage,
							Command:         []string{"sh", "-c", script},
							Env:             env,
							SecurityContext: initSecurityContext,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      PvcSourceMountName,
									MountPath: MountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: PvcSourceMountName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(owner, job, c.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on permission fix job: %w", err)
	}

	createdJob, err := c.Clientset.BatchV1().Jobs(jobNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create permission fix job: %w", err)
	}
	c.Log.Info("Created permission fix job", "name", createdJob.Name, "node", nodeName)
	return nil
}
