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
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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
        if the mode is "RawDeployment" or "Serverless", return it.
		else return config.deploy.defaultDeploymentMode
*/
func GetDeploymentMode(annotations map[string]string, deployConfig *v1beta1api.DeployConfig) constants.DeploymentModeType {
	deploymentMode, ok := annotations[constants.DeploymentMode]
	if ok && (deploymentMode == string(constants.RawDeployment) || deploymentMode == string(constants.Serverless)) {
		return constants.DeploymentModeType(deploymentMode)
	}
	return constants.DeploymentModeType(deployConfig.DefaultDeploymentMode)
}
