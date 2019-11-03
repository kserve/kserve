package pod

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
)

const (
	LoggerContainerName         = "inferenceservice-logger"
	LoggerConfigMapKeyName      = "logger"
	LoggerContainerImage        = "gcr.io/kfserving/logger"
	LoggerContainerImageVersion = "latest"
	PodKnativeServiceLabel      = "serving.knative.dev/service"
)

type LoggerConfig struct {
	Image string `json:"image"`
}

type LoggerInjector struct {
	config *LoggerConfig
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

	logType, ok := pod.ObjectMeta.Annotations[constants.LoggerModeInternalAnnotationKey]
	if !ok {
		logType = string(v1alpha2.LogAll)
	}

	modelId, _ := pod.ObjectMeta.Labels[PodKnativeServiceLabel]

	// Dont inject if Contianer already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, LoggerContainerName) == 0 {
			return nil
		}
	}

	loggerImage := LoggerContainerImage + ":" + LoggerContainerImageVersion
	if il.config != nil && il.config.Image != "" {
		loggerImage = il.config.Image
	}

	loggerContainer := &v1.Container{
		Name:  LoggerContainerName,
		Image: loggerImage,
		Args: []string{
			"--log-url",
			logUrl,
			"--source-uri",
			pod.Name,
			"--log-mode",
			logType,
			"--model-id",
			modelId,
		},
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *loggerContainer)

	return nil
}
