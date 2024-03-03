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
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/s3"
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
	OciURIPrefix                            = "oci://"
	PvcSourceMountName                      = "kserve-pvc-source"
	PvcSourceMountPath                      = "/mnt/pvc"
	CaBundleVolumeName                      = "cabundle-cert"
	ModelcarContainerName                   = "modelcar"
	ModelInitModeEnv                        = "MODEL_INIT_MODE"
	CpuModelcarDefault                      = "10m"
	MemoryModelcarDefault                   = "15Mi"
)

type StorageInitializerConfig struct {
	Image                      string `json:"image"`
	CpuRequest                 string `json:"cpuRequest"`
	CpuLimit                   string `json:"cpuLimit"`
	CpuModelcar                string `json:"cpuModelcar"`
	MemoryRequest              string `json:"memoryRequest"`
	MemoryLimit                string `json:"memoryLimit"`
	CaBundleConfigMapName      string `json:"caBundleConfigMapName"`
	CaBundleVolumeMountPath    string `json:"caBundleVolumeMountPath"`
	MemoryModelcar             string `json:"memoryModelcar"`
	EnableDirectPvcVolumeMount bool   `json:"enableDirectPvcVolumeMount"`
	EnableOciImageSource       bool   `json:"enableModelcar"`
	UidModelcar                *int64 `json:"uidModelcar"`
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
			panic(fmt.Errorf("Unable to unmarshall %v json string due to %w ", StorageInitializerConfigMapKeyName, err))
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

// InjectModelcar injects a sidecar with the full model included to the Pod.
// This so called "modelcar" is then directly accessed from the user container
// via the proc filesystem (possible when `shareProcessNamespace` is enabled in the Pod spec)
func (mi *StorageInitializerInjector) InjectModelcar(pod *v1.Pod) error {
	srcURI, ok := pod.ObjectMeta.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Only inject modelcar if requested
	if !strings.HasPrefix(srcURI, OciURIPrefix) {
		return nil
	}

	// Add an emptyDir Volume to Pod
	pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
		Name: StorageInitializerVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	})

	// Extract image reference for modelcar from URI
	image := strings.TrimPrefix(srcURI, OciURIPrefix)

	userContainer := getContainerWithName(pod, constants.InferenceServiceContainerName)
	if userContainer == nil {
		return fmt.Errorf("no container found with name %s", constants.InferenceServiceContainerName)
	}
	transformerContainer := getContainerWithName(pod, constants.TransformerContainerName)
	// Indicate to the runtime that it the model directory could be
	// available a bit later only so that it should wait and retry when
	// starting up
	addOrReplaceEnv(userContainer, ModelInitModeEnv, "async")

	// Mount volume initialized by the modelcar container to the user container and transformer (if exists)
	modelMount := v1.VolumeMount{
		Name:      StorageInitializerVolumeName,
		MountPath: getParentDirectory(constants.DefaultModelLocalMountPath),
		ReadOnly:  false,
	}
	userContainer.VolumeMounts = append(userContainer.VolumeMounts, modelMount)
	if transformerContainer != nil {
		transformerContainer.VolumeMounts = append(transformerContainer.VolumeMounts, modelMount)
	}

	// If configured, run as the given user. There might be certain installations
	// of Kubernetes where sharing the filesystem via the process namespace only works
	// when both containers are running as root
	if mi.config.UidModelcar != nil {
		userContainer.SecurityContext = &v1.SecurityContext{
			RunAsUser: mi.config.UidModelcar,
		}
	}

	// Create the modelcar that is used as a sidecar in Pod and add it to the end
	// of the containers
	modelContainer := mi.createModelContainer(image, constants.DefaultModelLocalMountPath)
	pod.Spec.Containers = append(pod.Spec.Containers, *modelContainer)

	// Enable process namespace sharing so that the modelcar's root filesystem
	// can be reached by the user container
	shareProcessNamespace := true
	pod.Spec.ShareProcessNamespace = &shareProcessNamespace

	return nil
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

	// Don't inject init-containers if a modelcar is used
	if mi.config.EnableOciImageSource && strings.HasPrefix(srcURI, OciURIPrefix) {
		return nil
	}

	// Don't inject if InitContainer already injected
	for _, container := range pod.Spec.InitContainers {
		if strings.Compare(container.Name, StorageInitializerContainerName) == 0 {
			return nil
		}
	}

	// Find the kserve-container (this is the model inference server) and transformer container
	userContainer := getContainerWithName(pod, constants.InferenceServiceContainerName)
	transformerContainer := getContainerWithName(pod, constants.TransformerContainerName)

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
		if mi.config.EnableDirectPvcVolumeMount {

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

	// Inject CA bundle configMap if caBundleConfigMapName or constants.DefaultGlobalCaBundleConfigMapName annotation is set
	caBundleConfigMapName := mi.config.CaBundleConfigMapName
	if ok := needCaBundleMount(caBundleConfigMapName, initContainer); ok {
		if pod.Namespace != constants.KServeNamespace {
			caBundleConfigMapName = constants.DefaultGlobalCaBundleConfigMapName
		}

		caBundleVolumeMountPath := mi.config.CaBundleVolumeMountPath
		if caBundleVolumeMountPath == "" {
			caBundleVolumeMountPath = constants.DefaultCaBundleVolumeMountPath
		}

		for _, envVar := range initContainer.Env {
			if envVar.Name == s3.AWSCABundleConfigMap {
				caBundleConfigMapName = envVar.Value
			}
			if envVar.Name == s3.AWSCABundle {
				caBundleVolumeMountPath = filepath.Dir(envVar.Value)
			}
		}

		initContainer.Env = append(initContainer.Env, v1.EnvVar{
			Name:  constants.CaBundleConfigMapNameEnvVarKey,
			Value: caBundleConfigMapName,
		})

		initContainer.Env = append(initContainer.Env, v1.EnvVar{
			Name:  constants.CaBundleVolumeMountPathEnvVarKey,
			Value: caBundleVolumeMountPath,
		})

		caBundleVolume := v1.Volume{
			Name: CaBundleVolumeName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: caBundleConfigMapName,
					},
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

	// Add init container to the spec
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, *initContainer)

	return nil
}

