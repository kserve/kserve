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

package pod

import (
	"fmt"
	"k8s.io/klog"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials"

	v1 "k8s.io/api/core/v1"
)

const (
	StorageInitializerContainerName         = "storage-initializer"
	StorageInitializerConfigMapKeyName      = "storageInitializer"
	StorageInitializerVolumeName            = "kfserving-provision-location"
	StorageInitializerContainerImage        = "gcr.io/kfserving/storage-initializer"
	StorageInitializerContainerImageVersion = "latest"
	PvcURIPrefix                            = "pvc://"
	PvcSourceMountName                      = "kfserving-pvc-source"
	PvcSourceMountPath                      = "/mnt/pvc"
	UserContainerName                       = "user-container"
)

type StorageInitializerConfig struct {
	Image string `json:"image"`
}
type StorageInitializerInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	config            *StorageInitializerConfig
}

// InjectStorageInitializer injects an init container to provision model data
// for the serving container in a unified way across storage tech by injecting
// a provisioning INIT container. This is a work around because KNative does not
// support INIT containers: https://github.com/knative/serving/issues/4307
func (mi *StorageInitializerInjector) InjectStorageInitializer(pod *v1.Pod) error {
	// Only inject if the required annotations are set
	srcURI, ok := pod.ObjectMeta.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Dont inject if InitContianer already injected
	for _, container := range pod.Spec.InitContainers {
		if strings.Compare(container.Name, StorageInitializerContainerName) == 0 {
			return nil
		}
	}

	// Find the knative user-container (this is the model inference server)
	var userContainer *v1.Container
	for idx, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, UserContainerName) == 0 {
			userContainer = &pod.Spec.Containers[idx]
			break
		}
	}

	if userContainer == nil {
		return fmt.Errorf("Invalid configuration: cannot find container: %s", UserContainerName)
	}

	podVolumes := []v1.Volume{}
	storageInitializerMounts := []v1.VolumeMount{}

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
		storageInitializerMounts = append(storageInitializerMounts, pvcSourceVolumeMount)

		// modify the sourceURI to point to the PVC path
		srcURI = PvcSourceMountPath + "/" + pvcPath
	}

	// Create a volume that is shared between the storage-initializer and user-container
	sharedVolume := v1.Volume{
		Name: StorageInitializerVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	podVolumes = append(podVolumes, sharedVolume)

	// Create a write mount into the shared volume
	sharedVolumeWriteMount := v1.VolumeMount{
		Name:      StorageInitializerVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  false,
	}
	storageInitializerMounts = append(storageInitializerMounts, sharedVolumeWriteMount)

	storageInitializerImage := StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion
	if mi.config != nil && mi.config.Image != "" {
		storageInitializerImage = mi.config.Image
	}
	// Add an init container to run provisioning logic to the PodSpec
	initContainer := &v1.Container{
		Name:  StorageInitializerContainerName,
		Image: storageInitializerImage,
		Args: []string{
			srcURI,
			constants.DefaultModelLocalMountPath,
		},
		VolumeMounts: storageInitializerMounts,
	}

	// Add a mount the shared volume on the user-container, update the PodSpec
	sharedVolumeReadMount := v1.VolumeMount{
		Name:      StorageInitializerVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  true,
	}
	userContainer.VolumeMounts = append(userContainer.VolumeMounts, sharedVolumeReadMount)
	podNamespace := pod.Namespace
	// Change the CustomSpecStorageUri env variable value to the default model path if present
	for index, envVar := range userContainer.Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
			userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
		}
	}

	for _, container := range pod.Spec.Containers {
		for _, envVar := range container.Env {
			// Somehow pod namespace is empty when coming into pod mutator, here we need to use
			// the serving namespace populated on queue-proxy to set the pod namespace if it is empty
			if envVar.Name == constants.ServingNamespace && podNamespace == "" {
				klog.Infof("Setting pod namespace from SERVING_NAMESPACE env: %s", envVar.Name)
				podNamespace = envVar.Value
				break
			}
		}
	}

	// Add volumes to the PodSpec
	pod.Spec.Volumes = append(pod.Spec.Volumes, podVolumes...)

	// Inject credentials
	if err := mi.credentialBuilder.CreateSecretVolumeAndEnv(
		podNamespace,
		pod.Spec.ServiceAccountName,
		initContainer,
		&pod.Spec.Volumes,
	); err != nil {
		return err
	}

	// Add init container to the spec
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, *initContainer)

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
