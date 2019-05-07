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
	ValidateSpec() error
}

func Get(modelSpec *v1alpha1.ModelSpec) (interface{ FrameworkHandler }, error) {
	handlers := []interface{ FrameworkHandler }{}
	if modelSpec.Custom != nil {
		handlers = append(handlers, &custom.CustomFramework{Spec: modelSpec.Custom})
	}
	if modelSpec.XGBoost != nil {
		// TODO: add fwk for xgboost
		handlers = append(handlers, &custom.CustomFramework{Spec: modelSpec.Custom})
	}
	if modelSpec.ScikitLearn != nil {
		// TODO: add fwk for sklearn
		handlers = append(handlers, &custom.CustomFramework{Spec: modelSpec.Custom})
	}
	if modelSpec.Tensorflow != nil {
		handlers = append(handlers, &tensorflow.TensorflowFramework{Spec: modelSpec.Tensorflow})
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf(AtLeastOneModelSpecViolatedError)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneModelSpecViolatedError)
	}
	return handlers[0], nil
}
