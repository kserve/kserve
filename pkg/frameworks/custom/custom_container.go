package custom

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func CreateCustomContainer(customSpec *v1alpha1.CustomSpec) *v1.Container {

	return &customSpec.Container

}
