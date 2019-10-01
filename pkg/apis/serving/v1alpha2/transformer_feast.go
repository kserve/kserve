package v1alpha2

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var _ Transformer = (*FeastTransformerSpec)(nil)

// Constants
const (
	DefaultFeastImage  = "gcr.io/kfserving/feasttransformer"
	ArgumentFeastURL   = "--feast-url"
	ArgumentDataType   = "--data-type"
	ArgumentEntityIds  = "--entity-ids"
	ArgumentFeatureIds = "--feature-ids"
)

// Errors
var (
	AllowedFeastDataTypes     = strings.Join([]string{string(NDArray), string(TensorProto)}, ", ")
	InvalidFeastURLError      = "FeastURL must be of the form http(s)://hostname:port."
	InvalidFeastDataTypeError = fmt.Sprintf("FeastDataType must be one of %v.", AllowedFeastDataTypes)
	InvalidEntityIdsError     = "EntityIds cannot be empty."
	InvalidFeatureIdsError    = "FeatureIds cannot be empty."
)

// FeastDataType to expect from the response.
type FeastDataType string

// FeastDataType enum
const (
	NDArray     FeastDataType = "NDArray"
	TensorProto FeastDataType = "TensorProto"
)

// FeastTransformerSpec is EXPERIMENTAL! Use at your own risk.
type FeastTransformerSpec struct {
	FeastURL   string        `json:"feastUrl,omitempty"`
	DataType   FeastDataType `json:"dataType,omitempty"`
	EntityIds  []string      `json:"entityIds,omitempty"`
	FeatureIds []string      `json:"featureIds,omitempty"`
}

// GetTransformerContainer for the FeastTransformerSpec
func (f FeastTransformerSpec) GetTransformerContainer() *v1.Container {
	args := []string{}
	args = append(args, ArgumentFeastURL)
	args = append(args, f.FeastURL)
	args = append(args, ArgumentDataType)
	args = append(args, f.DataType.ToCLIArgument())
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

// Validate the FeastTransformerSpec.
func (f *FeastTransformerSpec) Validate() error {
	for assertion, err := range map[bool]string{
		len(f.EntityIds) != 0:                              InvalidEntityIdsError,
		len(f.FeatureIds) != 0:                             InvalidFeatureIdsError,
		f.DataType == NDArray || f.DataType == TensorProto: InvalidFeastDataTypeError,
		f.hasValidFeastURL():                               InvalidFeastURLError,
	} {
		if !assertion {
			return fmt.Errorf(err)
		}
	}
	return nil
}

func (f *FeastTransformerSpec) hasValidFeastURL() bool {
	_, err := url.ParseRequestURI(f.FeastURL)
	return err == nil
}

// ToCLIArgument translates FeastDataType to CLI Arguments
func (f FeastDataType) ToCLIArgument() string {
	return strings.ToLower(string(f))
}
