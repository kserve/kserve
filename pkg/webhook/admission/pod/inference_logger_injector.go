package pod

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
)

const (
	InferenceLoggerContainerName         = "inferenceservice-logger"
	InferenceLoggerConfigMapKeyName      = "logger"
	InferenceLoggerContainerImage        = "gcr.io/kfserving/logger"
	InferenceLoggerContainerImageVersion = "latest"
)

type InferenceLoggerConfig struct {
	Image string `json:"image"`
}

type InferenceLoggerInjector struct {
	config *InferenceLoggerConfig
}

func (il *InferenceLoggerInjector) InjectInferenceLogger(pod *v1.Pod) error {
	// Only inject if the required annotations are set
	_, ok := pod.ObjectMeta.Annotations[constants.LoggerInternalAnnotationKey]
	if !ok {
		return nil
	}

	logUrl, ok := pod.ObjectMeta.Annotations[constants.LoggerSinkUrlInternalAnnotationKey]
	if !ok {
		logUrl = constants.GetInferenceLoggerDefaultUrl(pod.Namespace)
	}

	logType, ok := pod.ObjectMeta.Annotations[constants.LoggerTypeInternalAnnotationKey]
	if !ok {
		logType = string(v1alpha2.LogAll)
	}

	modelURI, _ := pod.ObjectMeta.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]

	// Dont inject if Contianer already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, InferenceLoggerContainerName) == 0 {
			return nil
		}
	}

	inferenceLoggerImage := InferenceLoggerContainerImage + ":" + InferenceLoggerContainerImageVersion
	if il.config != nil && il.config.Image != "" {
		inferenceLoggerImage = il.config.Image
	}

	loggerContainer := &v1.Container{
		Name:  InferenceLoggerContainerName,
		Image: inferenceLoggerImage,
		Args: []string{
			"--log_url",
			logUrl,
			"--source_uri",
			pod.Name,
			"--log_type",
			logType,
			"--model_uri",
			modelURI,
		},
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *loggerContainer)

	return nil
}
