package v1alpha2

import (
	"fmt"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// Constants
const (
	ExactlyOneTransformerViolatedError = "Exactly one of [Custom, Feast] must be specified in TransformerSpec"
)

// +k8s:openapi-gen=false
type TransformerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion string `json:"defaultImageVersion"`

	AllowedImageVersions []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type TransformersConfig struct {
	Feast TransformerConfig `json:"feast,omitempty"`
}

// Transformer interface is implemented by all Transformers
type Transformer interface {
	GetContainerSpec() *v1.Container
	ApplyTransformerDefaults(config *TransformersConfig)
	ValidateTransformer(config *TransformersConfig) error
}

// GetContainerSpec for the transformer
func (t *TransformerSpec) GetContainerSpec(metadata metav1.ObjectMeta, isCanary bool) *v1.Container {
	transformer, err := getTransformer(t)
	if err != nil {
		return &v1.Container{}
	}
	container := transformer.GetContainerSpec().DeepCopy()
	container.Args = append(container.Args, []string{
		constants.ArgumentModelName,
		metadata.Name,
		constants.ArgumentPredictorHost,
		constants.PredictorURL(metadata, isCanary),
	}...)
	return container
}

// ApplyDefaults to the TransformerSpec
func (t *TransformerSpec) ApplyDefaults(config *TransformersConfig) {
	transformer, err := getTransformer(t)
	if err == nil {
		transformer.ApplyTransformerDefaults(config)
	}
}

// Validate the TransformerSpec
func (t *TransformerSpec) Validate(config *TransformersConfig) error {
	transformer, err := getTransformer(t)
	if err != nil {
		return err
	}
	return transformer.ValidateTransformer(config)
}

func getTransformer(t *TransformerSpec) (Transformer, error) {
	transformers := []Transformer{}
	if t.Custom != nil {
		transformers = append(transformers, t.Custom)
	}
	// Fail if not exactly one
	if len(transformers) != 1 {
		err := fmt.Errorf(ExactlyOneTransformerViolatedError)
		klog.Error(err)
		return nil, err
	}
	return transformers[0], nil
}
