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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/credentials"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
)

const (
	LoggerConfigMapKeyName         = "logger"
	LoggerArgumentLogUrl           = "--log-url"
	LoggerArgumentSourceUri        = "--source-uri"
	LoggerArgumentMode             = "--log-mode"
	LoggerArgumentInferenceService = "--inference-service"
	LoggerArgumentNamespace        = "--namespace"
	LoggerArgumentEndpoint         = "--endpoint"
)

type AgentConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

type LoggerConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	DefaultUrl    string `json:"defaultUrl"`
}

type AgentInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	agentConfig       *AgentConfig
	loggerConfig      *LoggerConfig
	batcherConfig     *BatcherConfig
}

func getAgentConfigs(configMap *v1.ConfigMap) (*AgentConfig, error) {

	agentConfig := &AgentConfig{}
	if agentConfigValue, ok := configMap.Data[constants.AgentConfigMapKeyName]; ok {
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
				constants.AgentConfigMapKeyName, err.Error())
		}
	}

	return agentConfig, nil
}

func getLoggerConfigs(configMap *v1.ConfigMap) (*LoggerConfig, error) {

	loggerConfig := &LoggerConfig{}
	if loggerConfigValue, ok := configMap.Data[LoggerConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(loggerConfigValue), &loggerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall logger json string due to %v ", err))
		}
	}

	//Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{loggerConfig.MemoryRequest,
		loggerConfig.MemoryLimit,
		loggerConfig.CpuRequest,
		loggerConfig.CpuLimit}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return loggerConfig, fmt.Errorf("Failed to parse resource configuration for %q: %q", LoggerConfigMapKeyName, err.Error())
		}
	}

	return loggerConfig, nil
}

