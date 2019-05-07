package custom

import (
	"fmt"

	knserving "github.com/knative/serving/pkg/apis/serving"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type CustomFramework struct {
	Spec *v1alpha1.CustomSpec
}

func (c *CustomFramework) CreateModelServingContainer(modelName string) *v1.Container {
	return &c.Spec.Container
}

func (c *CustomFramework) ValidateSpec() error {
	knativeErrs := knserving.ValidateContainer(c.Spec.Container, sets.String{})
	if knativeErrs != nil {
		return fmt.Errorf("Custom: " + knativeErrs.Error())
	}
	return nil
}
