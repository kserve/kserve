package custom

import (
	"fmt"

	knserving "github.com/knative/serving/pkg/apis/serving"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type CustomSpec v1alpha1.CustomSpec

func (c *CustomSpec) CreateModelServingContainer(modelName string) *v1.Container {
	return &c.Container
}

func (c *CustomSpec) Validate() error {
	knativeErrs := knserving.ValidateContainer(c.Container, sets.String{})
	if knativeErrs != nil {
		return fmt.Errorf("Custom: " + knativeErrs.Error())
	}
	return nil
}
