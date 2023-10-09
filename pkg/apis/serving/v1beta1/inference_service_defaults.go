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
	deployConfig, err := NewDeployConfig(cli)
	if err != nil {
		panic(err)
	}
	isvc.DefaultInferenceService(configMap, deployConfig)
}

func (isvc *InferenceService) DefaultInferenceService(config *InferenceServicesConfig, deployConfig *DeployConfig) {
	deploymentMode, ok := isvc.ObjectMeta.Annotations[constants.DeploymentMode]

	if !ok && deployConfig != nil {
		if deployConfig.DefaultDeploymentMode == string(constants.ModelMeshDeployment) ||
			deployConfig.DefaultDeploymentMode == string(constants.RawDeployment) {
			if isvc.ObjectMeta.Annotations == nil {
				isvc.ObjectMeta.Annotations = map[string]string{}
			}
			isvc.ObjectMeta.Annotations[constants.DeploymentMode] = deployConfig.DefaultDeploymentMode
		}
	}
	components := []Component{isvc.Spec.Transformer, isvc.Spec.Explainer}
	if !ok || deploymentMode != string(constants.ModelMeshDeployment) {
		// Only attempt to assign runtimes and apply defaulting logic for non-modelmesh predictors
		isvc.setPredictorModelDefaults()
		components = append(components, &isvc.Spec.Predictor)
	} else {
		// If this is a modelmesh predictor, we still want to do "Exactly One" validation.
		if err := validateExactlyOneImplementation(&isvc.Spec.Predictor); err != nil {
			mutatorLogger.Error(ExactlyOneErrorFor(&isvc.Spec.Predictor), "Missing component implementation")
		}
	}

	for _, component := range components {
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

	if isvc.Spec.Predictor.Model != nil && isvc.Spec.Predictor.Model.ProtocolVersion == nil {
		if isvc.Spec.Predictor.Model.ModelFormat.Name == constants.SupportedModelTriton {
			// set 'v2' as default protocol version for triton server
			protocolV2 := constants.ProtocolV2
			isvc.Spec.Predictor.Model.ProtocolVersion = &protocolV2
		}
	}
}

func (isvc *InferenceService) assignSKLearnRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelSKLearn},
		PredictorExtensionSpec: isvc.Spec.Predictor.SKLearn.PredictorExtensionSpec,
	}
	// remove sklearn spec
	isvc.Spec.Predictor.SKLearn = nil
}

func (isvc *InferenceService) assignTensorflowRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelTensorflow},
		PredictorExtensionSpec: isvc.Spec.Predictor.Tensorflow.PredictorExtensionSpec,
	}
	// remove tensorflow spec
	isvc.Spec.Predictor.Tensorflow = nil
}

func (isvc *InferenceService) assignXGBoostRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelXGBoost},
		PredictorExtensionSpec: isvc.Spec.Predictor.XGBoost.PredictorExtensionSpec,
	}
	// remove xgboost spec
	isvc.Spec.Predictor.XGBoost = nil
}

func (isvc *InferenceService) assignPyTorchRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPyTorch},
		PredictorExtensionSpec: isvc.Spec.Predictor.PyTorch.PredictorExtensionSpec,
	}
	// remove pytorch spec
	isvc.Spec.Predictor.PyTorch = nil
}

func (isvc *InferenceService) assignTritonRuntime() {
	// assign protocol version 'v2' if not provided for backward compatibility
	if isvc.Spec.Predictor.Triton.ProtocolVersion == nil {
		protocolV2 := constants.ProtocolV2
		isvc.Spec.Predictor.Triton.ProtocolVersion = &protocolV2
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelTriton},
		PredictorExtensionSpec: isvc.Spec.Predictor.Triton.PredictorExtensionSpec,
	}
	// remove triton spec
	isvc.Spec.Predictor.Triton = nil
}

func (isvc *InferenceService) assignONNXRuntime() {
	// assign protocol version 'v2' if not provided for backward compatibility
	if isvc.Spec.Predictor.ONNX.ProtocolVersion == nil {
		protocolV2 := constants.ProtocolV2
		isvc.Spec.Predictor.ONNX.ProtocolVersion = &protocolV2
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelONNX},
		PredictorExtensionSpec: isvc.Spec.Predictor.ONNX.PredictorExtensionSpec,
	}
	// remove onnx spec
	isvc.Spec.Predictor.ONNX = nil
}

