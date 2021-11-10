/*

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
	"encoding/json"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Only enable MMS predictor when predictor config sets MMS to true and storage uri is not set
func IsMMSPredictor(predictor *v1beta1api.PredictorSpec, isvcConfig *v1beta1api.InferenceServicesConfig) bool {
	return predictor.GetImplementation().IsMMS(isvcConfig) && predictor.GetImplementation().GetStorageUri() == nil
}

// Container
func IsMemoryResourceAvailable(isvc *v1beta1api.InferenceService, totalReqMemory resource.Quantity, isvcConfig *v1beta1api.InferenceServicesConfig) bool {
	if isvc.Spec.Predictor.GetExtensions() == nil || len(isvc.Spec.Predictor.GetImplementations()) == 0 {
		return false
	}

	container := isvc.Spec.Predictor.GetImplementation().GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), isvcConfig)

	if constants.InferenceServiceContainerName == container.Name {
		predictorMemoryLimit := container.Resources.Limits.Memory()
		return predictorMemoryLimit.Cmp(totalReqMemory) >= 0
	}

	return false
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

// Merge the predictor Container struct with the runtime Container struct, allowing users
// to override runtime container settings from the predictor spec.
func MergeRuntimeContainers(runtimeContainer *v1alpha1.Container, predictorContainer *v1.Container) (*v1.Container, error) {
	// Default container configuration from the runtime.
	coreContainer := v1.Container{
		Args:            runtimeContainer.Args,
		Command:         runtimeContainer.Command,
		Env:             runtimeContainer.Env,
		Image:           runtimeContainer.Image,
		Name:            runtimeContainer.Name,
		Resources:       runtimeContainer.Resources,
		ImagePullPolicy: runtimeContainer.ImagePullPolicy,
		WorkingDir:      runtimeContainer.WorkingDir,
		LivenessProbe:   runtimeContainer.LivenessProbe,
	}
	// Save runtime container name, as the name can be overridden as empty string during the Unmarshal below
	// since the Name field does not have the 'omitempty' struct tag.
	runtimeContainerName := runtimeContainer.Name

	// Args and Env will be combined instead of overridden.
	argCopy := make([]string, len(coreContainer.Args))
	copy(argCopy, coreContainer.Args)

	envCopy := make([]v1.EnvVar, len(coreContainer.Env))
	copy(envCopy, coreContainer.Env)

	// Use JSON Marshal/Unmarshal to merge Container structs.
	overrides, err := json.Marshal(predictorContainer)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(overrides, &coreContainer); err != nil {
		return nil, err
	}

	if coreContainer.Name == "" {
		coreContainer.Name = runtimeContainerName
	}

	argCopy = append(argCopy, predictorContainer.Args...)
	envCopy = append(envCopy, predictorContainer.Env...)

	coreContainer.Args = argCopy
	coreContainer.Env = envCopy

	return &coreContainer, nil

}

// Merge the predictor PodSpec struct with the runtime PodSpec struct, allowing users
// to override runtime PodSpec settings from the predictor spec.
func MergePodSpec(runtimePodSpec *v1alpha1.ServingRuntimePodSpec, predictorPodSpec *v1beta1.PodSpec) (*v1.PodSpec, error) {

	corePodSpec := v1.PodSpec{
		NodeSelector: runtimePodSpec.NodeSelector,
		Affinity:     runtimePodSpec.Affinity,
		Tolerations:  runtimePodSpec.Tolerations,
	}

	// Use JSON Marshal/Unmarshal to merge PodSpec structs.
	overrides, err := json.Marshal(predictorPodSpec)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(overrides, &corePodSpec); err != nil {
		return nil, err
	}

	return &corePodSpec, nil

}
