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

	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"
	goerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants
var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://", "https://", "http://", "hdfs://", "webhdfs://", "oci://"}
)

const (
	AzureBlobURL      = "blob.core.windows.net"
	AzureBlobURIRegEx = "https://(.+?).blob.core.windows.net/(.+)"
)

// IsMMSPredictor Only enable MMS predictor when predictor config sets MMS to true and neither
// storage uri nor storage spec is set
func IsMMSPredictor(predictor *v1beta1api.PredictorSpec) bool {
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

func IsMemoryResourceAvailable(isvc *v1beta1api.InferenceService, totalReqMemory resource.Quantity) bool {
	if isvc.Spec.Predictor.GetExtensions() == nil || len(isvc.Spec.Predictor.GetImplementations()) == 0 {
		return false
	}

	container := isvc.Spec.Predictor.GetImplementation().GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), nil)

	predictorMemoryLimit := container.Resources.Limits.Memory()
	return predictorMemoryLimit.Cmp(totalReqMemory) >= 0
}

// GetModelName returns the model name for single model serving case
func GetModelName(isvc *v1beta1api.InferenceService) string {
	modelName := isvc.Name
	// Return model name from args for KServe custom model server if transformer presents
	if isvc.Spec.Transformer != nil && len(isvc.Spec.Transformer.Containers) > 0 {
		for _, arg := range isvc.Spec.Transformer.Containers[0].Args {
			if strings.HasPrefix(arg, constants.ArgumentModelName) {
				modelNameValueArr := strings.Split(arg, "=")
				if len(modelNameValueArr) == 2 {
					return modelNameValueArr[1]
				}
			}
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
		for _, arg := range isvc.Spec.Predictor.Model.Args {
			if strings.HasPrefix(arg, constants.ArgumentModelName) {
				modelNameValueArr := strings.Split(arg, "=")
				if len(modelNameValueArr) == 2 {
					return modelNameValueArr[1]
				}
			}
		}
	} else if len(isvc.Spec.Predictor.Containers) > 0 {
		// Return model name from args for KServe custom model server
		for _, arg := range isvc.Spec.Predictor.Containers[0].Args {
			if strings.HasPrefix(arg, constants.ArgumentModelName) {
				modelNameValueArr := strings.Split(arg, "=")
				if len(modelNameValueArr) == 2 {
					return modelNameValueArr[1]
				}
			}
		}
	}
	return modelName
}

// GetPredictorEndpoint returns the predictor endpoint if status.address.url is not nil else returns empty string with error.
func GetPredictorEndpoint(isvc *v1beta1.InferenceService) (string, error) {
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
			protocol := isvc.Spec.Predictor.GetImplementation().GetProtocol()
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
func GetDeploymentMode(annotations map[string]string, deployConfig *v1beta1api.DeployConfig) constants.DeploymentModeType {
	deploymentMode, ok := annotations[constants.DeploymentMode]
	if ok && (deploymentMode == string(constants.RawDeployment) || deploymentMode ==
		string(constants.Serverless) || deploymentMode == string(constants.ModelMeshDeployment)) {
		return constants.DeploymentModeType(deploymentMode)
	}
	return constants.DeploymentModeType(deployConfig.DefaultDeploymentMode)
}

// MergeRuntimeContainers Merge the predictor Container struct with the runtime Container struct, allowing users
// to override runtime container settings from the predictor spec.
func MergeRuntimeContainers(runtimeContainer *v1.Container, predictorContainer *v1.Container) (*v1.Container, error) {
	// Save runtime container name, as the name can be overridden as empty string during the Unmarshal below
	// since the Name field does not have the 'omitempty' struct tag.
	runtimeContainerName := runtimeContainer.Name

	// Use JSON Marshal/Unmarshal to merge Container structs using strategic merge patch
	runtimeContainerJson, err := json.Marshal(runtimeContainer)
	if err != nil {
		return nil, err
	}

	overrides, err := json.Marshal(predictorContainer)
	if err != nil {
		return nil, err
	}

	mergedContainer := v1.Container{}
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
	mergedContainer.Args = append(append([]string{}, runtimeContainer.Args...), predictorContainer.Args...)

	return &mergedContainer, nil
}

// MergePodSpec Merge the predictor PodSpec struct with the runtime PodSpec struct, allowing users
// to override runtime PodSpec settings from the predictor spec.
func MergePodSpec(runtimePodSpec *v1alpha1.ServingRuntimePodSpec, predictorPodSpec *v1beta1.PodSpec) (*v1.PodSpec, error) {
	runtimePodSpecJson, err := json.Marshal(v1.PodSpec{
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

	corePodSpec := v1.PodSpec{}
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
func GetServingRuntime(cl client.Client, name string, namespace string) (*v1alpha1.ServingRuntimeSpec, error) {
	runtime := &v1alpha1.ServingRuntime{}
	err := cl.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, runtime)
	if err == nil {
		return &runtime.Spec, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}

	clusterRuntime := &v1alpha1.ClusterServingRuntime{}
	err = cl.Get(context.TODO(), client.ObjectKey{Name: name}, clusterRuntime)
	if err == nil {
		return &clusterRuntime.Spec, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}
	return nil, goerrors.New("No ServingRuntimes or ClusterServingRuntimes with the name: " + name)
}

// ReplacePlaceholders Replace placeholders in runtime container by values from inferenceservice metadata
func ReplacePlaceholders(container *v1.Container, meta metav1.ObjectMeta) error {
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
func UpdateImageTag(container *v1.Container, runtimeVersion *string, servingRuntime *string) {
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
			// For TFServing/TorchServe the GPU image is tagged with suffix "-gpu", when the version is found in the tag
			// and runtimeVersion is not specified, we default to append the "-gpu" suffix to the image tag
			if servingRuntime != nil && (*servingRuntime == constants.TFServing || *servingRuntime == constants.TorchServe) {
				// check for the case when image field is specified directly with gpu tag
				if !strings.HasSuffix(container.Image, "-gpu") {
					container.Image = image + "-gpu"
				}
			}
		}
	}
}

// ListPodsByLabel Get a PodList by label.
func ListPodsByLabel(cl client.Client, namespace string, labelKey string, labelVal string) (*v1.PodList, error) {
	podList := &v1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{labelKey: labelVal},
	}
	err := cl.List(context.TODO(), podList, opts...)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	sortPodsByCreatedTimestampDesc(podList)
	return podList, nil
}

func sortPodsByCreatedTimestampDesc(pods *v1.PodList) {
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[j].ObjectMeta.CreationTimestamp.Before(&pods.Items[i].ObjectMeta.CreationTimestamp)
	})
}

func ValidateStorageURI(storageURI *string, client client.Client) error {
	if storageURI == nil {
		return nil
	}

	// Step 1: Passes the validation if we have a storage container CR that supports this storageURI.
	storageContainerSpec, err := pod.GetContainerSpecForStorageUri(*storageURI, client)
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
