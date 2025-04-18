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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
)

const (
	LoggerConfigMapKeyName            = "logger"
	LoggerArgumentLogUrl              = "--log-url"
	LoggerArgumentSourceUri           = "--source-uri"
	LoggerArgumentMode                = "--log-mode"
	LoggerArgumentInferenceService    = "--inference-service"
	LoggerArgumentNamespace           = "--namespace"
	LoggerArgumentEndpoint            = "--endpoint"
	LoggerArgumentComponent           = "--component"
	LoggerArgumentCaCertFile          = "--logger-ca-cert-file"
	LoggerArgumentTlsSkipVerify       = "--logger-tls-skip-verify"
	LoggerArgumentMetadataHeaders     = "--metadata-headers"
	LoggerArgumentMetadataAnnotations = "--metadata-annotations"
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
	CaBundle      string `json:"caBundle"`
	CaCertFile    string `json:"caCertFile"`
	TlsSkipVerify bool   `json:"tlsSkipVerify"`
}

type AgentInjector struct {
	credentialBuilder *credentials.CredentialBuilder
	agentConfig       *AgentConfig
	loggerConfig      *LoggerConfig
	batcherConfig     *BatcherConfig
}

// TODO agent config
func getAgentConfigs(configMap *corev1.ConfigMap) (*AgentConfig, error) {
	agentConfig := &AgentConfig{}
	if agentConfigValue, ok := configMap.Data[constants.AgentConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(agentConfigValue), &agentConfig)
		if err != nil {
			panic(fmt.Errorf("unable to unmarshall agent json string due to %w", err))
		}
	}

	// Ensure that we set proper values
	resourceDefaults := []string{
		agentConfig.MemoryRequest,
		agentConfig.MemoryLimit,
		agentConfig.CpuRequest,
		agentConfig.CpuLimit,
	}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return agentConfig, fmt.Errorf("failed to parse resource configuration for %q: %s",
				constants.AgentConfigMapKeyName, err.Error())
		}
	}

	return agentConfig, nil
}

func getLoggerConfigs(configMap *corev1.ConfigMap) (*LoggerConfig, error) {
	loggerConfig := &LoggerConfig{}
	if loggerConfigValue, ok := configMap.Data[LoggerConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(loggerConfigValue), &loggerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall logger json string due to %w ", err))
		}
	}

	// Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{
		loggerConfig.MemoryRequest,
		loggerConfig.MemoryLimit,
		loggerConfig.CpuRequest,
		loggerConfig.CpuLimit,
	}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return loggerConfig, fmt.Errorf("Failed to parse resource configuration for %q: %q", LoggerConfigMapKeyName, err.Error())
		}
	}
	return loggerConfig, nil
}

