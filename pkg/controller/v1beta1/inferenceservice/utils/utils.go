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
	"html/template"
	"regexp"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	goerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsMMSPredictor Only enable MMS predictor when predictor config sets MMS to true and neither
// storage uri nor storage spec is set
func IsMMSPredictor(predictor *v1beta1api.PredictorSpec, isvcConfig *v1beta1api.InferenceServicesConfig) bool {
	return predictor.GetImplementation().IsMMS(isvcConfig) &&
		predictor.GetImplementation().GetStorageUri() == nil && predictor.GetImplementation().GetStorageSpec() == nil
}

func IsMemoryResourceAvailable(isvc *v1beta1api.InferenceService, totalReqMemory resource.Quantity, isvcConfig *v1beta1api.InferenceServicesConfig) bool {
	if isvc.Spec.Predictor.GetExtensions() == nil || len(isvc.Spec.Predictor.GetImplementations()) == 0 {
		return false
	}

	container := isvc.Spec.Predictor.GetImplementation().GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), isvcConfig)

	predictorMemoryLimit := container.Resources.Limits.Memory()
	return predictorMemoryLimit.Cmp(totalReqMemory) >= 0
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

// MergeRuntimeContainers Merge the predictor Container struct into a runtime Container struct, allowing users
// to override runtime container settings from the predictor spec.
func MergeRuntimeContainers(runtimeContainer *v1.Container, predictorContainer *v1.Container) error {
	// Save runtime container name, as the name can be overridden as empty string during the Unmarshal below
	// since the Name field does not have the 'omitempty' struct tag.
	runtimeContainerName := runtimeContainer.Name
	// Args and Env will be combined instead of overridden
	newArgs := make([]string, len(runtimeContainer.Args), len(runtimeContainer.Args)+len(predictorContainer.Args))
	copy(newArgs, runtimeContainer.Args)
	newArgs = append(newArgs, predictorContainer.Args...)
	newEnv := mergeEnvs(runtimeContainer.Env, predictorContainer.Env)

	// Use JSON Marshal/Unmarshal to merge Container structs.
	overrides, err := json.Marshal(predictorContainer)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(overrides, runtimeContainer); err != nil {
		return err
	}

	if runtimeContainer.Name == "" {
		runtimeContainer.Name = runtimeContainerName
	}

	runtimeContainer.Args = newArgs
	runtimeContainer.Env = newEnv

	return nil
}

// mergeRuntimeVolumes Merge the predictor Volumes list with the runtime Volumes list, allowing users
// to override/augment runtime volume settings from the predictor spec.
func mergeRuntimeVolumes(runtimeVolumes []v1.Volume, predictorVolumes []v1.Volume) []v1.Volume {
	if len(runtimeVolumes) == 0 {
		return predictorVolumes
	} else if len(predictorVolumes) == 0 {
		return runtimeVolumes
	}
	vols := append(([]v1.Volume)(nil), runtimeVolumes...)
outer:
	for ip, _ := range predictorVolumes {
		for ir, _ := range vols {
			if predictorVolumes[ip].Name == vols[ir].Name {
				vols[ir] = predictorVolumes[ip]
				continue outer
			}
		}
		vols = append(vols, predictorVolumes[ip])
	}
	return vols
}

// mergeEnvs Merge the predictor Env list with the runtime Env list, allowing users
// to override/augment environment variables from the predictor spec.
func mergeEnvs(runtimeEnv []v1.EnvVar, predictorEnv []v1.EnvVar) []v1.EnvVar {
	if len(runtimeEnv) == 0 {
		return predictorEnv
	} else if len(predictorEnv) == 0 {
		return runtimeEnv
	}
	env := append(([]v1.EnvVar)(nil), runtimeEnv...)
outer:
	for ip, _ := range predictorEnv {
		for ir, _ := range env {
			if predictorEnv[ip].Name == env[ir].Name {
				env[ir] = predictorEnv[ip]
				continue outer
			}
		}
		env = append(env, predictorEnv[ip])
	}
	return env
}

// MergePodSpec Merge the predictor PodSpec struct with the runtime PodSpec struct, allowing users
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

	corePodSpec.Volumes = mergeRuntimeVolumes(runtimePodSpec.Volumes, predictorPodSpec.Volumes)

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
func UpdateImageTag(container *v1.Container, runtimeVersion *string, isvcConfig *v1beta1.InferenceServicesConfig) {
	image := container.Image
	if runtimeVersion != nil {
		re := regexp.MustCompile(`(:([\w.\-_]*))$`)
		if len(re.FindString(image)) == 0 {
			container.Image = image + ":" + *runtimeVersion
		} else {
			container.Image = re.ReplaceAllString(image, ":"+*runtimeVersion)
		}
		return
	}
	if utils.IsGPUEnabled(container.Resources) && len(strings.Split(image, ":")) > 0 {
		imageName := strings.Split(image, ":")[0]
		if imageName == isvcConfig.Predictors.Tensorflow.ContainerImage {
			container.Image = imageName + ":" + isvcConfig.Predictors.Tensorflow.DefaultGpuImageVersion
		} else if imageName == isvcConfig.Predictors.PyTorch.ContainerImage {
			container.Image = imageName + ":" + isvcConfig.Predictors.PyTorch.DefaultGpuImageVersion
		}
	}
}
