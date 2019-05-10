package frameworks

import (
	"fmt"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/frameworks/custom"
	"github.com/kubeflow/kfserving/pkg/frameworks/tensorflow"
	v1 "k8s.io/api/core/v1"
)

type FrameworkHandler interface {
	CreateModelServingContainer(modelName string) *v1.Container
	Validate() error
}

const (
	// ExactlyOneModelSpecViolatedError is a known error message
	ExactlyOneModelSpecViolatedError = "Exactly one of [Custom, Tensorflow, ScikitLearn, XGBoost] must be specified in ModelSpec"
	// AtLeastOneModelSpecViolatedError is a known error message
	AtLeastOneModelSpecViolatedError = "At least one of [Custom, Tensorflow, ScikitLearn, XGBoost] must be specified in ModelSpec"
)

func MakeHandler(modelSpec *v1alpha1.ModelSpec) (interface{ FrameworkHandler }, error) {
	handlers := []interface{ FrameworkHandler }{}
	if modelSpec.Custom != nil {
		customSpec := custom.CustomSpec(*modelSpec.Custom)
		handlers = append(handlers, &customSpec)
	}
	if modelSpec.XGBoost != nil {
		// TODO: add fwk for xgboost
	}
	if modelSpec.ScikitLearn != nil {
		// TODO: add fwk for sklearn
	}
	if modelSpec.Tensorflow != nil {
		tfSpec := tensorflow.TensorflowSpec(*modelSpec.Tensorflow)
		handlers = append(handlers, &tfSpec)
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf(AtLeastOneModelSpecViolatedError)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneModelSpecViolatedError)
	}
	return handlers[0], nil
}
