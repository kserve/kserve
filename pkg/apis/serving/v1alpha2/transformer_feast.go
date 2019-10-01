package v1alpha2

import v1 "k8s.io/api/core/v1"

var _ Transformer = (*FeastTransformerSpec)(nil)

// Constants
const (
	DefaultFeastImage = "gcr.io/kfserving/feasttransformer"
)

// FeastTransformerSpec defines arguments for configuring a Transformer to call Feast
type FeastTransformerSpec struct {
	FeastURL   string   `json:"feastUrl"`
	EntityIds  []string `json:"entityIds"`
	FeatureIds []string `json:"featureIds"`
}

// GetContainerSpec for the FeastTransformerSpec
func (f FeastTransformerSpec) GetContainerSpec() *v1.Container {
	return &v1.Container{
		Image: DefaultFeastImage,
	}
}

// ApplyDefaults to the FeastTransformerSpec
func (f *FeastTransformerSpec) ApplyDefaults() {
}

// Validate the FeastTransformerSpec
func (f *FeastTransformerSpec) Validate() error {
	return nil
}
