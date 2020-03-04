package pod

import (
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
)

const (
	LoggerContainerName            = "inferenceservice-logger"
	LoggerConfigMapKeyName         = "logger"
	PodKnativeServiceLabel         = "serving.knative.dev/service"
	LoggerArgumentLogUrl           = "--log-url"
	LoggerArgumentSourceUri        = "--source-uri"
	LoggerArgumentMode             = "--log-mode"
	LoggerArgumentInferenceService = "--inference-service"
	LoggerArgumentNamespace        = "--namespace"
	LoggerArgumentEndpoint         = "--endpoint"
)

type LoggerConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

type LoggerInjector struct {
	config *LoggerConfig
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

func (il *LoggerInjector) InjectLogger(pod *v1.Pod) error {
	// Only inject if the required annotations are set
	_, ok := pod.ObjectMeta.Annotations[constants.LoggerInternalAnnotationKey]
	if !ok {
		return nil
	}

	logUrl, ok := pod.ObjectMeta.Annotations[constants.LoggerSinkUrlInternalAnnotationKey]
	if !ok {
		logUrl = constants.GetLoggerDefaultUrl(pod.Namespace)
	}

	logMode, ok := pod.ObjectMeta.Annotations[constants.LoggerModeInternalAnnotationKey]
	if !ok {
		logMode = string(v1alpha2.LogAll)
	}

	inferenceServiceName, _ := pod.ObjectMeta.Labels[constants.KServiceModelLabel]
	namespace := pod.ObjectMeta.Namespace
	endpoint := pod.ObjectMeta.Labels[constants.KServiceEndpointLabel]

	// Don't inject if Contianer already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, LoggerContainerName) == 0 {
			return nil
		}
	}

	// Make sure securityContext is initialized and valid
	securityContext := pod.Spec.Containers[0].SecurityContext.DeepCopy()

	loggerContainer := &v1.Container{
		Name:  LoggerContainerName,
		Image: il.config.Image,
		Args: []string{
			LoggerArgumentLogUrl,
			logUrl,
			LoggerArgumentSourceUri,
			pod.Name,
			LoggerArgumentMode,
			logMode,
			LoggerArgumentInferenceService,
			inferenceServiceName,
			LoggerArgumentNamespace,
			namespace,
			LoggerArgumentEndpoint,
			endpoint,
		},
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
	pod.Spec.Containers = append(pod.Spec.Containers, *loggerContainer)

	return nil
}