func (isvc *InferenceService) assignPMMLRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPMML},
		PredictorExtensionSpec: isvc.Spec.Predictor.PMML.PredictorExtensionSpec,
	}
	// remove pmml spec
	isvc.Spec.Predictor.PMML = nil
}

func (isvc *InferenceService) assignLightGBMRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelLightGBM},
		PredictorExtensionSpec: isvc.Spec.Predictor.LightGBM.PredictorExtensionSpec,
	}
	// remove lightgbm spec
	isvc.Spec.Predictor.LightGBM = nil
}

func (isvc *InferenceService) assignPaddleRuntime() {
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelPaddle},
		PredictorExtensionSpec: isvc.Spec.Predictor.Paddle.PredictorExtensionSpec,
	}
	// remove paddle spec
	isvc.Spec.Predictor.Paddle = nil
}

func (isvc *InferenceService) SetRuntimeDefaults() {
	// add mlserver specific default values
	if *isvc.Spec.Predictor.Model.Runtime == constants.MLServer {
		isvc.SetMlServerDefaults()
	}
	// add torchserve specific default values
	if *isvc.Spec.Predictor.Model.Runtime == constants.TorchServe {
		isvc.SetTorchServeDefaults()
	}
	// add triton specific default values
	if *isvc.Spec.Predictor.Model.Runtime == constants.TritonServer {
		isvc.SetTritonDefaults()
	}
}

func (isvc *InferenceService) SetMlServerDefaults() {
	// set 'v2' as default protocol version for mlserver
	if isvc.Spec.Predictor.Model.ProtocolVersion == nil {
		protocolV2 := constants.ProtocolV2
		isvc.Spec.Predictor.Model.ProtocolVersion = &protocolV2
	}
	// set environment variables based on storage uri
	if isvc.Spec.Predictor.Model.StorageURI == nil && isvc.Spec.Predictor.Model.Storage == nil {
		isvc.Spec.Predictor.Model.Env = utils.AppendEnvVarIfNotExists(isvc.Spec.Predictor.Model.Env,
			v1.EnvVar{
				Name:  constants.MLServerLoadModelsStartupEnv,
				Value: strconv.FormatBool(false),
			},
		)
	} else {
		isvc.Spec.Predictor.Model.Env = utils.AppendEnvVarIfNotExists(isvc.Spec.Predictor.Model.Env,
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

func (isvc *InferenceService) SetTorchServeDefaults() {
	// set 'v1' as default protocol version for torchserve
	if isvc.Spec.Predictor.Model.ProtocolVersion == nil {
		protocolV1 := constants.ProtocolV1
		isvc.Spec.Predictor.Model.ProtocolVersion = &protocolV1
	}
	// set torchserve service envelope based on protocol version
	if isvc.ObjectMeta.Labels == nil {
		isvc.ObjectMeta.Labels = map[string]string{constants.ServiceEnvelope: constants.ServiceEnvelopeKServe}
	} else {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServe
	}
	if constants.ProtocolV2 == *isvc.Spec.Predictor.Model.ProtocolVersion {
		isvc.ObjectMeta.Labels[constants.ServiceEnvelope] = constants.ServiceEnvelopeKServeV2
	}

	// set torchserve env variable "PROTOCOL_VERSION" based on ProtocolVersion
	isvc.Spec.Predictor.Model.Env = append(isvc.Spec.Predictor.Model.Env,
		v1.EnvVar{
			Name:  constants.ProtocolVersionENV,
			Value: string(*isvc.Spec.Predictor.Model.ProtocolVersion),
		})
}

func (isvc *InferenceService) SetTritonDefaults() {
	// set 'v2' as default protocol version for triton server
	if isvc.Spec.Predictor.Model.ProtocolVersion == nil {
		protocolV2 := constants.ProtocolV2
		isvc.Spec.Predictor.Model.ProtocolVersion = &protocolV2
	}
	// set model-control-model arg to 'explicit' if storage uri is nil
	if isvc.Spec.Predictor.Model.StorageURI == nil && isvc.Spec.Predictor.Model.Storage == nil {
		isvc.Spec.Predictor.Model.Args = append(isvc.Spec.Predictor.Model.Args,
			fmt.Sprintf("%s=%s", "--model-control-mode", "explicit"))
	}
}
