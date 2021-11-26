/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"reflect"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	// logger for the mutating webhook.
	mutatorLogger = logf.Log.WithName("inferenceservice-v1beta1-mutating-webhook")
)

// +kubebuilder:webhook:path=/mutate-inferenceservices,mutating=true,failurePolicy=fail,groups=serving.kserve.io,resources=inferenceservices,verbs=create;update,versions=v1beta1,name=inferenceservice.kserve-webhook-server.defaulter
var _ webhook.Defaulter = &InferenceService{}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}
	for k, v := range defaultResource {
		if _, ok := requirements.Requests[k]; !ok {
			requirements.Requests[k] = v
		}
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}
	for k, v := range defaultResource {
		if _, ok := requirements.Limits[k]; !ok {
			requirements.Limits[k] = v
		}
	}
}

func (isvc *InferenceService) Default() {
	mutatorLogger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "isvc", isvc.Spec.Predictor)
	cli, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		panic("Failed to create client in defaulter")
	}
	configMap, err := NewInferenceServicesConfig(cli)
	if err != nil {
		panic(err)
	}
	isvc.DefaultInferenceService(configMap)
}

func (isvc *InferenceService) DefaultInferenceService(config *InferenceServicesConfig) {
	for _, component := range []Component{
		&isvc.Spec.Predictor,
		isvc.Spec.Transformer,
		isvc.Spec.Explainer,
	} {
		if !reflect.ValueOf(component).IsNil() {
			if err := validateExactlyOneImplementation(component); err != nil {
				mutatorLogger.Error(ExactlyOneErrorFor(component), "Missing component implementation")
			} else {
				component.GetImplementation().Default(config)
				component.GetExtensions().Default(config)
			}
		}
	}
	isvc.setPredictorModelDefaults(config)
}

func (isvc *InferenceService) setPredictorModelDefaults(config *InferenceServicesConfig) {
	if isvc.Spec.Predictor.Model != nil {
		return
	}
	var predictorModel *ModelSpec
	switch {
	case isvc.Spec.Predictor.SKLearn != nil:
		predictorModel = isvc.assignSKLearnRuntime(config)

	case isvc.Spec.Predictor.Tensorflow != nil:
		predictorModel = isvc.assignTensorflowRuntime(config)

	case isvc.Spec.Predictor.XGBoost != nil:
		predictorModel = isvc.assignXGBoostRuntime(config)

	case isvc.Spec.Predictor.PyTorch != nil:
		predictorModel = isvc.assignPyTorchRuntime(config)

	case isvc.Spec.Predictor.Triton != nil:
		predictorModel = isvc.assignTritonRuntime(config)

	case isvc.Spec.Predictor.ONNX != nil:
		predictorModel = isvc.assignONNXRuntime(config)

	case isvc.Spec.Predictor.PMML != nil:
		predictorModel = isvc.assignPMMLRuntime(config)

	case isvc.Spec.Predictor.LightGBM != nil:
		predictorModel = isvc.assignLightGBMRuntime(config)

	case isvc.Spec.Predictor.Paddle != nil:
		predictorModel = isvc.assignPaddleRuntime(config)
	}
	isvc.Spec.Predictor.Model = predictorModel
}

