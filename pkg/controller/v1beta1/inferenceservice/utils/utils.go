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

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	goerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"
)

// Constants
var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://", "https://", "http://", "hdfs://", "webhdfs://", "oci://", "hf://"}
)

const (
	AzureBlobURL      = "blob.core.windows.net"
	AzureBlobURIRegEx = "https://(.+?).blob.core.windows.net/(.+)"
)

// IsMMSPredictor Only enable MMS predictor when predictor config sets MMS to true and neither
// storage uri nor storage spec is set
func IsMMSPredictor(predictor *v1beta1.PredictorSpec) bool {
	if len(predictor.Containers) > 0 {
		container := predictor.Containers[0]
		for _, envVar := range container.Env {
			if envVar.Name == constants.CustomSpecMultiModelServerEnvVarKey && envVar.Value == "true" {
				return true
			}
		}
		return false
	} else {
		impl := predictor.GetImplementation()
		res := impl.GetStorageUri() == nil && impl.GetStorageSpec() == nil
		// HuggingFace supports model ID without storage initializer, but it should not be a multi-model server.
		if predictor.HuggingFace != nil || (predictor.Model != nil && predictor.Model.ModelFormat.Name == "huggingface") {
			return false
		}
		return res
	}
}

func IsMemoryResourceAvailable(isvc *v1beta1.InferenceService, totalReqMemory resource.Quantity) bool {
	if isvc.Spec.Predictor.GetExtensions() == nil || len(isvc.Spec.Predictor.GetImplementations()) == 0 {
		return false
	}

	container := isvc.Spec.Predictor.GetImplementation().GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), nil)

	predictorMemoryLimit := container.Resources.Limits.Memory()
	return predictorMemoryLimit.Cmp(totalReqMemory) >= 0
}

func getModelNameFromArgs(args []string) string {
	modelName := ""
	for i, arg := range args {
		if strings.HasPrefix(arg, constants.ArgumentModelName) {
			// Case 1: ["--model-name=<model-name>"]
			modelNameValueArr := strings.Split(arg, "=")
			if len(modelNameValueArr) == 2 {
				modelName = modelNameValueArr[1]
			}
			// Case 2: ["--model-name", "<model-name>"]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				modelName = args[i+1]
			}
		}
	}
	return modelName
}

// GetModelName returns the model name for single model serving case
func GetModelName(isvc *v1beta1.InferenceService) string {
	// Return model name from args for KServe custom model server if transformer presents
	if isvc.Spec.Transformer != nil && len(isvc.Spec.Transformer.Containers) > 0 {
		modelName := getModelNameFromArgs(isvc.Spec.Transformer.Containers[0].Args)
		if modelName != "" {
			return modelName
		}
	}
	if isvc.Spec.Predictor.Model != nil {
		// Return model name from env variable for MLServer
		for _, env := range isvc.Spec.Predictor.Model.Env {
			if env.Name == constants.MLServerModelNameEnv {
				return env.Value
			}
		}
		// Return model name from args for tfserving or KServe model server
		modelName := getModelNameFromArgs(isvc.Spec.Predictor.Model.Args)
		if modelName != "" {
			return modelName
		}
	} else if len(isvc.Spec.Predictor.Containers) > 0 {
		// Return model name from args for KServe custom model server
		modelName := getModelNameFromArgs(isvc.Spec.Predictor.Containers[0].Args)
		if modelName != "" {
			return modelName
		}
	}
	// Return isvc name if model name is not found
	return isvc.Name
}