func (ag *AgentInjector) InjectAgent(pod *corev1.Pod) error {
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

		inferenceServiceName := pod.ObjectMeta.Labels[constants.InferenceServiceLabel]
		namespace := pod.ObjectMeta.Namespace
		endpoint := pod.ObjectMeta.Labels[constants.KServiceEndpointLabel]
		component := pod.ObjectMeta.Labels[constants.KServiceComponentLabel]

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
			LoggerArgumentComponent,
			component,
		}
		logHeaderMetadata, ok := pod.ObjectMeta.Annotations[constants.LoggerMetadataHeadersInternalAnnotationKey]
		if ok {
			loggerArgs = append(loggerArgs, LoggerArgumentMetadataHeaders)
			loggerArgs = append(loggerArgs, logHeaderMetadata)
		}
		logMetadataAnnotations, ok := pod.ObjectMeta.Annotations[constants.LoggerMetadataAnnotationsInternalAnnotationKey]
		if ok {
			annotationKeys := strings.Split(logMetadataAnnotations, ",")
			kvPairs := []string{}
			for _, metadataAnnotation := range annotationKeys {
				val, exists := pod.ObjectMeta.Annotations[metadataAnnotation]
				if exists {
					kvPairs = append(kvPairs, fmt.Sprintf("%s=%s", metadataAnnotation, val))
				} else {
					klog.Warningf("failed to find matching annotation %s on inference service", metadataAnnotation)
				}
			}
			loggerArgs = append(loggerArgs, LoggerArgumentMetadataAnnotations, strings.Join(kvPairs, ","))
		}
		args = append(args, loggerArgs...)

		// Add TLS cert name if specified. If not specified it will fall back to the arg's default.
		if ag.loggerConfig.CaCertFile != "" {
			args = append(args, LoggerArgumentCaCertFile, ag.loggerConfig.CaCertFile)
		}
		// Whether to skip TLS verification. If not present in the ConfigMap, this will default to `false`
		args = append(args, LoggerArgumentTlsSkipVerify, strconv.FormatBool(ag.loggerConfig.TlsSkipVerify))
	}

	var queueProxyEnvs []corev1.EnvVar
	var agentEnvs []corev1.EnvVar
	queueProxyAvailable := false
	transformerContainerIdx := -1
	componentPort := constants.InferenceServiceDefaultHttpPort
	for idx, container := range pod.Spec.Containers {
		if container.Name == "queue-proxy" {
			agentEnvs = make([]corev1.EnvVar, 0, len(container.Env))
			agentEnvs = append(agentEnvs, container.Env...)
			queueProxyEnvs = container.Env
			queueProxyAvailable = true
		}

		if container.Name == constants.TransformerContainerName {
			transformerContainerIdx = idx
		}

		if container.Name == constants.InferenceServiceContainerName {
			if len(container.Ports) > 0 {
				componentPort = strconv.Itoa(int(container.Ports[0].ContainerPort))
			}
		}
	}
	// If the transformer container is present, use its port as the component port
	if transformerContainerIdx != -1 {
		transContainer := pod.Spec.Containers[transformerContainerIdx]
		if len(transContainer.Ports) == 0 {
			componentPort = constants.InferenceServiceDefaultHttpPort
		} else {
			componentPort = strconv.Itoa(int(transContainer.Ports[0].ContainerPort))
		}
	}
	args = append(args, constants.AgentComponentPortArgName, componentPort)

	if !queueProxyAvailable {
		readinessProbe := pod.Spec.Containers[0].ReadinessProbe
		// If the transformer container is present, use its readiness probe
		if transformerContainerIdx != -1 {
			readinessProbe = pod.Spec.Containers[transformerContainerIdx].ReadinessProbe
		}

		// Check if the readiness probe exists
		if readinessProbe != nil {
			if readinessProbe.HTTPGet != nil || readinessProbe.TCPSocket != nil {
				// Marshal the readiness probe into JSON format
				readinessProbeJson, err := json.Marshal(readinessProbe)
				if err != nil {
					klog.Errorf("Failed to marshal readiness probe for pod %s/%s: %v", pod.Namespace, pod.Name, err)
					return fmt.Errorf("failed to marshal readiness probe: %w", err)
				}

				// Log successful addition of readiness probe
				klog.Infof("Readiness probe marshaled and added as environment variable for pod %s/%s", pod.Namespace, pod.Name)

				// Append the marshaled readiness probe as an environment variable for the agent container
				agentEnvs = append(agentEnvs, corev1.EnvVar{Name: "SERVING_READINESS_PROBE", Value: string(readinessProbeJson)})
			} else if readinessProbe.Exec != nil {
				// Log the skipping of ExecAction readiness probes
				klog.Infof("Exec readiness probe skipped for pod %s/%s", pod.Namespace, pod.Name)
			}
		}
	} else {
		// Adjust USER_PORT when queueProxy is available
		for i, envVar := range queueProxyEnvs {
			if envVar.Name == "USER_PORT" {
				klog.Infof("Adjusting USER_PORT to %s for pod %s/%s", constants.InferenceServiceDefaultAgentPortStr, pod.Namespace, pod.Name)
				envVar.Value = constants.InferenceServiceDefaultAgentPortStr
				queueProxyEnvs[i] = envVar // Update the environment variable in the list
			}
		}
	}

	// Make sure securityContext is initialized and valid
	securityContext := pod.Spec.Containers[0].SecurityContext.DeepCopy()

	agentContainer := &corev1.Container{
		Name:  constants.AgentContainerName,
		Image: ag.agentConfig.Image,
		Args:  args,
		Resources: corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(ag.agentConfig.CpuLimit),
				corev1.ResourceMemory: resource.MustParse(ag.agentConfig.MemoryLimit),
			},
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(ag.agentConfig.CpuRequest),
				corev1.ResourceMemory: resource.MustParse(ag.agentConfig.MemoryRequest),
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "agent-port",
				ContainerPort: constants.InferenceServiceDefaultAgentPort,
				Protocol:      "TCP",
			},
		},
		SecurityContext: securityContext,
		Env:             agentEnvs,
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					HTTPHeaders: []corev1.HTTPHeader{
						{
							Name:  "K-Network-Probe",
							Value: "queue",
						},
					},
					Port:   intstr.FromInt(constants.InferenceServiceDefaultAgentPort),
					Path:   "/",
					Scheme: "HTTP",
				},
			},
		},
	}

	// If the Logger TLS bundle ConfigMap is specified, mount it
	if injectLogger && ag.loggerConfig.CaBundle != "" {
		// Optional. If the ConfigMap is not found, this will not make the Pod fail
		optionalVolume := true
		configMapVolume := corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ag.loggerConfig.CaBundle,
				},
				Optional: &optionalVolume,
			},
		}

		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name:         constants.LoggerCaBundleVolume,
			VolumeSource: configMapVolume,
		})

		agentContainer.VolumeMounts = append(agentContainer.VolumeMounts, corev1.VolumeMount{
			Name:      constants.LoggerCaBundleVolume,
			MountPath: constants.LoggerCaCertMountPath,
			ReadOnly:  true,
		})
	}

	// Inject credentials
	if err := ag.credentialBuilder.CreateSecretVolumeAndEnv(
		pod.Namespace,
		pod.Annotations,
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

func mountModelDir(pod *corev1.Pod) error {
	if _, ok := pod.ObjectMeta.Annotations[constants.AgentModelDirAnnotationKey]; ok {
		modelDirVolume := corev1.Volume{
			Name: constants.ModelDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		// Mount the model dir into agent container
		mountVolumeToContainer(constants.AgentContainerName, pod, modelDirVolume, constants.ModelDir)
		// Mount the model dir into model server container
		mountVolumeToContainer(constants.InferenceServiceContainerName, pod, modelDirVolume, constants.ModelDir)
		return nil
	}
	return fmt.Errorf("can not find %v label", constants.AgentModelConfigVolumeNameAnnotationKey)
}

func mountModelConfig(pod *corev1.Pod) error {
	if modelConfigName, ok := pod.ObjectMeta.Annotations[constants.AgentModelConfigVolumeNameAnnotationKey]; ok {
		modelConfigVolume := corev1.Volume{
			Name: constants.ModelConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
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

func mountVolumeToContainer(containerName string, pod *corev1.Pod, additionalVolume corev1.Volume, mountPath string) {
	pod.Spec.Volumes = appendVolume(pod.Spec.Volumes, additionalVolume)
	mountedContainers := make([]corev1.Container, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			if container.VolumeMounts == nil {
				container.VolumeMounts = []corev1.VolumeMount{}
			}
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      additionalVolume.Name,
				ReadOnly:  false,
				MountPath: mountPath,
			})
		}
		mountedContainers = append(mountedContainers, container)
	}
	pod.Spec.Containers = mountedContainers
}

func appendVolume(existingVolumes []corev1.Volume, additionalVolume corev1.Volume) []corev1.Volume {
	if existingVolumes == nil {
		existingVolumes = []corev1.Volume{}
	}
	for _, volume := range existingVolumes {
		if volume.Name == additionalVolume.Name {
			return existingVolumes
		}
	}
	existingVolumes = append(existingVolumes, additionalVolume)
	return existingVolumes
}
