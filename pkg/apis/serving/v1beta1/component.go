package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Components interface is implemented by predictor, transformer and explainer
// +kubebuilder:object:generate=false
type Component interface {
	GetContainer(metadata metav1.ObjectMeta, containerConcurrency *int64, config *InferenceServicesConfig) *v1.Container
	GetStorageUri() *string
	Default(config *InferenceServicesConfig)
	Validate() error
}