// GetPredictorEndpoint returns the predictor endpoint if status.address.url is not nil else returns empty string with error.
func GetPredictorEndpoint(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService) (string, error) {
	if isvc.Status.Address != nil && isvc.Status.Address.URL != nil {
		hostName := isvc.Status.Address.URL.String()
		path := ""
		modelName := GetModelName(isvc)
		if isvc.Spec.Transformer != nil {
			protocol := isvc.Spec.Transformer.GetImplementation().GetProtocol()
			if protocol == constants.ProtocolV1 {
				path = constants.PredictPath(modelName, constants.ProtocolV1)
			} else if protocol == constants.ProtocolV2 {
				path = constants.PredictPath(modelName, constants.ProtocolV2)
			}
		} else if !IsMMSPredictor(&isvc.Spec.Predictor) {
			predictorImplementation := isvc.Spec.Predictor.GetImplementation()
			protocol := predictorImplementation.GetProtocol()

			if modelSpec, ok := predictorImplementation.(*v1beta1.ModelSpec); ok {
				if modelSpec.Runtime != nil {
					// When a Runtime is specified, and there is no protocol specified
					// in the ISVC, the protocol cannot imply to be V1. The protocol
					// needs to be extracted from the Runtime.

					runtime, err, _ := GetServingRuntime(ctx, client, *modelSpec.Runtime, isvc.Namespace)
					if err != nil {
						return "", err
					}

					// If the runtime has protocol versions, use the first one supported by IG.
					// Otherwise, assume Protocol V1.
					if len(runtime.ProtocolVersions) != 0 {
						found := false
						for _, pversion := range runtime.ProtocolVersions {
							if pversion == constants.ProtocolV1 || pversion == constants.ProtocolV2 {
								protocol = pversion
								found = true
								break
							}
						}

						if !found {
							return "", errors.New("the runtime does not support a protocol compatible with Inference Graphs")
						}
					}
				}

				// else {
				//   Notice that when using auto-selection (i.e. Runtime is nil), the
				//   ISVC is assumed to be protocol v1. Thus, for auto-select, a runtime
				//   will only match if it lists protocol v1 as supported. In this case,
				//   the code above (protocol := predictorImplementation.GetProtocol()) would
				//   already get the right protocol to configure in the InferenceGraph.
				// }
			}

			if protocol == constants.ProtocolV1 {
				path = constants.PredictPath(modelName, constants.ProtocolV1)
			} else if protocol == constants.ProtocolV2 {
				path = constants.PredictPath(modelName, constants.ProtocolV2)
			}
		}
		return fmt.Sprintf("%s%s", hostName, path), nil
	} else {
		return "", goerrors.Errorf("service %s is not ready", isvc.Name)
	}
}

/*
GetDeploymentMode returns the current deployment mode, supports Knative and Standard
case 1: no serving.kserve.org/deploymentMode annotation

	return config.deploy.defaultDeploymentMode

case 2: serving.kserve.org/deploymentMode is set

	        if the mode is "Standard", "Knative" or "ModelMesh", return it.
			else return config.deploy.defaultDeploymentMode
*/
func GetDeploymentMode(statusDeploymentMode string, annotations map[string]string, deployConfig *v1beta1.DeployConfig) constants.DeploymentModeType {
	// First priority is the deploymentMode recorded in the status
	if len(statusDeploymentMode) != 0 {
		return constants.DeploymentModeType(statusDeploymentMode)
	}

	// Second priority, if the status doesn't have the deploymentMode recorded, is explicit annotations
	deploymentMode, ok := annotations[constants.DeploymentMode]
	if deploymentMode == string(constants.LegacyRawDeployment) {
		// LegacyRawDeployment is deprecated, so we treat it as Standard
		deploymentMode = string(constants.Standard)
	}
	if deploymentMode == string(constants.LegacyServerless) {
		// LegacyServerless is deprecated, so we treat it as Knative
		deploymentMode = string(constants.Knative)
	}
	if ok && (deploymentMode == string(constants.Standard) ||
		deploymentMode == string(constants.Knative) ||
		deploymentMode == string(constants.ModelMeshDeployment)) {
		return constants.DeploymentModeType(deploymentMode)
	}

	// Finally, if an InferenceService is being created and does not explicitly specify a DeploymentMode
	return constants.DeploymentModeType(deployConfig.DefaultDeploymentMode)
}

// MergeRuntimeContainers Merge the predictor or transformer Container struct with the runtime Container struct, allowing users
// to override runtime container settings from the predictor spec.
func MergeRuntimeContainers(runtimeContainer *corev1.Container, isvcContainer *corev1.Container) (*corev1.Container, error) {
	// Save runtime container name, as the name can be overridden as empty string during the Unmarshal below
	// since the Name field does not have the 'omitempty' struct tag.
	runtimeContainerName := runtimeContainer.Name

	// Use JSON Marshal/Unmarshal to merge Container structs using strategic merge patch
	runtimeContainerJson, err := json.Marshal(runtimeContainer)
	if err != nil {
		return nil, err
	}

	overrides, err := json.Marshal(isvcContainer)
	if err != nil {
		return nil, err
	}

	mergedContainer := corev1.Container{}
	jsonResult, err := strategicpatch.StrategicMergePatch(runtimeContainerJson, overrides, mergedContainer)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonResult, &mergedContainer); err != nil {
		return nil, err
	}

	if mergedContainer.Name == "" {
		mergedContainer.Name = runtimeContainerName
	}

	// Strategic merge patch will replace args but more useful behaviour here is to concatenate
	mergedContainer.Args = append(append([]string{}, runtimeContainer.Args...), isvcContainer.Args...)

	return &mergedContainer, nil
}

