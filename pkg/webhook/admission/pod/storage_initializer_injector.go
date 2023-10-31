/*
Copyright 2021 The KServe Authors.

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
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StorageInitializerContainerName         = "storage-initializer"
	StorageInitializerConfigMapKeyName      = "storageInitializer"
	StorageInitializerVolumeName            = "kserve-provision-location"
	StorageInitializerContainerImage        = "kserve/storage-initializer"
	StorageInitializerContainerImageVersion = "latest"
	PvcURIPrefix                            = "pvc://"
	PvcSourceMountName                      = "kserve-pvc-source"
	PvcSourceMountPath                      = "/mnt/pvc"
	CaBundleVolumeName                      = "custom-certs"
)

type StorageInitializerConfig struct {
	Image                      string `json:"image"`
	CpuRequest                 string `json:"cpuRequest"`
	CpuLimit                   string `json:"cpuLimit"`
	MemoryRequest              string `json:"memoryRequest"`
	MemoryLimit                string `json:"memoryLimit"`
	CaBundleSecretName         string `json:"caBundleSecretName"`
	CaBundleVolumeMountPath    string `json:"caBundleVolumeMountPath"`
	EnableDirectPvcVolumeMount bool   `json:"enableDirectPvcVolumeMount"`
}

type StorageInitializerInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	config            *StorageInitializerConfig
	client            client.Client
}

func getStorageInitializerConfigs(configMap *v1.ConfigMap) (*StorageInitializerConfig, error) {
	storageInitializerConfig := &StorageInitializerConfig{}
	if initializerConfig, ok := configMap.Data[StorageInitializerConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(initializerConfig), &storageInitializerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall %v json string due to %v ", StorageInitializerConfigMapKeyName, err))
		}
	}
	//Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{storageInitializerConfig.MemoryRequest,
		storageInitializerConfig.MemoryLimit,
		storageInitializerConfig.CpuRequest,
		storageInitializerConfig.CpuLimit}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return storageInitializerConfig, fmt.Errorf("Failed to parse resource configuration for %q: %q", StorageInitializerConfigMapKeyName, err.Error())
		}
	}

	return storageInitializerConfig, nil
}

func GetContainerSpecForStorageUri(storageUri string, client client.Client) (*v1.Container, error) {
	storageContainers := &v1alpha1.ClusterStorageContainerList{}
	if err := client.List(context.TODO(), storageContainers); err != nil {
		return nil, err
	}

	for _, sc := range storageContainers.Items {
		if sc.IsDisabled() {
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

	return nil, nil
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

	// Don't inject if model agent is injected
	if _, ok := pod.ObjectMeta.Annotations[constants.AgentShouldInjectAnnotationKey]; ok {
		return nil
	}

	// Don't inject if InitContainer already injected
	for _, container := range pod.Spec.InitContainers {
		if strings.Compare(container.Name, StorageInitializerContainerName) == 0 {
			return nil
		}
	}

	// Find the kserve-container (this is the model inference server) and transformer container
	var userContainer *v1.Container
	var transformerContainer *v1.Container
	for idx, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, constants.InferenceServiceContainerName) == 0 {
			userContainer = &pod.Spec.Containers[idx]
		}
		if container.Name == constants.TransformerContainerName {
			transformerContainer = &pod.Spec.Containers[idx]
		}
	}

	if userContainer == nil {
		return fmt.Errorf("Invalid configuration: cannot find container: %s", constants.InferenceServiceContainerName)
	}

	podVolumes := []v1.Volume{}
	storageInitializerMounts := []v1.VolumeMount{}

	// For PVC source URIs we need to mount the source to be able to access it
	// See design and discussion here: https://github.com/kserve/kserve/issues/148
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

		// check if using direct volume mount to mount the pvc
		// if yes, mount the pvc to model local mount path and return
		if mi.config.EnableDirectPvcVolumeMount == true {

			// add a corresponding pvc volume mount to the userContainer
			// pvc will be mount to /mnt/models rather than /mnt/pvc
			// pvcPath will be injected via SubPath, pvcPath must be a root or Dir
			// it is user responsibility to ensure it is a root or Dir
			pvcSourceVolumeMount := v1.VolumeMount{
				Name:      PvcSourceMountName,
				MountPath: constants.DefaultModelLocalMountPath,
				// only path to volume's root ("") or folder is supported
				SubPath:  pvcPath,
				ReadOnly: true,
			}

			// Check if PVC source URIs is already mounted
			// this may occur when mutator is triggered more than once
			if userContainer.VolumeMounts != nil {
				for _, volumeMount := range userContainer.VolumeMounts {
					if strings.Compare(volumeMount.Name, PvcSourceMountName) == 0 {
						return nil
					}
				}
			}

			userContainer.VolumeMounts = append(userContainer.VolumeMounts, pvcSourceVolumeMount)
			if transformerContainer != nil {
				// Check if PVC source URIs is already mounted
				if transformerContainer.VolumeMounts != nil {
					for _, volumeMount := range transformerContainer.VolumeMounts {
						if strings.Compare(volumeMount.Name, PvcSourceMountName) == 0 {
							return nil
						}
					}
				}

				transformerContainer.VolumeMounts = append(transformerContainer.VolumeMounts, pvcSourceVolumeMount)

				// change the CustomSpecStorageUri env variable value
				// to the default model path if present
				for index, envVar := range transformerContainer.Env {
					if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
						transformerContainer.Env[index].Value = constants.DefaultModelLocalMountPath
					}
				}
			}
			// change the CustomSpecStorageUri env variable value
			// to the default model path if present
			for index, envVar := range userContainer.Env {
				if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
					userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
				}
			}

			// add volumes to the PodSpec
			pod.Spec.Volumes = append(pod.Spec.Volumes, podVolumes...)

			// not inject the storage initializer
			return nil
		}

		// below use storage initializer to handle the pvc
		// add a corresponding PVC volume mount to the INIT container
		pvcSourceVolumeMount := v1.VolumeMount{
			Name:      PvcSourceMountName,
			MountPath: PvcSourceMountPath,
			ReadOnly:  true,
		}
		storageInitializerMounts = append(storageInitializerMounts, pvcSourceVolumeMount)

		// Since the model path is linked from source pvc, userContainer also need to mount the pvc.
		userContainer.VolumeMounts = append(userContainer.VolumeMounts, pvcSourceVolumeMount)
		if transformerContainer != nil {
			transformerContainer.VolumeMounts = append(transformerContainer.VolumeMounts, pvcSourceVolumeMount)
		}
		// modify the sourceURI to point to the PVC path
		srcURI = PvcSourceMountPath + "/" + pvcPath
	}

	// Create a volume that is shared between the storage-initializer and kserve-container
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

	securityContext := userContainer.SecurityContext.DeepCopy()
	// Add an init container to run provisioning logic to the PodSpec
	initContainer := &v1.Container{
		Name:  StorageInitializerContainerName,
		Image: storageInitializerImage,
		Args: []string{
			srcURI,
			constants.DefaultModelLocalMountPath,
		},
		TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
		VolumeMounts:             storageInitializerMounts,
		Resources: v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(mi.config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(mi.config.MemoryLimit),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(mi.config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(mi.config.MemoryRequest),
			},
		},
		SecurityContext: securityContext,
	}

	// Add a mount the shared volume on the kserve-container, update the PodSpec
	sharedVolumeReadMount := v1.VolumeMount{
		Name:      StorageInitializerVolumeName,
		MountPath: constants.DefaultModelLocalMountPath,
		ReadOnly:  true,
	}
	userContainer.VolumeMounts = append(userContainer.VolumeMounts, sharedVolumeReadMount)
	if transformerContainer != nil {
		transformerContainer.VolumeMounts = append(transformerContainer.VolumeMounts, sharedVolumeReadMount)
		// Change the CustomSpecStorageUri env variable value to the default model path if present
		for index, envVar := range transformerContainer.Env {
			if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
				transformerContainer.Env[index].Value = constants.DefaultModelLocalMountPath
			}
		}
	}
	// Change the CustomSpecStorageUri env variable value to the default model path if present
	for index, envVar := range userContainer.Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
			userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
		}
	}

	// Add volumes to the PodSpec
	pod.Spec.Volumes = append(pod.Spec.Volumes, podVolumes...)

	// Inject credentials
	hasStorageSpec := pod.ObjectMeta.Annotations[constants.StorageSpecAnnotationKey]
	storageKey := pod.ObjectMeta.Annotations[constants.StorageSpecKeyAnnotationKey]
	// Inject Storage Spec credentials if exist
	if hasStorageSpec == "true" {
		var overrideParams map[string]string
		if storageSpecParam, ok := pod.ObjectMeta.Annotations[constants.StorageSpecParamAnnotationKey]; ok {
			if err := json.Unmarshal([]byte(storageSpecParam), &overrideParams); err != nil {
				return err
			}
		}
		if err := mi.credentialBuilder.CreateStorageSpecSecretEnvs(
			pod.Namespace,
			pod.Annotations,
			storageKey,
			overrideParams,
			initContainer,
		); err != nil {
			return err
		}
		// initContainer.Args[0] is set up in CreateStorageSpecSecretEnvs
		// srcURI is updated here to match storage container CRs below
		srcURI = initContainer.Args[0]
	} else {
		// Inject service account credentials if storage spec doesn't exist
		if err := mi.credentialBuilder.CreateSecretVolumeAndEnv(
			pod.Namespace,
			pod.Annotations,
			pod.Spec.ServiceAccountName,
			initContainer,
			&pod.Spec.Volumes,
		); err != nil {
			return err
		}
	}
	
	// Inject CA bundle secret if set
	caBundleSecretName := mi.config.CaBundleSecretName
	if caBundleSecretName != "" {
		caBundleVolumeMountPath := mi.config.CaBundleVolumeMountPath
		if caBundleVolumeMountPath == "" {
			caBundleVolumeMountPath = constants.DefaultCaBundleVolumeMountPath
		}
		
		caBundleVolume := v1.Volume{
			Name: CaBundleVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: caBundleSecretName,
				},
			},
		}

		caBundleVolumeMount := v1.VolumeMount{
			Name:      CaBundleVolumeName,
			MountPath: caBundleVolumeMountPath,
			ReadOnly:  true,
		}
		
		pod.Spec.Volumes = append(pod.Spec.Volumes, caBundleVolume)
		initContainer.VolumeMounts = append(initContainer.VolumeMounts, caBundleVolumeMount)
	}

	// Update initContainer (container spec) from a storage container CR if there is a match,
	// otherwise initContainer is not updated.
	// Priority: CR > configMap
	storageContainerSpec, err := GetContainerSpecForStorageUri(srcURI, mi.client)
	if err != nil {
		return err
	}
	if storageContainerSpec != nil {
		initContainer, err = mergeContainerSpecs(initContainer, storageContainerSpec)
		if err != nil {
			return err
		}
	}

	// Allow to override the uid for the case where ISTIO CNI with DNS proxy is enabled
	// See for more: https://istio.io/latest/docs/setup/additional-setup/cni/#compatibility-with-application-init-containers.
	if value, ok := pod.GetAnnotations()[constants.IstioSidecarUIDAnnotationKey]; ok {
		if uid, err := strconv.ParseInt(value, 10, 64); err == nil {
			initContainer.SecurityContext.RunAsUser = ptr.Int64(uid)
		}
	}

	// Add init container to the spec
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, *initContainer)

	return nil
}

// Use JSON Marshal/Unmarshal to merge Container structs using strategic merge patch.
// Use container name from defaultContainer spec, crdContainer takes precedence for other fields.
func mergeContainerSpecs(defaultContainer *v1.Container, crdContainer *v1.Container) (*v1.Container, error) {
	if defaultContainer == nil {
		return nil, fmt.Errorf("defaultContainer is nil")
	}

	containerName := defaultContainer.Name

	defaultContainerJson, err := json.Marshal(*defaultContainer)
	if err != nil {
		return nil, err
	}

	overrides, err := json.Marshal(*crdContainer)
	if err != nil {
		return nil, err
	}

	mergedContainer := v1.Container{}
	jsonResult, err := strategicpatch.StrategicMergePatch(defaultContainerJson, overrides, mergedContainer)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonResult, &mergedContainer); err != nil {
		return nil, err
	}

	if mergedContainer.Name == "" {
		mergedContainer.Name = containerName
	}

	return &mergedContainer, nil
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
