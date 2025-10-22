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
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
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

// StorageInitializerParams contains all the parameters needed for storage initialization
// across both single and multiple storage URI scenarios. This struct encapsulates
// the configuration, context, and resources required to set up model downloading.
type StorageInitializerParams struct {
	// Namespace is the Kubernetes namespace where the InferenceService is deployed
	Namespace string

	// StorageURIs is a slice of storage URIs with their corresponding mount paths.
	// Supports multiple URIs for complex scenarios like base models with adapters.
	// Each StorageUri contains both the source URI and the target mount path.
	StorageURIs []v1beta1.StorageUri

	// IsReadOnly indicates whether the mounted storage should be read-only.
	// When true, volumes are mounted with read-only permissions.
	IsReadOnly bool

	// PodSpec is the pod specification that will be modified to include
	// init containers and volume mounts for model downloading.
	PodSpec *corev1.PodSpec

	// CredentialBuilder provides access to storage credentials for authentication
	// with cloud storage providers (S3, GCS, Azure, etc.).
	CredentialBuilder *credentials.CredentialBuilder

	// Client is the Kubernetes client used for reading ClusterStorageContainer
	// specifications and other cluster resources.
	Client client.Client

	// Config contains the storage initializer configuration including
	// container image, resource limits, and feature flags.
	Config *types.StorageInitializerConfig

	// IsvcAnnotations contains InferenceService annotations that may affect
	// storage initialization behavior (e.g., agent injection flags).
	IsvcAnnotations map[string]string

	// StorageSpec contains legacy storage configuration for backward compatibility.
	// May be nil when using the newer StorageURIs field.
	StorageSpec *v1beta1.StorageSpec

	// StorageContainerSpec specifies a custom storage container for downloading
	// models. When nil, the default storage initializer is used.
	StorageContainerSpec *v1alpha1.StorageContainerSpec

	// Indicates whether to use the legacy logic for storage initialization.
	IsLegacyURI bool
}

// GetStorageContainerSpec finds and returns a ClusterStorageContainer specification
// that supports the given storage URI. This function searches through all available
// ClusterStorageContainer resources in the cluster and returns the first one that
// can handle the specified URI format.
//
// Parameters:
//   - ctx: Context for the Kubernetes API call
//   - storageUri: The storage URI to find a compatible container for (e.g., "s3://bucket/path")
//   - client: Kubernetes client for listing ClusterStorageContainer resources
//
// Returns:
//   - *v1alpha1.StorageContainerSpec: The container specification that supports the URI, or nil if none found
//   - error: Error if the Kubernetes API call fails or URI format checking fails
//
// The function iterates through ClusterStorageContainer resources and uses their
// IsStorageUriSupported method to determine compatibility with the provided URI.
func GetStorageContainerSpec(ctx context.Context, storageUri string, client client.Client) (*v1alpha1.StorageContainerSpec, error) {
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
			return &sc.Spec, nil
		}
	}

	return nil, nil
}

