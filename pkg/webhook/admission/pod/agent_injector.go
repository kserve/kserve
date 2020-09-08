/*
Copyright 2020 kubeflow.org.

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
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
)

const (
	AgentContainerName       = "agent"
	AgentConfigMapKeyName    = "agent"
	AgentS3EndpointArgName   = "-s3-endpoint"
)

type AgentConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

type AgentInjector struct {
	config *AgentConfig
}

func getAgentConfigs(configMap *v1.ConfigMap) (*AgentConfig, error) {

	agentConfig := &AgentConfig{}
	if agentConfigValue, ok := configMap.Data[AgentConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(agentConfigValue), &agentConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall agent json string due to %v ", err))
		}
	}

	//Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{agentConfig.MemoryRequest,
		agentConfig.MemoryLimit,
		agentConfig.CpuRequest,
		agentConfig.CpuLimit}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return agentConfig, fmt.Errorf("Failed to parse resource configuration for %q: %q",
				BatcherConfigMapKeyName, err.Error())
		}
	}

	//constants.KServiceComponentLabel
	return agentConfig, nil
}

func (il *AgentInjector) InjectAgent(pod *v1.Pod) error {
	// Only inject if the required annotations are set
	_, ok := pod.ObjectMeta.Annotations[constants.AgentInternalAnnotationKey]
	if !ok {
		return nil
	}

	// Don't inject if Container already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, AgentContainerName) == 0 {
			return nil
		}
	}

	var args []string
	s3Endpoint, ok := pod.ObjectMeta.Annotations[constants.AgentS3endpointAnnotationKey]
	if ok {
		args = append(args, AgentS3EndpointArgName)
		args = append(args, s3Endpoint)
	}

	// Make sure securityContext is initialized and valid
	securityContext := pod.Spec.Containers[0].SecurityContext.DeepCopy()

	agentContainer := &v1.Container{
		Name:  AgentContainerName,
		Image: il.config.Image,
		Args:  args,
		Resources: v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(il.config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(il.config.MemoryLimit),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(il.config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(il.config.MemoryRequest),
			},
		},
		SecurityContext: securityContext,
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *agentContainer)

	if _, ok := pod.ObjectMeta.Annotations[constants.AgentInternalAnnotationKey]; ok {
		// Mount the modelconfig volume to the pod and model agent container
		return mountModelConfigMap(pod)
	}

	return nil
}

func mountModelConfigMap(pod *v1.Pod) error {
	if modelConfigName, ok := pod.ObjectMeta.Annotations[constants.AgentModelConfigAnnotationKey]; ok {
		modelConfigVolume := v1.Volume{
			Name: constants.ModelConfigVolumeName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: modelConfigName,
					},
				},
			},
		}
		mountVolume(AgentContainerName, pod, modelConfigVolume)
		return nil
	}
	return fmt.Errorf("can not find %v label", constants.AgentModelConfigAnnotationKey)
}

func mountVolume(containerName string, pod *v1.Pod, additionalVolume v1.Volume) {
	pod.Spec.Volumes = appendVolume(pod.Spec.Volumes, additionalVolume)
	var mountedContainers []v1.Container
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			if container.VolumeMounts == nil {
				container.VolumeMounts = []v1.VolumeMount{}
			}
			container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
				Name:      constants.ModelConfigVolumeName,
				ReadOnly:  true,
				MountPath: constants.ModelConfigDir,
			})
		}
		mountedContainers = append(mountedContainers, container)
	}
	pod.Spec.Containers = mountedContainers
}

func appendVolume(existingVolumes []v1.Volume, additionalVolume v1.Volume) []v1.Volume {
	if existingVolumes == nil {
		existingVolumes = []v1.Volume{}
	}
	for _, volume := range existingVolumes {
		if volume.Name == additionalVolume.Name {
			return existingVolumes
		}
	}
	updatedVolumes := append(existingVolumes, additionalVolume)
	return updatedVolumes
}
