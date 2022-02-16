/*
Copyright 2021 The KServe Authors.

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
	"fmt"
	"reflect"
	"strconv"

	"github.com/kserve/kserve/pkg/constants"
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
	deploymentMode, ok := isvc.ObjectMeta.Annotations[constants.DeploymentMode]
	if !ok || deploymentMode != string(constants.ModelMeshDeployment) {
		// Only attempt to assign runtimes for non-modelmesh predictors
		isvc.setPredictorModelDefaults()
	}
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
}

func (isvc *InferenceService) setPredictorModelDefaults() {
	if isvc.Spec.Predictor.Model != nil {
		// add mlserver specific default values
		if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelMLFlow ||
			(isvc.Spec.Predictor.Model.Runtime != nil &&
				*isvc.Spec.Predictor.Model.Runtime == constants.MLServer) {
			isvc.setMlServerDefaults()
		}
		// add torchserve specific default values
		if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelPyTorch &&
			(isvc.Spec.Predictor.Model.Runtime == nil ||
				*isvc.Spec.Predictor.Model.Runtime == constants.TorchServe) {
			isvc.setTorchServeDefaults()
		}
	}
	switch {
	case isvc.Spec.Predictor.SKLearn != nil:
		isvc.assignSKLearnRuntime()

	case isvc.Spec.Predictor.Tensorflow != nil:
		isvc.assignTensorflowRuntime()

	case isvc.Spec.Predictor.XGBoost != nil:
		isvc.assignXGBoostRuntime()

	case isvc.Spec.Predictor.PyTorch != nil:
		isvc.assignPyTorchRuntime()

	case isvc.Spec.Predictor.Triton != nil:
		isvc.assignTritonRuntime()

	case isvc.Spec.Predictor.ONNX != nil:
		isvc.assignONNXRuntime()

	case isvc.Spec.Predictor.PMML != nil:
		isvc.assignPMMLRuntime()

	case isvc.Spec.Predictor.LightGBM != nil:
		isvc.assignLightGBMRuntime()

	case isvc.Spec.Predictor.Paddle != nil:
		isvc.assignPaddleRuntime()
	}
}

func (isvc *InferenceService) assignSKLearnRuntime() {
	// assign built-in runtime based on protocol version
	if isvc.Spec.Predictor.SKLearn.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.SKLearn.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.SKLearnServer
	if isvc.Spec.Predictor.SKLearn.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.SKLearn.ProtocolVersion {
		runtime = constants.MLServer

		if isvc.Spec.Predictor.SKLearn.StorageURI == nil {
			isvc.Spec.Predictor.SKLearn.Env = append(isvc.Spec.Predictor.SKLearn.Env,
				v1.EnvVar{
					Name:  constants.MLServerLoadModelsStartupEnv,
					Value: strconv.FormatBool(false),
				},
			)
		} else {
			isvc.Spec.Predictor.SKLearn.Env = append(isvc.Spec.Predictor.SKLearn.Env,
				v1.EnvVar{
					Name:  constants.MLServerModelNameEnv,
					Value: isvc.Name,
				},
				v1.EnvVar{
					Name:  constants.MLServerModelURIEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
			)
		}

		if isvc.ObjectMeta.Labels == nil {
			isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: constants.MLServerModelClassSKLearn}
		} else {
			isvc.ObjectMeta.Labels[constants.ModelClassLabel] = constants.MLServerModelClassSKLearn
		}
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelSKLearn},
		PredictorExtensionSpec: isvc.Spec.Predictor.SKLearn.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove sklearn spec
	isvc.Spec.Predictor.SKLearn = nil
}

func (isvc *InferenceService) assignTensorflowRuntime() {
	// assign built-in runtime based on gpu config
	if isvc.Spec.Predictor.Tensorflow.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.Tensorflow.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.TFServing
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelTensorflow},
		PredictorExtensionSpec: isvc.Spec.Predictor.Tensorflow.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove tensorflow spec
	isvc.Spec.Predictor.Tensorflow = nil
}

func (isvc *InferenceService) assignXGBoostRuntime() {
	// assign built-in runtime based on protocol version
	if isvc.Spec.Predictor.XGBoost.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.XGBoost.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.XGBServer
	if isvc.Spec.Predictor.XGBoost.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.XGBoost.ProtocolVersion {
		runtime = constants.MLServer

		if isvc.Spec.Predictor.XGBoost.StorageURI == nil {
			isvc.Spec.Predictor.XGBoost.Env = append(isvc.Spec.Predictor.XGBoost.Env,
				v1.EnvVar{
					Name:  constants.MLServerLoadModelsStartupEnv,
					Value: strconv.FormatBool(false),
				},
			)
		} else {
			isvc.Spec.Predictor.XGBoost.Env = append(isvc.Spec.Predictor.XGBoost.Env,
				v1.EnvVar{
					Name:  constants.MLServerModelNameEnv,
					Value: isvc.Name,
				},
				v1.EnvVar{
					Name:  constants.MLServerModelURIEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
			)
		}

		if isvc.ObjectMeta.Labels == nil {
			isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: constants.MLServerModelClassXGBoost}
		} else {
			isvc.ObjectMeta.Labels[constants.ModelClassLabel] = constants.MLServerModelClassXGBoost
		}
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelXGBoost},
		PredictorExtensionSpec: isvc.Spec.Predictor.XGBoost.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove xgboost spec
	isvc.Spec.Predictor.XGBoost = nil
}

func (isvc *InferenceService) assignPyTorchRuntime() {
	// assign built-in runtime based on gpu config
	if isvc.ObjectMeta.Labels == nil {
		isvc.ObjectMeta.Labels = map[string]string{constants.ServiceEnvelope: constants.ServiceEnvelopeKServe}
	} else {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServe
	}
	if isvc.Spec.Predictor.PyTorch.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.PyTorch.ProtocolVersion {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServeV2
	}
	runtime := constants.TorchServe
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPyTorch},
		PredictorExtensionSpec: isvc.Spec.Predictor.PyTorch.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove pytorch spec
	isvc.Spec.Predictor.PyTorch = nil
}

func (isvc *InferenceService) assignTritonRuntime() {
	// assign built-in runtime
	var runtime = constants.TritonServer
	if isvc.Spec.Predictor.Triton.StorageURI == nil {
		isvc.Spec.Predictor.Triton.Args = append(isvc.Spec.Predictor.Triton.Args,
			fmt.Sprintf("%s=%s", "--model-control-mode", "explicit"))
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelTriton},
		PredictorExtensionSpec: isvc.Spec.Predictor.Triton.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove triton spec
	isvc.Spec.Predictor.Triton = nil
}

func (isvc *InferenceService) assignONNXRuntime() {
	// assign built-in runtime
	var runtime = constants.TritonServer
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelONNX},
		PredictorExtensionSpec: isvc.Spec.Predictor.ONNX.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove onnx spec
	isvc.Spec.Predictor.ONNX = nil
}

func (isvc *InferenceService) assignPMMLRuntime() {
	// assign built-in runtime
	if isvc.Spec.Predictor.PMML.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.PMML.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.PMMLServer
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPMML},
		PredictorExtensionSpec: isvc.Spec.Predictor.PMML.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove pmml spec
	isvc.Spec.Predictor.PMML = nil
}

func (isvc *InferenceService) assignLightGBMRuntime() {
	// assign built-in runtime
	if isvc.Spec.Predictor.LightGBM.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.LightGBM.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.LGBServer
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelLightGBM},
		PredictorExtensionSpec: isvc.Spec.Predictor.LightGBM.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove lightgbm spec
	isvc.Spec.Predictor.LightGBM = nil
}

func (isvc *InferenceService) assignPaddleRuntime() {
	// assign built-in runtime
	if isvc.Spec.Predictor.Paddle.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		isvc.Spec.Predictor.Paddle.ProtocolVersion = &defaultProtocol
	}
	var runtime = constants.PaddleServer
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPaddle},
		PredictorExtensionSpec: isvc.Spec.Predictor.Paddle.PredictorExtensionSpec,
		Runtime:                &runtime,
	}
	// remove paddle spec
	isvc.Spec.Predictor.Paddle = nil
}

func (isvc *InferenceService) setMlServerDefaults() {
	// set environment variables based on storage uri
	if isvc.Spec.Predictor.Model.StorageURI == nil {
		isvc.Spec.Predictor.Model.Env = append(isvc.Spec.Predictor.Model.Env,
			v1.EnvVar{
				Name:  constants.MLServerLoadModelsStartupEnv,
				Value: strconv.FormatBool(false),
			},
		)
	} else {
		isvc.Spec.Predictor.Model.Env = append(isvc.Spec.Predictor.Model.Env,
			v1.EnvVar{
				Name:  constants.MLServerModelNameEnv,
				Value: isvc.Name,
			},
			v1.EnvVar{
				Name:  constants.MLServerModelURIEnv,
				Value: constants.DefaultModelLocalMountPath,
			},
		)
	}
	// set model class
	modelClass := constants.MLServerModelClassSKLearn
	if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelXGBoost {
		modelClass = constants.MLServerModelClassXGBoost
	} else if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelLightGBM {
		modelClass = constants.MLServerModelClassLightGBM
	} else if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelMLFlow {
		modelClass = constants.MLServerModelClassMLFlow
	}
	if isvc.ObjectMeta.Labels == nil {
		isvc.ObjectMeta.Labels = map[string]string{constants.ModelClassLabel: modelClass}
	} else {
		isvc.ObjectMeta.Labels[constants.ModelClassLabel] = modelClass
	}
}

func (isvc *InferenceService) setTorchServeDefaults() {
	// set torchserve service envelope based on protocol version
	if isvc.ObjectMeta.Labels == nil {
		isvc.ObjectMeta.Labels = map[string]string{constants.ServiceEnvelope: constants.ServiceEnvelopeKServe}
	} else {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServe
	}
	if isvc.Spec.Predictor.Model.ProtocolVersion != nil &&
		constants.ProtocolV2 == *isvc.Spec.Predictor.Model.ProtocolVersion {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServeV2
	}
}
