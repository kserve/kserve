package v1alpha2

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var _ Transformer = (*FeastTransformerSpec)(nil)

// Constants
const (
	DefaultFeastImage  = "gcr.io/kfserving/feasttransformer"
	ArgumentFeastURL   = "--feast-url"
	ArgumentEntityIds  = "--entity-ids"
	ArgumentFeatureIds = "--feature-ids"
)

// FeastTransformerSpec defines arguments for configuring a Transformer to call Feast
type FeastTransformerSpec struct {
	FeastURL   string   `json:"feastUrl"`
	EntityIds  []string `json:"entityIds"`
	FeatureIds []string `json:"featureIds"`
}

// GetTransformerContainer for the FeastTransformerSpec
func (f FeastTransformerSpec) GetTransformerContainer() *v1.Container {
	args := []string{}
	args = append(args, ArgumentFeastURL)
	args = append(args, f.FeastURL)
	args = append(args, ArgumentEntityIds)
	args = append(args, f.EntityIds...)
	args = append(args, ArgumentFeatureIds)
	args = append(args, f.FeatureIds...)

	return &v1.Container{
		Name:  constants.Transformer.String(),
		Image: DefaultFeastImage,
		Args:  args,
	}
}

// ApplyDefaults to the FeastTransformerSpec
func (f *FeastTransformerSpec) ApplyDefaults() {
}

// Validate the FeastTransformerSpec
func (f *FeastTransformerSpec) Validate() error {
	return nil
}
