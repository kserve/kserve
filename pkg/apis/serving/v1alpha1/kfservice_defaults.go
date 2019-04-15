package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

// DefaultTensorflowVersion runs if not provided in TensorflowSpec
const (
	DefaultTensorflowVersion  = "1.12"
	DefaultXGBoostVersion     = "1.12"
	DefaultScikitLearnVersion = "1.12"
)

// DefaultModelServerResourceRequirements sets initial limits on serving pods
var DefaultModelServerResourceRequirements = v1.ResourceRequirements{
	Requests: v1.ResourceList{
		"cpu": resource.MustParse("1"),
		"mem": resource.MustParse("2Gi"),
	},
	Limits: v1.ResourceList{
		"cpu": resource.MustParse("1"),
		"mem": resource.MustParse("2Gi"),
	},
}

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter
func (kfsvc *KFService) Default() {
	setModelSpecDefaults(&kfsvc.Spec.Default)
	if kfsvc.Spec.Canary != nil {
		setModelSpecDefaults(&kfsvc.Spec.Canary.ModelSpec)
	}
}

func setModelSpecDefaults(modelSpec *ModelSpec) {
	if modelSpec.Tensorflow != nil {
		setTensorflowDefaults(modelSpec.Tensorflow)
	}
	if modelSpec.XGBoost != nil {
		setXGBoostDefaults(modelSpec.XGBoost)
	}
	if modelSpec.ScikitLearn != nil {
		setScikitLearnDefaults(modelSpec.ScikitLearn)
	}
	// todo(ellis-bigelow) Custom container
}

func setTensorflowDefaults(tensorflowSpec *TensorflowSpec) {
	if tensorflowSpec.RuntimeVersion == "" {
		tensorflowSpec.RuntimeVersion = DefaultTensorflowVersion
	}
	if equality.Semantic.DeepEqual(tensorflowSpec.Resources, v1.ResourceRequirements{}) {
		tensorflowSpec.Resources = DefaultModelServerResourceRequirements
	}
}

func setXGBoostDefaults(xgBoostSpec *XGBoostSpec) {
	if xgBoostSpec.RuntimeVersion == "" {
		xgBoostSpec.RuntimeVersion = DefaultXGBoostVersion
	}
	if equality.Semantic.DeepEqual(xgBoostSpec.Resources, v1.ResourceRequirements{}) {
		xgBoostSpec.Resources = DefaultModelServerResourceRequirements
	}
}

func setScikitLearnDefaults(scikitLearnSpec *ScikitLearnSpec) {
	if scikitLearnSpec.RuntimeVersion == "" {
		scikitLearnSpec.RuntimeVersion = DefaultScikitLearnVersion
	}
	if equality.Semantic.DeepEqual(scikitLearnSpec.Resources, v1.ResourceRequirements{}) {
		scikitLearnSpec.Resources = DefaultModelServerResourceRequirements
	}
}
