/*
Copyright 2019 kubeflow.org.
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

package deployment

import (
	"fmt"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	ModelProvisioningContainerName    = "model-provisioner"
	ModelProvisioningVolumeName       = "kfserving-provision-location"
	ModelProvisioningContainerImage   = "kcorer/kfdownloader"
	ModelProvisioningContainerVersion = "latest"
	PvcURIPrefix                      = "pvc://"
	PvcSourceMountName                = "kfserving-pvc-source"
	PvcSourceMountPath                = "/mnt/pvc"
	UserContainerName                 = "user-container"
)

// InjectModelProvisioner injects an init container to provision model data
// for the serving container in a unified way across storage tech by injecting
// a provisioning INIT container. This is a work around because KNative does not
// support INIT containers: https://github.com/knative/serving/issues/4307
func InjectModelProvisioner(deployment *appsv1.Deployment) error {

	var srcURI, mountPath string

	annotations := deployment.Spec.Template.ObjectMeta.Annotations
	podSpec := &deployment.Spec.Template.Spec

	// Only inject if the required annotations are set
	if _, ok := annotations[constants.KFServiceModelInitializerSourceURIInternalAnnotationKey]; ok {
		srcURI = annotations[constants.KFServiceModelInitializerSourceURIInternalAnnotationKey]
	} else {
		return nil
	}

	// Only inject provisioning for supported URIs
	if !strings.HasPrefix(srcURI, "gs://") &&
		!strings.HasPrefix(srcURI, "s3://") &&
		!strings.HasPrefix(srcURI, "pvc://") {
		// TODO: would be nice to log something here so that future generations know what happened?
		return nil
	}

	// Find the knative user-container (this is the model inference server)
	userContainerIndex := -1
	for idx, container := range podSpec.Containers {
		if strings.Compare(container.Name, UserContainerName) == 0 {
			userContainerIndex = idx
			break
		}
	}

	if userContainerIndex < 0 {
		return fmt.Errorf("Invalid configuration: cannot find container: %s", UserContainerName)
	}

	podVolumes := []v1.Volume{}
	provisionerMounts := []v1.VolumeMount{}

	// For PVC source URIs we need to mount the source to be able to access it
	// See design and discussion here: https://github.com/kubeflow/kfserving/issues/148
	if strings.HasPrefix(srcURI, PvcURIPrefix) {
		pvcName, pvcPath, err := parsePvcURI(srcURI)
		if err != nil {
			return err
		}

		// add the PVC volume on the pod
		pvcSourceVolume := v1.Volume{
			Name: PvcSourceMountName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		}
		podVolumes = append(podVolumes, pvcSourceVolume)

		// add a corresponding PVC volume mount to the INIT container
		pvcSourceVolumeMount := v1.VolumeMount{
			Name:      PvcSourceMountName,
			MountPath: PvcSourceMountPath,
			ReadOnly:  true,
		}
		provisionerMounts = append(provisionerMounts, pvcSourceVolumeMount)

		// modify the sourceURI to point to the PVC path
		srcURI = PvcSourceMountPath + "/" + pvcPath
	}

	// Create a volume that is shared between the provisioner and user-container
	sharedVolume := v1.Volume{
		Name: ModelProvisioningVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	podVolumes = append(podVolumes, sharedVolume)

	// Create a write mount into the shared volume
	sharedVolumeWriteMount := v1.VolumeMount{
		Name:      ModelProvisioningVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  false,
	}
	provisionerMounts = append(provisionerMounts, sharedVolumeWriteMount)

	// Add an init container to run provisioning logic to the PodSpec
	initContianer := v1.Container{
		Name:  ModelProvisioningContainerName,
		Image: ModelProvisioningContainerImage + ":" + ModelProvisioningContainerVersion,
		Args: []string{
			srcURI,
			constants.DefaultModelLocalMountPath,
		},
		VolumeMounts: provisionerMounts,
	}
	podSpec.InitContainers = append(podSpec.InitContainers, initContianer)

	// Add a mount the shared volume on the user-container, update the PodSpec
	sharedVolumeReadMount := v1.VolumeMount{
		Name:      ModelProvisioningVolumeName,
		MountPath: mountPath,
		ReadOnly:  true,
	}
	podSpec.Containers[userContainerIndex].VolumeMounts = append(podSpec.Containers[userContainerIndex].VolumeMounts, sharedVolumeReadMount)

	// Add volumes to the PodSpec
	podSpec.Volumes = append(podSpec.Volumes, podVolumes...)

	return nil
}

func parsePvcURI(srcURI string) (pvcName string, pvcPath string, err error) {
	parts := strings.Split(strings.TrimPrefix(srcURI, PvcURIPrefix), "/")
	if len(parts) > 1 {
		pvcName = parts[0]
		pvcPath = strings.Join(parts[1:], "/")
	} else if len(parts) == 1 {
		pvcName = parts[0]
		pvcPath = ""
	} else {
		return "", "", fmt.Errorf("Invalid URI must be pvc://<pvcname>/[path]: %s", srcURI)
	}

	return pvcName, pvcPath, nil
}
