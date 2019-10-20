package pod

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
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
	_, ok := pod.ObjectMeta.Annotations[constants.InferenceLoggerInternalAnnotationKey]
	if !ok {
		return nil
	}

	logUrl, ok := pod.ObjectMeta.Annotations[constants.InferenceLoggerSinkUrlInternalAnnotationKey]
	if !ok {
		logUrl = constants.GetInferenceLoggerDefaultUrl(pod.Namespace)
	}

	logType, ok := pod.ObjectMeta.Annotations[constants.InferenceLoggerLoggingTypeInternalAnnotationKey]
	if !ok {
		logType = string(v1alpha2.InferenceLogBoth)
	}

	sample, ok := pod.ObjectMeta.Annotations[constants.InferenceLoggerSampleInternalAnnotationKey]
	if !ok {
		sample = "1.0"
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
			"--source_uri",
			pod.Name,
			"--log_type",
			logType,
			"--sample",
			sample,
		},
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *loggerContainer)

	return nil
}