// SetIstioCniSecurityContext determines if Istio is installed in using the CNI plugin. If so,
// the UserID of the storage initializer is changed to match the UserID of the Istio sidecar.
// This is to ensure that the storage initializer can access the network.
func (mi *StorageInitializerInjector) SetIstioCniSecurityContext(pod *v1.Pod) error {
	// Find storage initializer container
	var storageInitializerContainer *v1.Container
	for idx, c := range pod.Spec.InitContainers {
		if c.Name == StorageInitializerContainerName {
			storageInitializerContainer = &pod.Spec.InitContainers[idx]
		}
	}

	// If the storage initializer is not injected, there is no action to do
	if storageInitializerContainer == nil {
		return nil
	}

	// Allow to override the uid for the case where ISTIO CNI with DNS proxy is enabled
	// See for more: https://istio.io/latest/docs/setup/additional-setup/cni/#compatibility-with-application-init-containers.
	if value, ok := pod.GetAnnotations()[constants.IstioSidecarUIDAnnotationKey]; ok {
		if uid, err := strconv.ParseInt(value, 10, 64); err == nil {
			if storageInitializerContainer.SecurityContext == nil {
				storageInitializerContainer.SecurityContext = &v1.SecurityContext{}
			}
			storageInitializerContainer.SecurityContext.RunAsUser = ptr.Int64(uid)
		}
	} else {
		// When Istio CNI is disabled, the istio-init container would be present.
		// If it is there, there is no need to touch the security context of the pod.
		// Reference: https://github.com/istio/istio/blob/d533e52acc54b4721d23b1332aea1f234ecbe3e6/pkg/config/analysis/analyzers/maturity/maturity.go#L134
		for _, container := range pod.Spec.InitContainers {
			if container.Name == constants.IstioInitContainerName {
				return nil
			}
		}

		// When Istio CNI is enabled, a sidecar.istio.io/interceptionMode annotation is injected to the pod.
		// There are three interception modes: REDIRECT, TPROXY and NONE.
		// It only makes sense to adjust the security context of the storage initializer if REDIRECT mode is
		// observed, because the Istio sidecar would be injected and traffic would be sent to it, but the
		// sidecar won't be running at PodInitialization phase.
		// The TPROXY mode can indicate that Istio Ambient is enabled. The Waypoint proxy would already be running and
		// captured traffic can go through.
		// The TPROXY mode can also be set by the user. If this is the case, it is not possible to infer the setup.
		// Lastly, if interception mode is NONE the traffic is not being captured. This is an advanced mode, and
		// it is not possible to infer the setup.
		istioInterceptionMode := pod.Annotations[constants.IstioInterceptionModeAnnotation]
		if istioInterceptionMode != constants.IstioInterceptModeRedirect {
			return nil
		}

		// The storage initializer can only run smoothly when running with the same UID as the Istio sidecar.
		// First, find the name of the Istio sidecar container. This is found in a status annotation injected
		// by Istio. If there is no Istio sidecar status annotation, assume that the pod does
		// not have a sidecar and leave untouched the security context.
		istioStatus, istioStatusOk := pod.Annotations[constants.IstioSidecarStatusAnnotation]
		if !istioStatusOk {
			return nil
		}

		// Decode the Istio status JSON document
		var istioStatusDecoded interface{}
		if err := json.Unmarshal([]byte(istioStatus), &istioStatusDecoded); err != nil {
			return err
		}

		// Get the Istio sidecar container name.
		istioSidecarContainerName := ""
		istioStatusMap := istioStatusDecoded.(map[string]interface{})
		if istioContainers, istioContainersOk := istioStatusMap["containers"].([]interface{}); istioContainersOk {
			if len(istioContainers) > 0 {
				istioSidecarContainerName = istioContainers[0].(string)
			}
		}

		// If there is no Istio sidecar, it is not possible to set any UID.
		if len(istioSidecarContainerName) == 0 {
			return nil
		}

		// Find the Istio sidecar container in the pod.
		var istioSidecarContainer *v1.Container
		for idx, container := range pod.Spec.Containers {
			if container.Name == istioSidecarContainerName {
				istioSidecarContainer = &pod.Spec.Containers[idx]
				break
			}
		}

		// Set the UserID of the storage initializer to the same as the Istio sidecar
		if istioSidecarContainer != nil {
			if storageInitializerContainer.SecurityContext == nil {
				storageInitializerContainer.SecurityContext = &v1.SecurityContext{}
			}
			if istioSidecarContainer.SecurityContext == nil || istioSidecarContainer.SecurityContext.RunAsUser == nil {
				// If the Istio sidecar does not explicitly have a UID set, use 1337 which is the
				// UID hardcoded in Istio. This would require privileges to run with AnyUID, which should
				// be OK because, otherwise, the Istio sidecar also would not work correctly.
				storageInitializerContainer.SecurityContext.RunAsUser = ptr.Int64(constants.DefaultIstioSidecarUID)
			} else {
				// If the Istio sidecar has a UID copy it to the storage initializer because this
				// would be the UID that allows access the network.
				sidecarUID := *istioSidecarContainer.SecurityContext.RunAsUser
				storageInitializerContainer.SecurityContext.RunAsUser = ptr.Int64(sidecarUID)

				// Notice that despite in standard Istio the 1337 UID is hardcoded, there exist
				// other flavors, like Maistra, that allow using arbitrary UIDs on the sidecar.
				// The need is the same: the storage-initializer needs to run with the UID of
				// the sidecar to be able to access the network. This is why copying the UID is
				// preferred over using the default UID of 1337.
			}

			log.V(1).Info("Storage initializer UID is set", "pod", pod.Name, "uid", storageInitializerContainer.SecurityContext.RunAsUser)
		}
	}

	return nil
}

