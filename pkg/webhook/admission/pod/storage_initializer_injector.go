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
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/strategicpatch"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	PvcSourceMountPath = "/mnt/pvc"
	CaBundleVolumeName = "cabundle-cert"
)

type StorageInitializerInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	config            *types.StorageInitializerConfig
	client            client.Client
}

func GetContainerSpecForStorageUri(ctx context.Context, storageUri string, client client.Client) (*corev1.Container, error) {
	storageContainers := &v1alpha1.ClusterStorageContainerList{}
	if err := client.List(ctx, storageContainers); err != nil {
		return nil, err
	}

	for _, sc := range storageContainers.Items {
		if sc.IsDisabled() {
			continue
		}
		if sc.Spec.WorkloadType != v1alpha1.InitContainer {
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
// via the proc filesystem (possible when `shareProcessNamespace` is enabled in the Pod spec).
// This method is idempotent so can be called multiple times like it happens when the
// webhook is configured with `reinvocationPolicy: IfNeeded`
func (mi *StorageInitializerInjector) InjectModelcar(pod *corev1.Pod) error {
	srcURI, ok := pod.ObjectMeta.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Only inject modelcar if requested
	if !strings.HasPrefix(srcURI, constants.OciURIPrefix) {
		return nil
	}

	if err := utils.ConfigureModelcarToContainer(srcURI, &pod.Spec, constants.InferenceServiceContainerName, mi.config); err != nil {
		return err
	}

	if utils.GetContainerWithName(&pod.Spec, constants.TransformerContainerName) != nil {
		return utils.ConfigureModelcarToContainer(srcURI, &pod.Spec, constants.TransformerContainerName, mi.config)
	}

	return nil
}

// InjectStorageInitializer injects an init container to provision model data
// for the serving container in a unified way across storage tech by injecting
// a provisioning INIT container. This is a workaround because KNative does not
// support INIT containers: https://github.com/knative/serving/issues/4307
func (mi *StorageInitializerInjector) InjectStorageInitializer(pod *corev1.Pod) error {
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
	if mi.config.EnableOciImageSource && strings.HasPrefix(srcURI, constants.OciURIPrefix) {
		return nil
	}

	// Don't inject if InitContainer already injected
	for _, container := range pod.Spec.InitContainers {
		if strings.Compare(container.Name, constants.StorageInitializerContainerName) == 0 {
			return nil
		}
	}

	// Update volume mount's readonly annotation based on the ISVC annotation
	isvcReadonlyStringFlag := true
	isvcReadonlyString, ok := pod.ObjectMeta.Annotations[constants.StorageReadonlyAnnotationKey]
	if ok {
		if isvcReadonlyString == "false" {
			isvcReadonlyStringFlag = false
		}
	}

	// Find the kserve-container (this is the model inference server) and transformer container and the worker-container
	userContainer := utils.GetContainerWithName(&pod.Spec, constants.InferenceServiceContainerName)
	transformerContainer := utils.GetContainerWithName(&pod.Spec, constants.TransformerContainerName)
	workerContainer := utils.GetContainerWithName(&pod.Spec, constants.WorkerContainerName)

	if userContainer == nil {
		if workerContainer == nil {
			return fmt.Errorf("Invalid configuration: cannot find container: %s", constants.InferenceServiceContainerName)
		} else {
			userContainer = workerContainer
		}
	}

	// Mount pvc directly if local model label exists
	if modelName, ok := pod.ObjectMeta.Labels[constants.LocalModelLabel]; ok {
		subPath, _ := strings.CutPrefix(srcURI, pod.ObjectMeta.Annotations[constants.LocalModelSourceUriAnnotationKey])
		if !strings.HasPrefix(subPath, "/") {
			subPath = "/" + subPath
		}
		if pvcName, ok := pod.ObjectMeta.Annotations[constants.LocalModelPVCNameAnnotationKey]; ok {
			srcURI = "pvc://" + pvcName + "/models/" + modelName + subPath
		} else {
			return fmt.Errorf("Annotation %s not found", constants.LocalModelPVCNameAnnotationKey)
		}
	}

	podVolumes := []corev1.Volume{}
	storageInitializerMounts := []corev1.VolumeMount{}

	// For PVC source URIs we need to mount the source to be able to access it
	// See design and discussion here: https://github.com/kserve/kserve/issues/148
	if strings.HasPrefix(srcURI, constants.PvcURIPrefix) {
		// check if using direct volume mount to mount the pvc
		// if yes, mount the pvc to model local mount path and return
		if mi.config.EnableDirectPvcVolumeMount {
			// add a corresponding pvc volume mount to the userContainer and transformerContainer.
			// Pvc will be mount to /mnt/models rather than /mnt/pvc.
			// PvcPath will be injected via SubPath, pvcPath must be a root or Dir.
			// It is user responsibility to ensure it is a root or Dir
			if mountErr := utils.AddModelPvcMount(srcURI, userContainer.Name, isvcReadonlyStringFlag, &pod.Spec); mountErr != nil {
				return mountErr
			}
			if transformerContainer != nil {
				if mountErr := utils.AddModelPvcMount(srcURI, transformerContainer.Name, isvcReadonlyStringFlag, &pod.Spec); mountErr != nil {
					return mountErr
				}
			}

			// change the CustomSpecStorageUri env variable value
			// to the default model path if present
			for index, envVar := range userContainer.Env {
				if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
					userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
				}
			}

			// not inject the storage initializer
			return nil
		}

		// It follows logic for PVC as model storage, using storage-initializer
		pvcName, pvcPath, err := utils.ParsePvcURI(srcURI)
		if err != nil {
			return err
		}

		// add the PVC volume on the pod
		pvcSourceVolume := corev1.Volume{
			Name: constants.PvcSourceMountName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		}
		podVolumes = append(podVolumes, pvcSourceVolume)

		// below use storage initializer to handle the pvc
		// add a corresponding PVC volume mount to the INIT container
		pvcSourceVolumeMount := corev1.VolumeMount{
			Name:      constants.PvcSourceMountName,
			MountPath: PvcSourceMountPath,
			ReadOnly:  isvcReadonlyStringFlag,
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

	initContainer := utils.AddStorageInitializerContainer(&pod.Spec, userContainer.Name, srcURI, isvcReadonlyStringFlag, mi.config)
	if transformerContainer != nil {
		initContainer = utils.AddStorageInitializerContainer(&pod.Spec, transformerContainer.Name, srcURI, isvcReadonlyStringFlag, mi.config)
	}

	initContainer.VolumeMounts = append(initContainer.VolumeMounts, storageInitializerMounts...)

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

		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name:  constants.CaBundleConfigMapNameEnvVarKey,
			Value: caBundleConfigMapName,
		})

		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name:  constants.CaBundleVolumeMountPathEnvVarKey,
			Value: caBundleVolumeMountPath,
		})

		caBundleVolume := corev1.Volume{
			Name: CaBundleVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caBundleConfigMapName,
					},
				},
			},
		}

		caBundleVolumeMount := corev1.VolumeMount{
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
	storageContainerSpec, err := GetContainerSpecForStorageUri(context.Background(), srcURI, mi.client)
	if err != nil {
		return err
	}
	if storageContainerSpec != nil {
		err = mergeContainerSpecs(initContainer, storageContainerSpec)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetIstioCniSecurityContext determines if Istio is installed in using the CNI plugin. If so,
// the UserID of the storage initializer is changed to match the UserID of the Istio sidecar.
// This is to ensure that the storage initializer can access the network.
func (mi *StorageInitializerInjector) SetIstioCniSecurityContext(pod *corev1.Pod) error {
	// Find storage initializer container
	var storageInitializerContainer *corev1.Container
	for idx, c := range pod.Spec.InitContainers {
		if c.Name == constants.StorageInitializerContainerName {
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
				storageInitializerContainer.SecurityContext = &corev1.SecurityContext{}
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
		var istioSidecarContainer *corev1.Container
		for idx, container := range pod.Spec.Containers {
			if container.Name == istioSidecarContainerName {
				istioSidecarContainer = &pod.Spec.Containers[idx]
				break
			}
		}

		// Set the UserID of the storage initializer to the same as the Istio sidecar
		if istioSidecarContainer != nil {
			if storageInitializerContainer.SecurityContext == nil {
				storageInitializerContainer.SecurityContext = &corev1.SecurityContext{}
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

// Use JSON Marshal/Unmarshal to merge Container structs using strategic merge patch.
// Use container name from defaultContainer spec, crdContainer takes precedence for other fields.
func mergeContainerSpecs(targetContainer *corev1.Container, crdContainer *corev1.Container) error {
	if targetContainer == nil {
		return errors.New("targetContainer is nil")
	}

	containerName := targetContainer.Name

	defaultContainerJson, err := json.Marshal(*targetContainer)
	if err != nil {
		return err
	}

	overrides, err := json.Marshal(*crdContainer)
	if err != nil {
		return err
	}

	jsonResult, err := strategicpatch.StrategicMergePatch(defaultContainerJson, overrides, corev1.Container{})
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonResult, targetContainer); err != nil {
		return err
	}

	if targetContainer.Name == "" {
		targetContainer.Name = containerName
	}

	return nil
}

func needCaBundleMount(caBundleConfigMapName string, initContainer *corev1.Container) bool {
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
