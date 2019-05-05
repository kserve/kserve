package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

// Default Values
var (
	DefaultTensorflowVersion  = "1.13.0"
	DefaultXGBoostVersion     = "1.12"
	DefaultScikitLearnVersion = "1.12"

	DefaultMemoryRequests = resource.MustParse("2Gi")
	DefaultCPURequests    = resource.MustParse("1")
)

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter
func (kfsvc *KFService) Default() {
	logger.Info("Defaulting KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
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
}

func setTensorflowDefaults(tensorflowSpec *TensorflowSpec) {
	if tensorflowSpec.RuntimeVersion == "" {
		tensorflowSpec.RuntimeVersion = DefaultTensorflowVersion
	}
	setResourceRequirementDefaults(&tensorflowSpec.Resources)
}

func setXGBoostDefaults(xgBoostSpec *XGBoostSpec) {
	if xgBoostSpec.RuntimeVersion == "" {
		xgBoostSpec.RuntimeVersion = DefaultXGBoostVersion
	}
	setResourceRequirementDefaults(&xgBoostSpec.Resources)
}

func setScikitLearnDefaults(scikitLearnSpec *ScikitLearnSpec) {
	if scikitLearnSpec.RuntimeVersion == "" {
		scikitLearnSpec.RuntimeVersion = DefaultScikitLearnVersion
	}
	setResourceRequirementDefaults(&scikitLearnSpec.Resources)
}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPURequests
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemoryRequests
	}
}