func getContainerWithName(pod *v1.Pod, name string) *v1.Container {
	for idx, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, name) == 0 {
			return &pod.Spec.Containers[idx]
		}
	}
	return nil
}

// Add an environment variable with the given value to the environments
// variables of the given container, potentially replacing an env var that already exists
// with this name
func addOrReplaceEnv(container *v1.Container, envKey string, envValue string) {
	if container.Env == nil {
		container.Env = []v1.EnvVar{}
	}

	for i, envVar := range container.Env {
		if envVar.Name == envKey {
			container.Env[i].Value = envValue
			return
		}
	}

	container.Env = append(container.Env, v1.EnvVar{
		Name:  envKey,
		Value: envValue,
	})
}

func (mi *StorageInitializerInjector) createModelContainer(image string, modelPath string) *v1.Container {
	cpu := mi.config.CpuModelcar
	if cpu == "" {
		cpu = CpuModelcarDefault
	}
	memory := mi.config.MemoryModelcar
	if memory == "" {
		memory = MemoryModelcarDefault
	}

	modelContainer := &v1.Container{
		Name:  ModelcarContainerName,
		Image: image,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      StorageInitializerVolumeName,
				MountPath: getParentDirectory(modelPath),
				ReadOnly:  false,
			},
		},
		Args: []string{
			"sh",
			"-c",
			// $$$$ gets escaped by YAML to $$, which is the current PID
			fmt.Sprintf("ln -s /proc/$$$$/root/models %s && sleep infinity", modelPath),
		},
		Resources: v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				// Could possibly be reduced to even less
				v1.ResourceCPU:    resource.MustParse(cpu),
				v1.ResourceMemory: resource.MustParse(memory),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(cpu),
				v1.ResourceMemory: resource.MustParse(memory),
			},
		},
		TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
	}

	if mi.config.UidModelcar != nil {
		modelContainer.SecurityContext = &v1.SecurityContext{
			RunAsUser: mi.config.UidModelcar,
		}
	}

	return modelContainer
}

// GetParentDirectory returns the parent directory of the given path,
// or "/" if the path is a top-level directory.
func getParentDirectory(path string) string {
	// Get the parent directory
	parentDir := filepath.Dir(path)

	// Check if it's a top-level directory
	if parentDir == "." || parentDir == "/" {
		return "/"
	}

	return parentDir
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

func needCaBundleMount(caBundleConfigMapName string, initContainer *v1.Container) bool {
	result := false
	if caBundleConfigMapName != "" {
		result = true
	}
	for _, envVar := range initContainer.Env {
		if envVar.Name == s3.AWSCABundleConfigMap {
			result = true
			break
		}
	}
	return result
}
