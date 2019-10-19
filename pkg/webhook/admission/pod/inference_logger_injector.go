package pod

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
)

const (
	InferenceLoggerContainerName         = "inference-inference-inferencelogger"
	InferenceLoggerConfigMapKeyName      = "inferenceLogger"
	InferenceLoggerContainerImage        = "gcr.io/kfserving/inference-inference-inferencelogger"
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
	logUrl, ok := pod.ObjectMeta.Annotations[constants.InferenceLoggerSinkUrlInternalAnnotationKey]
	if !ok {
		return nil
	}

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
		},
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *loggerContainer)

	return nil
}