func (ag *AgentInjector) InjectAgent(pod *v1.Pod) error {
	// Only inject the model agent sidecar if the required annotations are set
	_, injectLogger := pod.ObjectMeta.Annotations[constants.LoggerInternalAnnotationKey]
	_, injectPuller := pod.ObjectMeta.Annotations[constants.AgentShouldInjectAnnotationKey]
	_, injectBatcher := pod.ObjectMeta.Annotations[constants.BatcherInternalAnnotationKey]

	if !injectLogger && !injectPuller && !injectBatcher {
		return nil
	}

	// Don't inject if Container already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, constants.AgentContainerName) == 0 {
			return nil
		}
	}

	var args []string
	if injectPuller {
		args = append(args, constants.AgentEnableFlag)
		args = append(args, "true")
		modelConfig, ok := pod.ObjectMeta.Annotations[constants.AgentModelConfigMountPathAnnotationKey]
		if ok {
			args = append(args, constants.AgentConfigDirArgName)
			args = append(args, modelConfig)
		}

		modelDir, ok := pod.ObjectMeta.Annotations[constants.AgentModelDirAnnotationKey]
		if ok {
			args = append(args, constants.AgentModelDirArgName)
			args = append(args, modelDir)
		}
	}
	// Only inject if the batcher required annotations are set
	if injectBatcher {
		args = append(args, BatcherEnableFlag)
		args = append(args, "true")
		maxBatchSize, ok := pod.ObjectMeta.Annotations[constants.BatcherMaxBatchSizeInternalAnnotationKey]
		if ok {
			args = append(args, BatcherArgumentMaxBatchSize)
			args = append(args, maxBatchSize)
		}

		maxLatency, ok := pod.ObjectMeta.Annotations[constants.BatcherMaxLatencyInternalAnnotationKey]
		if ok {
			args = append(args, BatcherArgumentMaxLatency)
			args = append(args, maxLatency)
		}
	}
	// Only inject if the logger required annotations are set
	if injectLogger {
		logUrl, ok := pod.ObjectMeta.Annotations[constants.LoggerSinkUrlInternalAnnotationKey]
		if !ok {
			logUrl = ag.loggerConfig.DefaultUrl
		}

		logMode, ok := pod.ObjectMeta.Annotations[constants.LoggerModeInternalAnnotationKey]
		if !ok {
			logMode = string(v1beta1.LogAll)
		}

		inferenceServiceName, _ := pod.ObjectMeta.Labels[constants.InferenceServiceLabel]
		namespace := pod.ObjectMeta.Namespace
		endpoint := pod.ObjectMeta.Labels[constants.KServiceEndpointLabel]

		loggerArgs := []string{
			LoggerArgumentLogUrl,
			logUrl,
			LoggerArgumentSourceUri,
			pod.ObjectMeta.Name,
			LoggerArgumentMode,
			logMode,
			LoggerArgumentInferenceService,
			inferenceServiceName,
			LoggerArgumentNamespace,
			namespace,
			LoggerArgumentEndpoint,
			endpoint,
		}
		args = append(args, loggerArgs...)
	}

	queueProxyEnvs := []v1.EnvVar{}
	for _, container := range pod.Spec.Containers {
		if container.Name == "queue-proxy" {
			queueProxyEnvs = container.Env
		}
	}

	// Make sure securityContext is initialized and valid
	securityContext := pod.Spec.Containers[0].SecurityContext.DeepCopy()

	agentContainer := &v1.Container{
		Name:  constants.AgentContainerName,
		Image: ag.agentConfig.Image,
		Args:  args,
		Resources: v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(ag.agentConfig.CpuLimit),
				v1.ResourceMemory: resource.MustParse(ag.agentConfig.MemoryLimit),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(ag.agentConfig.CpuRequest),
				v1.ResourceMemory: resource.MustParse(ag.agentConfig.MemoryRequest),
			},
		},
		SecurityContext: securityContext,
		Env:             queueProxyEnvs,
	}
	readinessProbe := &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{
					"/agent",
					"-probe-period",
					"0",
				},
			},
		},
	}
	if injectLogger || injectBatcher {
		agentContainer.ReadinessProbe = readinessProbe
	}

	// Inject credentials
	if err := ag.credentialBuilder.CreateSecretVolumeAndEnv(
		pod.Namespace,
		pod.Spec.ServiceAccountName,
		agentContainer,
		&pod.Spec.Volumes,
	); err != nil {
		return err
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *agentContainer)

	if _, ok := pod.ObjectMeta.Annotations[constants.AgentShouldInjectAnnotationKey]; ok {
		// Mount the modelDir volume to the pod and model agent container
		err := mountModelDir(pod)
		if err != nil {
			return err
		}
		// Mount the modelConfig volume to the pod and model agent container
		err = mountModelConfig(pod)
		if err != nil {
			return err
		}
	}

	return nil
}

func mountModelDir(pod *v1.Pod) error {
	if _, ok := pod.ObjectMeta.Annotations[constants.AgentModelDirAnnotationKey]; ok {
		modelDirVolume := v1.Volume{
			Name: constants.ModelDirVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		}
		//Mount the model dir into agent container
		mountVolumeToContainer(constants.AgentContainerName, pod, modelDirVolume, constants.ModelDir)
		//Mount the model dir into model server container
		mountVolumeToContainer(constants.InferenceServiceContainerName, pod, modelDirVolume, constants.ModelDir)
		return nil
	}
	return fmt.Errorf("can not find %v label", constants.AgentModelConfigVolumeNameAnnotationKey)
}

func mountModelConfig(pod *v1.Pod) error {
	if modelConfigName, ok := pod.ObjectMeta.Annotations[constants.AgentModelConfigVolumeNameAnnotationKey]; ok {
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
		mountVolumeToContainer(constants.AgentContainerName, pod, modelConfigVolume, constants.ModelConfigDir)
		return nil
	}
	return fmt.Errorf("can not find %v label", constants.AgentModelConfigVolumeNameAnnotationKey)
}

func mountVolumeToContainer(containerName string, pod *v1.Pod, additionalVolume v1.Volume, mountPath string) {
	pod.Spec.Volumes = appendVolume(pod.Spec.Volumes, additionalVolume)
	var mountedContainers []v1.Container
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			if container.VolumeMounts == nil {
				container.VolumeMounts = []v1.VolumeMount{}
			}
			container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
				Name:      additionalVolume.Name,
				ReadOnly:  false,
				MountPath: mountPath,
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