// MergePodSpec Merge the predictor PodSpec struct with the runtime PodSpec struct, allowing users
// to override runtime PodSpec settings from the predictor spec.
func MergePodSpec(runtimePodSpec *v1alpha1.ServingRuntimePodSpec, predictorPodSpec *v1beta1.PodSpec) (*corev1.PodSpec, error) {
	runtimePodSpecJson, err := json.Marshal(corev1.PodSpec{
		NodeSelector:     runtimePodSpec.NodeSelector,
		Affinity:         runtimePodSpec.Affinity,
		Tolerations:      runtimePodSpec.Tolerations,
		Volumes:          runtimePodSpec.Volumes,
		ImagePullSecrets: runtimePodSpec.ImagePullSecrets,
	})
	if err != nil {
		return nil, err
	}

	// Use JSON Marshal/Unmarshal to merge PodSpec structs.
	overrides, err := json.Marshal(predictorPodSpec)
	if err != nil {
		return nil, err
	}

	corePodSpec := corev1.PodSpec{}
	jsonResult, err := strategicpatch.StrategicMergePatch(runtimePodSpecJson, overrides, corePodSpec)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonResult, &corePodSpec); err != nil {
		return nil, err
	}

	return &corePodSpec, nil
}

// GetServingRuntime Get a ServingRuntime by name. First, ServingRuntimes in the given namespace will be checked.
// If a resource of the specified name is not found, then ClusterServingRuntimes will be checked.
// Third value will be true if the ServingRuntime is a ClusterServingRuntime.
func GetServingRuntime(ctx context.Context, cl client.Client, name string, namespace string) (*v1alpha1.ServingRuntimeSpec, error, bool) {
	runtime := &v1alpha1.ServingRuntime{}
	err := cl.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, runtime)
	if err == nil {
		return &runtime.Spec, nil, false
	} else if !apierrors.IsNotFound(err) {
		return nil, err, false
	}

	clusterRuntime := &v1alpha1.ClusterServingRuntime{}
	err = cl.Get(ctx, client.ObjectKey{Name: name}, clusterRuntime)
	if err == nil {
		return &clusterRuntime.Spec, nil, true
	} else if !apierrors.IsNotFound(err) {
		return nil, err, false
	}
	return nil, goerrors.New("No ServingRuntimes or ClusterServingRuntimes with the name: " + name), false
}

// ReplacePlaceholders Replace placeholders in runtime container by values from inferenceservice metadata
func ReplacePlaceholders(container *corev1.Container, meta metav1.ObjectMeta) error {
	data, _ := json.Marshal(container)
	tmpl, err := template.New("container-tmpl").Parse(string(data))
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, meta)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), container)
}

// UpdateImageTag Update image tag if GPU is enabled or runtime version is provided
func UpdateImageTag(container *corev1.Container, runtimeVersion *string, servingRuntime *string) {
	image := container.Image

	// If image uses a digest (e.g. image@sha256:...), do not change it.
	if strings.Contains(image, "@sha256:") {
		return
	}

	if runtimeVersion != nil {
		re := regexp.MustCompile(`(:([\w.\-_]*))$`)
		if len(re.FindString(image)) == 0 {
			container.Image = image + ":" + *runtimeVersion
		} else {
			container.Image = re.ReplaceAllString(image, ":"+*runtimeVersion)
		}
	} else if utils.IsGPUEnabled(container.Resources) && len(strings.Split(image, ":")) > 0 {
		re := regexp.MustCompile(`(:([\w.\-_]*))$`)
		if len(re.FindString(image)) > 0 {
			// For TFServing/TorchServe/HuggingFace the GPU image is tagged with suffix "-gpu", when the version is found in the tag
			// and runtimeVersion is not specified, we default to append the "-gpu" suffix to the image tag
			if servingRuntime != nil && (*servingRuntime == constants.TFServing || *servingRuntime == constants.TorchServe || *servingRuntime == constants.HuggingFaceServer) {
				// check for the case when image field is specified directly with gpu tag
				if !strings.HasSuffix(container.Image, "-gpu") {
					container.Image = image + "-gpu"
				}
			}
		}
	}
}

// ListPodsByLabel Get a PodList by label.
func ListPodsByLabel(ctx context.Context, cl client.Client, namespace string, labelKey string, labelVal string) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{labelKey: labelVal},
	}
	err := cl.List(ctx, podList, opts...)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	sortPodsByCreatedTimestampDesc(podList)
	return podList, nil
}

func sortPodsByCreatedTimestampDesc(pods *corev1.PodList) {
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[j].ObjectMeta.CreationTimestamp.Before(&pods.Items[i].ObjectMeta.CreationTimestamp)
	})
}

