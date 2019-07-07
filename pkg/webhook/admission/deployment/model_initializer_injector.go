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
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/credentials"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	ModelInitializerContainerName         = "model-initializer"
	ModelInitializerConfigMapKeyName      = "modelInitializer"
	ModelInitializerVolumeName            = "kfserving-provision-location"
	ModelInitializerContainerImage        = "gcr.io/kfserving/model-initializer"
	ModelInitializerContainerImageVersion = "latest"
	PvcURIPrefix                          = "pvc://"
	PvcSourceMountName                    = "kfserving-pvc-source"
	PvcSourceMountPath                    = "/mnt/pvc"
	UserContainerName                     = "user-container"
)

type ModelInitializerConfig struct {
	Image string `json:"image"`
}
type ModelInitializerInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	config            *ModelInitializerConfig
}

// InjectModelInitializer injects an init container to provision model data
// for the serving container in a unified way across storage tech by injecting
// a provisioning INIT container. This is a work around because KNative does not
// support INIT containers: https://github.com/knative/serving/issues/4307
func (mi *ModelInitializerInjector) InjectModelInitializer(deployment *appsv1.Deployment) error {

	annotations := deployment.Spec.Template.ObjectMeta.Annotations
	podSpec := &deployment.Spec.Template.Spec

	// Only inject if the required annotations are set
	srcURI, ok := annotations[constants.ModelInitializerSourceUriInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Dont inject if InitContianer already injected
	for _, container := range podSpec.InitContainers {
		if strings.Compare(container.Name, ModelInitializerContainerName) == 0 {
			return nil
		}
	}

	// Find the knative user-container (this is the model inference server)
	var userContainer *v1.Container
	for idx, container := range podSpec.Containers {
		if strings.Compare(container.Name, UserContainerName) == 0 {
			userContainer = &podSpec.Containers[idx]
			break
		}
	}

	if userContainer == nil {
		return fmt.Errorf("Invalid configuration: cannot find container: %s", UserContainerName)
	}

	podVolumes := []v1.Volume{}
	modelInitializerMounts := []v1.VolumeMount{}

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
		modelInitializerMounts = append(modelInitializerMounts, pvcSourceVolumeMount)

		// modify the sourceURI to point to the PVC path
		srcURI = PvcSourceMountPath + "/" + pvcPath
	}

	// Create a volume that is shared between the model-initializer and user-container
	sharedVolume := v1.Volume{
		Name: ModelInitializerVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	podVolumes = append(podVolumes, sharedVolume)

	// Create a write mount into the shared volume
	sharedVolumeWriteMount := v1.VolumeMount{
		Name:      ModelInitializerVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  false,
	}
	modelInitializerMounts = append(modelInitializerMounts, sharedVolumeWriteMount)

	modelInitializerImage := ModelInitializerContainerImage + ":" + ModelInitializerContainerImageVersion
	if mi.config != nil && mi.config.Image != "" {
		modelInitializerImage = mi.config.Image
	}
	// Add an init container to run provisioning logic to the PodSpec
	initContainer := &v1.Container{
		Name:  ModelInitializerContainerName,
		Image: modelInitializerImage,
		Args: []string{
			srcURI,
			constants.DefaultModelLocalMountPath,
		},
		VolumeMounts: modelInitializerMounts,
	}

	// Add a mount the shared volume on the user-container, update the PodSpec
	sharedVolumeReadMount := v1.VolumeMount{
		Name:      ModelInitializerVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  true,
	}
	userContainer.VolumeMounts = append(userContainer.VolumeMounts, sharedVolumeReadMount)

	// Change the CustomSpecModelUri env variable value to the default model path if present
	for index, envVar := range userContainer.Env {
		if envVar.Name == constants.CustomSpecModelUriEnvVarKey && envVar.Value != "" {
			userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
		}
	}

	// Add volumes to the PodSpec
	podSpec.Volumes = append(podSpec.Volumes, podVolumes...)

	// Inject credentials
	if err := mi.credentialBuilder.CreateSecretVolumeAndEnv(
		deployment.Namespace,
		podSpec.ServiceAccountName,
		initContainer,
		&podSpec.Volumes,
	); err != nil {
		return err
	}

	// Add init container to the spec
	podSpec.InitContainers = append(podSpec.InitContainers, *initContainer)

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