func (isvc *InferenceService) assignSKLearnRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.SKLearn.StorageURI == nil {
		return nil
	}
	// assign built-in runtime based on protocol version
	var runtime = constants.SKLearnServer
	if isvc.Spec.Predictor.SKLearn.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.SKLearn.ProtocolVersion {
		runtime = constants.MLServer
		if isvc.ObjectMeta.Labels == nil {
			isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: constants.MLServerModelClassSKLearn}
		} else {
			isvc.ObjectMeta.Labels[constants.ModelClassLabel] = constants.MLServerModelClassSKLearn
		}
	}
	// remove sklearn spec
	isvc.Spec.Predictor.SKLearn = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelSKLearn},
		PredictorExtensionSpec: isvc.Spec.Predictor.SKLearn.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignTensorflowRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.Tensorflow.StorageURI == nil {
		return nil
	}
	// assign built-in runtime based on gpu config
	var runtime = constants.TFServing
	if utils.IsGPUEnabled(isvc.Spec.Predictor.Tensorflow.Resources) {
		runtime = constants.TFServingGPU
	}
	// remove tensorflow spec
	isvc.Spec.Predictor.Tensorflow = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelTensorflow},
		PredictorExtensionSpec: isvc.Spec.Predictor.Tensorflow.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignXGBoostRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.SKLearn.StorageURI == nil {
		return nil
	}
	// assign built-in runtime based on protocol version
	var runtime = constants.XGBServer
	if isvc.Spec.Predictor.XGBoost.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.XGBoost.ProtocolVersion {
		runtime = constants.MLServer
		if isvc.ObjectMeta.Labels == nil {
			isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: constants.MLServerModelClassXGBoost}
		} else {
			isvc.ObjectMeta.Labels[constants.ModelClassLabel] = constants.MLServerModelClassXGBoost
		}
	}
	// remove xgboost spec
	isvc.Spec.Predictor.XGBoost = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelXGBoost},
		PredictorExtensionSpec: isvc.Spec.Predictor.XGBoost.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignPyTorchRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified or protocol version is not v1.
	if isvc.Spec.Predictor.PyTorch.StorageURI == nil ||
		(isvc.Spec.Predictor.PyTorch.ProtocolVersion != nil &&
			constants.ProtocolV1 != *isvc.Spec.Predictor.PyTorch.ProtocolVersion) {
		return nil
	}
	// assign built-in runtime based on gpu config
	var runtime string
	if isvc.Spec.Predictor.PyTorch.ModelClassName != "" {
		runtime = constants.PYTorchServer
		if utils.IsGPUEnabled(isvc.Spec.Predictor.PyTorch.Resources) {
			runtime = constants.PYTorchServerGPU
		}

		if isvc.ObjectMeta.Labels == nil {
			isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: isvc.Spec.Predictor.PyTorch.ModelClassName}
		} else {
			isvc.ObjectMeta.Labels[constants.ModelClassLabel] = isvc.Spec.Predictor.PyTorch.ModelClassName
		}
	} else {
		runtime = constants.TorchServe
		if utils.IsGPUEnabled(isvc.Spec.Predictor.PyTorch.Resources) {
			runtime = constants.TorchServeGPU
		}
	}
	// remove pytorch spec
	isvc.Spec.Predictor.PyTorch = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelPYTorch},
		PredictorExtensionSpec: isvc.Spec.Predictor.PyTorch.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignTritonRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.Triton.StorageURI == nil {
		return nil
	}
	// assign built-in runtime
	var runtime = constants.TritonServer
	// remove triton spec
	isvc.Spec.Predictor.Triton = nil

	// TODO: pytorch framework is assigned as default, needs to find a to get framework from model.
	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelPYTorch},
		PredictorExtensionSpec: isvc.Spec.Predictor.Triton.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignONNXRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.ONNX.StorageURI == nil {
		return nil
	}
	// assign built-in runtime
	var runtime = constants.TritonServer
	// remove onnx spec
	isvc.Spec.Predictor.ONNX = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelONNX},
		PredictorExtensionSpec: isvc.Spec.Predictor.ONNX.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignPMMLRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.PMML.StorageURI == nil {
		return nil
	}
	// assign built-in runtime
	var runtime = constants.PMMLServer
	// remove pmml spec
	isvc.Spec.Predictor.PMML = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelPMML},
		PredictorExtensionSpec: isvc.Spec.Predictor.PMML.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignLightGBMRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.LightGBM.StorageURI == nil {
		return nil
	}
	// assign built-in runtime
	var runtime = constants.LGBServer
	// remove lightgbm spec
	isvc.Spec.Predictor.LightGBM = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelLightGBM},
		PredictorExtensionSpec: isvc.Spec.Predictor.LightGBM.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}

func (isvc *InferenceService) assignPaddleRuntime(config *InferenceServicesConfig) *ModelSpec {
	// skips if the storage uri is not specified
	if isvc.Spec.Predictor.SKLearn.StorageURI == nil {
		return nil
	}
	// assign built-in runtime
	var runtime = constants.PaddleServer
	// remove paddle spec
	isvc.Spec.Predictor.Paddle = nil

	return &ModelSpec{
		Framework:              v1alpha1.Framework{Name: constants.SupportedModelPaddle},
		PredictorExtensionSpec: isvc.Spec.Predictor.Paddle.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
}