func ValidateStorageURI(ctx context.Context, storageURI *string, client client.Client) error {
	if storageURI == nil {
		return nil
	}

	// Step 1: Passes the validation if we have a storage container CR that supports this storageURI.
	storageContainerSpec, err := pod.GetContainerSpecForStorageUri(ctx, *storageURI, client)
	if err != nil {
		return err
	}
	if storageContainerSpec != nil {
		return nil
	}

	// Step 2: Does the default storage initializer image support this storageURI?
	// local path (not some protocol?)
	if !regexp.MustCompile(`\w+?://`).MatchString(*storageURI) {
		return nil
	}

	// need to verify Azure Blob first, because it uses http(s):// prefix
	if strings.Contains(*storageURI, AzureBlobURL) {
		azureURIMatcher := regexp.MustCompile(AzureBlobURIRegEx)
		if parts := azureURIMatcher.FindStringSubmatch(*storageURI); parts != nil {
			return nil
		}
	} else if utils.IsPrefixSupported(*storageURI, SupportedStorageURIPrefixList) {
		return nil
	}

	return fmt.Errorf(v1beta1.UnsupportedStorageURIFormatError, strings.Join(SupportedStorageURIPrefixList, ", "), *storageURI)
}

// ValidateStorageUrisSpec validates that paths are absolute
func ValidateStorageUriSpec(storageUri *v1beta1.StorageUrisSpec) error {
    //Validate that paths have a common parent
    if storageUri.Uri == "" {
        return fmt.Errorf("storage URI cannot be empty")
    }

    if storageUri.Path == "/" {
        return fmt.Errorf("storage path cannot be empty")
    }

    if !strings.HasPrefix(storageUri.Path, "/") {
        return fmt.Errorf("storage path must be absolute: %s", storageUri.Path)
    }

    return nil
}

// ValidateStorageUrisSpec validates that paths are absolute
func ValidateStorageUrisSpec(storageUris []v1beta1.StorageUrisSpec) error {
	if len(storageUris) == 0 {
        return nil
    }

	// Validate each individual StorageUrisSpec
    for _, storageUri := range storageUris {
        if err := ValidateStorageUriSpec(&storageUri); err != nil {
            return err
        }
    }

	// Validate that paths have a common parent path
	paths := make([]string, len(storageUris))
    for i, storageUri := range storageUris {
        paths[i] = storageUri.Path
    }

	// If only one storage URI, no need to check common parent
    if len(paths) <= 1 {
        return nil
    }

	commonParent := FindCommonParentPath(paths)
    if commonParent == "/" {
        return fmt.Errorf("storage paths must have a common parent directory")
    }

	return nil
}

// findCommonParentPath finds the common parent directory of multiple paths
func FindCommonParentPath(paths []string) string {
    if len(paths) == 0 {
        return ""
    }

    if len(paths) == 1 {
        return paths[0]
    }

    // Split all paths into components
    pathComponents := make([][]string, len(paths))
	minLength := 0
    for i, path := range paths {
        // Clean the path and split by "/"
        cleanPath := strings.Trim(path, "/")
        if cleanPath == "" {
            pathComponents[i] = []string{}
        } else {
            pathComponents[i] = strings.Split(cleanPath, "/")
			minLength = min(minLength, len(pathComponents[i]))
        }
    }

    // Find common prefix
    var commonComponents []string
	for i := 0; i < minLength; i++ {
		levelComponents := make(map[string]struct{})
		var pathComponent string

		for _, components := range pathComponents {
			pathComponent = components[i]
			levelComponents[pathComponent] = struct{}{}
		}

		if len(levelComponents) == 1 {
			commonComponents = append(commonComponents, pathComponent)
		}
	}

    if len(commonComponents) == 0 {
        return "/"
    }

    return "/" + strings.Join(commonComponents, "/")
}


func prepareStorageResources(storageUris []v1beta1.StorageUrisSpec) ([]corev1.VolumeMount, []corev1.Volume, []string, error) {
    var initContainerArgs []string
    var volumeMounts []corev1.VolumeMount
    var volumes []corev1.Volume
	var mountPaths []string

	for _, storageUri := range storageUris {
            initContainerArgs = append(initContainerArgs, storageUri.Uri, storageUri.Path)
			mountPaths = append(mountPaths, storageUri.Path)
        }

	mountPath := FindCommonParentPath(mountPaths)

	volumeName := GetVolumeNameFromPath(mountPath)

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  false,
	})

	volumes = append(volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

    return volumeMounts, volumes, initContainerArgs, nil
}

