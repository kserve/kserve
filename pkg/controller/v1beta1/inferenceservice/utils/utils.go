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

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"

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

					runtime, err := GetServingRuntime(ctx, client, *modelSpec.Runtime, isvc.Namespace)
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
GetDeploymentMode returns the current deployment mode, supports Serverless and RawDeployment
case 1: no serving.kserve.org/deploymentMode annotation

	return config.deploy.defaultDeploymentMode

case 2: serving.kserve.org/deploymentMode is set

	        if the mode is "RawDeployment", "Serverless" or "ModelMesh", return it.
			else return config.deploy.defaultDeploymentMode
*/
func GetDeploymentMode(statusDeploymentMode string, annotations map[string]string, deployConfig *v1beta1.DeployConfig) constants.DeploymentModeType {
	// First priority is the deploymentMode recorded in the status
	if len(statusDeploymentMode) != 0 {
		return constants.DeploymentModeType(statusDeploymentMode)
	}

	// Second priority, if the status doesn't have the deploymentMode recorded, is explicit annotations
	deploymentMode, ok := annotations[constants.DeploymentMode]
	if ok && (deploymentMode == string(constants.RawDeployment) || deploymentMode ==
		string(constants.Serverless) || deploymentMode == string(constants.ModelMeshDeployment)) {
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
func GetServingRuntime(ctx context.Context, cl client.Client, name string, namespace string) (*v1alpha1.ServingRuntimeSpec, error) {
	runtime := &v1alpha1.ServingRuntime{}
	err := cl.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, runtime)
	if err == nil {
		return &runtime.Spec, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	// ODH does not support ClusterServingRuntimes
	// clusterRuntime := &v1alpha1.ClusterServingRuntime{}
	// err = cl.Get(ctx, client.ObjectKey{Name: name}, clusterRuntime)
	// if err == nil {
	//	return &clusterRuntime.Spec, nil
	// } else if !apierrors.IsNotFound(err) {
	//	return nil, err
	//}
	return nil, goerrors.New("No ServingRuntimes with the name: " + name)
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

// Since the "cookie-secret" arg in the oauth proxy container is generated randomly,
// we exclude that arg while comparing existing and desired deployment specs
func RemoveCookieSecretArg(deployment appsv1.Deployment) *appsv1.Deployment {
	dep := deployment.DeepCopy()
	for i, container := range dep.Spec.Template.Spec.Containers {
		if container.Name == "oauth-proxy" {
			var newArgs []string
			for _, arg := range container.Args {
				if !strings.Contains(arg, "cookie-secret") {
					newArgs = append(newArgs, arg)
				}
			}
			dep.Spec.Template.Spec.Containers[i].Args = newArgs
		}
	}
	return dep
}

// Check for route created by odh-model-controller. If the route is found, use it as the isvc URL
func GetRouteURLIfExists(ctx context.Context, cli client.Client, metadata metav1.ObjectMeta, isvcName string) (*apis.URL, error) {
	foundRoute := false
	routeReady := false
	route := &routev1.Route{}
	err := cli.Get(ctx, types.NamespacedName{Name: isvcName, Namespace: metadata.Namespace}, route)
	if err != nil {
		return nil, err
	}

	// Check if the route is owned by the InferenceService
	for _, ownerRef := range route.OwnerReferences {
		if ownerRef.UID == metadata.UID {
			foundRoute = true
		}
	}

	// Check if the route is admitted
	for _, ingress := range route.Status.Ingress {
		for _, condition := range ingress.Conditions {
			if condition.Type == "Admitted" && condition.Status == "True" {
				routeReady = true
			}
		}
	}

	if !(foundRoute && routeReady) {
		return nil, fmt.Errorf("route %s/%s not found or not ready", metadata.Namespace, metadata.Name)
	}

	// Construct the URL
	host := route.Spec.Host
	scheme := "http"
	if route.Spec.TLS != nil && route.Spec.TLS.Termination != "" {
		scheme = "https"
	}

	// Create the URL as an apis.URL object
	routeURL := &apis.URL{
		Scheme: scheme,
		Host:   host,
	}

	return routeURL, nil
}

func FilterList(slice []string, element string) []string {
	var result []string
	for _, item := range slice {
		if item != element {
			result = append(result, item)
		}
	}
	return result
}