func GetContainerSpecForStorageUri(ctx context.Context, storageUri string, client client.Client) (*corev1.Container, error) {
	supported, err := GetStorageContainerSpec(ctx, storageUri, client)
	if err != nil {
		return nil, fmt.Errorf("error checking storage container %s: %w", supported.Container.Name, err)
	}
	if supported != nil {
		return &supported.Container, nil
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

func GetStorageInitializerReadOnlyFlag(annotations map[string]string) bool {
	isvcReadonlyStringFlag := true
	isvcReadonlyString, ok := annotations[constants.StorageReadonlyAnnotationKey]
	if ok {
		if isvcReadonlyString == "false" {
			isvcReadonlyStringFlag = false
		}
	}
	return isvcReadonlyStringFlag
}

// CommonStorageInitialization handles the injection of storage initialization logic for both single and multiple storage URIs.
// This function consolidates the shared logic for setting up init containers and volume mounts to download model artifacts
// from various storage backends (S3, GCS, Azure Blob, PVC, etc.) before the main inference container starts.
//
// The function supports:
//   - Multiple storage URIs with custom mount paths for complex use cases (e.g., base model + LoRA adapters)
//   - Backward compatibility with single storage URI scenarios
//   - Mixed PVC and cloud storage URI handling
//   - Automatic volume mounting and path management
//   - Storage container specification for custom downloaders
//
// Parameters:
//   - params: A StorageInitializerParams struct containing all necessary configuration including
//     storage URIs, pod spec, credentials, and storage container specifications;
//     podSpec is modified in place to include init containers and volume mounts
//
// Returns:
//   - error: nil on success, or an error if injection fails due to invalid configuration,
//     unsupported storage containers, or volume mounting issues
//
// The function will skip injection in the following cases:
//   - Model agent is already injected (annotation present)
//   - Modelcar (OCI container) is being used instead of init containers
//   - Storage initializer container is already present in the pod
func CommonStorageInitialization(ctx context.Context, params *StorageInitializerParams) error {
	// Don't inject if model agent is injected
	if _, ok := params.IsvcAnnotations[constants.AgentShouldInjectAnnotationKey]; ok {
		return nil
	}

	// Don't inject init-containers if a modelcar is used
	if params.Config.EnableOciImageSource && len(params.StorageURIs) > 0 && strings.HasPrefix(params.StorageURIs[0].Uri, constants.OciURIPrefix) {
		return nil
	}

	// Don't inject if InitContainer already injected
	for _, container := range params.PodSpec.InitContainers {
		if strings.Compare(container.Name, constants.StorageInitializerContainerName) == 0 {
			return nil
		}
	}

	if len(params.StorageURIs) == 0 {
		// No storage URIs provided - noop
		return nil
	}

	var initContainer *corev1.Container
	numStorageURIs := len(params.StorageURIs)
	initContainerArgs := make([]string, 0, numStorageURIs*2) // Each URI needs 2 args: URI and path
	mountContainerNames := make([]string, 0, 3)              // Containers that need volume mounts (userContainer, transformerContainer, initContainer)

	// Identify target containers for volume mounting
	// Priority: kserve-container > worker-container (fallback for some deployment modes)
	userContainer := utils.GetContainerWithName(params.PodSpec, constants.InferenceServiceContainerName)
	transformerContainer := utils.GetContainerWithName(params.PodSpec, constants.TransformerContainerName)
	workerContainer := utils.GetContainerWithName(params.PodSpec, constants.WorkerContainerName)

	if userContainer == nil {
		if workerContainer == nil {
			return errors.New("Invalid configuration: cannot find container")
		}
		// Use worker container as fallback for certain deployment scenarios
		userContainer = workerContainer
	}

	// Handle multiple storage URIs (new functionality)
	// This has to be enabled via the feature flag for Knative deployments
	if len(params.StorageURIs) > 0 && !params.IsLegacyURI {
		// Validate that the storage container supports multiple downloads
		if params.StorageContainerSpec != nil && !*params.StorageContainerSpec.SupportsMultiModelDownload {
			return fmt.Errorf("storage container %q does not support multi-model download; use the default kserve storage initializer or a compatible storage container",
				params.StorageContainerSpec.Container.Name)
		}

		nonPVCMountPaths := make([]string, 0, numStorageURIs)              // Paths for URIs that require init container download
		pvcStorageURIs := make([]v1beta1.StorageUri, 0, numStorageURIs)    // URIs that point to existing PVCs
		nonPVCStorageURIs := make([]v1beta1.StorageUri, 0, numStorageURIs) // URIs that require downloading (S3, GCS, etc.)

		// Separate PVC URIs from other storage URIs since they have different handling:
		// - PVC URIs are mounted directly as volumes (no download needed)
		// - Other URIs require init container to download artifacts first
		for _, storageUri := range params.StorageURIs {
			if strings.HasPrefix(storageUri.Uri, constants.PvcURIPrefix) {
				pvcStorageURIs = append(pvcStorageURIs, storageUri)
			} else {
				nonPVCStorageURIs = append(nonPVCStorageURIs, storageUri)
				nonPVCMountPaths = append(nonPVCMountPaths, storageUri.MountPath)
			}
		}

		mountContainerNames = append(mountContainerNames, userContainer.Name)
		if transformerContainer != nil {
			mountContainerNames = append(mountContainerNames, transformerContainer.Name)
		}

		// Mount PVC storage URIs directly as volumes (no init container needed)
		for _, storageURI := range pvcStorageURIs {
			for _, containerName := range mountContainerNames {
				pvcName, _, err := utils.ParsePvcURI(storageURI.Uri)
				if err != nil {
					return fmt.Errorf("failed to parse PVC URI %q: %w", storageURI.Uri, err)
				}

				storageMountParams := utils.StorageMountParams{
					MountPath:  storageURI.MountPath,
					SubPath:    "",
					VolumeName: utils.GetVolumeNameFromPath(storageURI.MountPath),
					PVCName:    pvcName,
					ReadOnly:   params.IsReadOnly,
				}

				if mountErr := utils.AddModelMount(storageMountParams, containerName, params.PodSpec); mountErr != nil {
					return fmt.Errorf("failed to add PVC mount for container %q: %w", containerName, mountErr)
				}
			}
		}

		if len(nonPVCStorageURIs) > 0 {
			// Find common parent path for non-PVC storage URIs
			nonPVCMountPath := utils.FindCommonParentPath(nonPVCMountPaths)

			// Build init container arguments: alternating pairs of source URI and target path
			for _, storageUri := range nonPVCStorageURIs {
				initContainerArgs = append(initContainerArgs, storageUri.Uri, storageUri.MountPath)
			}

			initContainer = utils.CreateInitContainerWithConfig(params.Config, initContainerArgs)

			// Append the init container to the pod spec
			params.PodSpec.InitContainers = append(params.PodSpec.InitContainers, *initContainer)
			// Get a pointer to the actual container in the slice for subsequent modifications
			initContainer = &params.PodSpec.InitContainers[len(params.PodSpec.InitContainers)-1]
			mountContainerNames = append(mountContainerNames, initContainer.Name)

			// Create shared volume mount for non-PVC storage URIs
			storageMountParams := utils.StorageMountParams{
				MountPath:  nonPVCMountPath,
				SubPath:    "",
				VolumeName: utils.GetVolumeNameFromPath(nonPVCMountPath),
				PVCName:    "",
				ReadOnly:   params.IsReadOnly,
			}

			// Apply volume mount to all relevant containers
			for _, containerName := range mountContainerNames {
				if mountErr := utils.AddModelMount(storageMountParams, containerName, params.PodSpec); mountErr != nil {
					return fmt.Errorf("failed to add volume mount for container %q: %w", containerName, mountErr)
				}
			}
		}
	} else if len(params.StorageURIs) == 1 {
		// Handle single storage URI (backward compatibility)
		storageURI := params.StorageURIs[0]

		storageMountParams := utils.StorageMountParams{
			MountPath:  constants.DefaultModelLocalMountPath,
			SubPath:    "",
			VolumeName: constants.StorageInitializerVolumeName,
			PVCName:    "",
			ReadOnly:   params.IsReadOnly,
		}

		mountContainerNames = append(mountContainerNames, userContainer.Name)
		if transformerContainer != nil {
			mountContainerNames = append(mountContainerNames, transformerContainer.Name)
		}

		// For PVC source URIs we need to mount the source to be able to access it
		// See design and discussion here: https://github.com/kserve/kserve/issues/148
		if strings.HasPrefix(storageURI.Uri, constants.PvcURIPrefix) {
			// Add a corresponding pvc volume mount to the userContainer and transformerContainer.
			// Pvc will be mount to /mnt/models rather than /mnt/pvc.
			// PvcPath will be injected via SubPath, pvcPath must be a root or Dir.
			// It is user responsibility to ensure it is a root or Dir
			pvcName, pvcPath, err := utils.ParsePvcURI(storageURI.Uri)
			if err != nil {
				return err
			}

			storageMountParams.SubPath = pvcPath
			storageMountParams.PVCName = pvcName
			storageMountParams.VolumeName = constants.PvcSourceMountName
		} else {
			initContainerArgs = append(initContainerArgs, storageURI.Uri, storageURI.MountPath)
			initContainer = utils.CreateInitContainerWithConfig(params.Config, initContainerArgs)

			// Append the init container to the pod spec
			params.PodSpec.InitContainers = append(params.PodSpec.InitContainers, *initContainer)
			// Get a pointer to the actual container in the slice for subsequent modifications
			initContainer = &params.PodSpec.InitContainers[len(params.PodSpec.InitContainers)-1]

			mountContainerNames = append(mountContainerNames, initContainer.Name)
		}

		for _, containerName := range mountContainerNames {
			mountErr := utils.AddModelMount(storageMountParams, containerName, params.PodSpec)
			if mountErr != nil {
				return mountErr
			}
		}

		// Set or update the CustomSpecStorageUri env variable value
		// Use the provided storageURIEnvVar value instead of always defaulting to DefaultModelLocalMountPath
		for index, envVar := range userContainer.Env {
			if envVar.Name == constants.CustomSpecStorageUriEnvVarKey && envVar.Value != "" {
				userContainer.Env[index].Value = constants.DefaultModelLocalMountPath
			}
		}
	}

	// Inject credentials only if we have an init container (not for PVC only sources)
	if initContainer != nil {
		if params.StorageSpec != nil && params.StorageSpec.StorageKey != nil && params.StorageSpec.Parameters != nil {
			// initContainer.Args (storageURI.URI) is modified up in CreateStorageSpecSecretEnvs
			if err := params.CredentialBuilder.CreateStorageSpecSecretEnvs(
				ctx,
				params.Namespace,
				params.IsvcAnnotations,
				*params.StorageSpec.StorageKey,
				*params.StorageSpec.Parameters,
				initContainer,
			); err != nil {
				return err
			}
		} else {
			// Inject service account credentials if storage spec doesn't exist
			err := params.CredentialBuilder.CreateSecretVolumeAndEnv(
				ctx,
				params.Namespace,
				params.IsvcAnnotations,
				params.PodSpec.ServiceAccountName,
				initContainer,
				&params.PodSpec.Volumes,
			)
			if err != nil {
				return err
			}
		}

		// Inject CA bundle configMap if caBundleConfigMapName or constants.DefaultGlobalCaBundleConfigMapName annotation is set
		caBundleConfigMapName := params.Config.CaBundleConfigMapName
		if ok := needCaBundleMount(caBundleConfigMapName, initContainer); ok {
			if params.Namespace != constants.KServeNamespace {
				caBundleConfigMapName = constants.DefaultGlobalCaBundleConfigMapName
			}

			caBundleVolumeMountPath := params.Config.CaBundleVolumeMountPath
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

			params.PodSpec.Volumes = append(params.PodSpec.Volumes, caBundleVolume)
			initContainer.VolumeMounts = append(initContainer.VolumeMounts, caBundleVolumeMount)
		}

		// Merge any customizations from the storage container spec into the init container
		if params.StorageContainerSpec != nil {
			err := mergeContainerSpecs(initContainer, &params.StorageContainerSpec.Container)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// InjectStorageInitializer injects an init container to provision model data
// for the serving container in a unified way across storage tech by injecting
// a provisioning INIT container. This is a workaround because Knative does not
// support INIT containers: https://github.com/knative/serving/issues/4307
// This method handles old single storage URI scenario for backward compatibility;
// Knative supports init containers so storage initializer is added as init container directly
// in the controller for new multiple storage URI scenarios.
func (mi *StorageInitializerInjector) InjectStorageInitializer(ctx context.Context, pod *corev1.Pod) error {
	// Only inject if the required annotations are set
	srcURI, ok := pod.ObjectMeta.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Mount pvc directly if local model label exists
	// Not supported with multiple storage URIs
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

	hasStorageSpec := pod.ObjectMeta.Annotations[constants.StorageSpecAnnotationKey]
	var storageSpec v1beta1.StorageSpec = v1beta1.StorageSpec{}

	if hasStorageSpec == "true" {
		var overrideParams map[string]string
		if storageSpecParam, ok := pod.ObjectMeta.Annotations[constants.StorageSpecParamAnnotationKey]; ok {
			if err := json.Unmarshal([]byte(storageSpecParam), &overrideParams); err != nil {
				return err
			}
		}
		storageKey := pod.ObjectMeta.Annotations[constants.StorageSpecKeyAnnotationKey]

		storageSpec.StorageKey = &storageKey
		storageSpec.Parameters = &overrideParams
	}

	isvcReadonlyStringFlag := GetStorageInitializerReadOnlyFlag(pod.ObjectMeta.Annotations)

	storageURIs := []v1beta1.StorageUri{{Uri: srcURI, MountPath: constants.DefaultModelLocalMountPath}}

	// Get storage container spec for the URI
	storageContainerSpec, err := GetStorageContainerSpec(ctx, srcURI, mi.client)
	if err != nil {
		return err
	}

	storageInitializerParams := &StorageInitializerParams{
		Namespace:            pod.Namespace,
		StorageURIs:          storageURIs,
		IsReadOnly:           isvcReadonlyStringFlag,
		PodSpec:              &pod.Spec,
		CredentialBuilder:    mi.credentialBuilder,
		Client:               mi.client,
		Config:               mi.config,
		IsvcAnnotations:      pod.ObjectMeta.Annotations,
		StorageSpec:          &storageSpec,
		StorageContainerSpec: storageContainerSpec,
		IsLegacyURI:          true,
	}

	return CommonStorageInitialization(ctx, storageInitializerParams)
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
		var istioStatusDecoded any
		if err := json.Unmarshal([]byte(istioStatus), &istioStatusDecoded); err != nil {
			return err
		}

		// Get the Istio sidecar container name.
		istioSidecarContainerName := ""
		istioStatusMap := istioStatusDecoded.(map[string]any)
		if istioContainers, istioContainersOk := istioStatusMap["containers"].([]any); istioContainersOk {
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

	// Handle environment variables separately to avoid conflicts between Value and ValueFrom fields
	// Strategic merge patch can cause both Value and ValueFrom to be set, which is invalid
	baseEnvVars := targetContainer.Env
	overrideEnvVars := crdContainer.Env

	// Create a temporary container without Env for merging other fields
	tempTarget := *targetContainer
	tempTarget.Env = nil
	tempOverride := *crdContainer
	tempOverride.Env = nil

	// Perform strategic merge on everything except environment variables
	defaultContainerJson, err := json.Marshal(tempTarget)
	if err != nil {
		return err
	}

	overrides, err := json.Marshal(tempOverride)
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

	// Manually merge environment variables with proper conflict resolution
	mergedEnvVars := mergeEnvironmentVariables(baseEnvVars, overrideEnvVars)
	targetContainer.Env = mergedEnvVars

	if targetContainer.Name == "" {
		targetContainer.Name = containerName
	}

	return nil
}

// mergeEnvironmentVariables merges two slices of environment variables.
// Override variables take precedence over base variables when they have the same name.
// This prevents conflicts between Value and ValueFrom fields.
func mergeEnvironmentVariables(baseEnvVars, overrideEnvVars []corev1.EnvVar) []corev1.EnvVar {
	envMap := make(map[string]corev1.EnvVar)

	// First, add all base environment variables
	for _, env := range baseEnvVars {
		envMap[env.Name] = env
	}

	// Then, override with any variables from the override list
	// This ensures that override variables completely replace base variables
	for _, env := range overrideEnvVars {
		envMap[env.Name] = env
	}

	// Convert back to slice
	var result []corev1.EnvVar
	for _, env := range envMap {
		result = append(result, env)
	}

	return result
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