func applyStorageTooPodSpec(podSpec *corev1.PodSpec, initContainer *corev1.Container,
    volumes []corev1.Volume, volumeMounts []corev1.VolumeMount) {

    podSpec.InitContainers = append(podSpec.InitContainers, *initContainer)
    podSpec.Volumes = append(podSpec.Volumes, volumes...)

    // Mount volumes to main containers as read-only
    for i := range podSpec.Containers {
        for _, volumeMount := range volumeMounts {
            podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts,
                corev1.VolumeMount{
                    Name:      volumeMount.Name,
                    MountPath: volumeMount.MountPath,
                    ReadOnly:  true,
                })
        }
    }
}

func SetupStorageInitialization(storageUrisSpec *[]v1beta1.StorageUrisSpec,
    podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec,
    storageConfig *types.StorageInitializerConfig) error {

    if storageUrisSpec == nil || len(*storageUrisSpec) == 0 {
        return nil
    }

    volumeMounts, volumes, args, err := prepareStorageResources(*storageUrisSpec)
    if err != nil {
        return err
    }

    initContainer := utils.CreateInitContainerWithConfig(args, volumeMounts, storageConfig)
    initContainer.VolumeMounts = append(initContainer.VolumeMounts, volumeMounts...)

    // Apply to main pod spec
    applyStorageTooPodSpec(podSpec, initContainer, volumes, volumeMounts)

    // Apply to worker pod spec if exists
    if workerPodSpec != nil {
        applyStorageTooPodSpec(workerPodSpec, initContainer, volumes, volumeMounts)
    }

    return nil
}

// Function to add a new environment variable to a specific container in the PodSpec
func AddEnvVarToPodSpec(podSpec *corev1.PodSpec, containerName, envName, envValue string) error {
	updatedResult := false
	// Iterate over the containers in the PodTemplateSpec to find the specified container
	for i, container := range podSpec.Containers {
		if container.Name == containerName {
			updatedResult = true
			if _, exists := utils.GetEnvVarValue(container.Env, envName); exists {
				// Overwrite the environment variable
				for j, envVar := range container.Env {
					if envVar.Name == envName {
						podSpec.Containers[i].Env[j].Value = envValue
						break
					}
				}
			} else {
				// Add the new environment variable to the Env field if it ooes not exist
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  envName,
					Value: envValue,
				})
				podSpec.Containers[i].Env = container.Env
			}
		}
	}

	if !updatedResult {
		return fmt.Errorf("target container(%s) does not exist", containerName)
	}
	return nil
}

func GetContainerIndexByName(containers []corev1.Container, containerName string) int {
	for i, container := range containers {
		if container.Name == containerName {
			return i
		}
	}
	return -1
}

func MergeServingRuntimeAndInferenceServiceSpecs(srContainers []corev1.Container, isvcContainer corev1.Container, isvc *v1beta1.InferenceService, targetContainerName string, srPodSpec v1alpha1.ServingRuntimePodSpec, isvcPodSpec v1beta1.PodSpec) (int, *corev1.Container, *corev1.PodSpec, error) {
	var err error
	containerIndexInSR := GetContainerIndexByName(srContainers, targetContainerName)
	if containerIndexInSR == -1 {
		errMsg := fmt.Sprintf("failed to find %s in ServingRuntime containers", targetContainerName)
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: errMsg,
		})
		return 0, nil, nil, errors.New(errMsg)
	}

	mergedContainer, err := MergeRuntimeContainers(&srContainers[containerIndexInSR], &isvcContainer)
	if err != nil {
		errMsg := fmt.Sprintf("failed to merge container. Detail: %s", err)
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: errMsg,
		})
		return 0, nil, nil, errors.New(errMsg)
	}

	mergedPodSpec, err := MergePodSpec(&srPodSpec, &isvcPodSpec)
	if err != nil {
		errMsg := fmt.Sprintf("failed to consolidate serving runtime PodSpecs. Detail: %s", err)
		isvc.Status.UpdateModelTransitionStatus(v1beta1.InvalidSpec, &v1beta1.FailureInfo{
			Reason:  v1beta1.InvalidPredictorSpec,
			Message: errMsg,
		})
		return 0, nil, nil, errors.New(errMsg)
	}
	return containerIndexInSR, mergedContainer, mergedPodSpec, nil
}

// Helper function to generate volume name from path
func GetVolumeNameFromPath(path string) string {
    // Convert path to valid volume name (remove slashes, etc.)
    return strings.ReplaceAll(strings.Trim(path, "/"), "/", "-")
}
