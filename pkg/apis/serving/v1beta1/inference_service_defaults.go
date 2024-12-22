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
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	// logger for the mutating webhook.
	mutatorLogger = logf.Log.WithName("inferenceservice-v1beta1-mutating-webhook")
)

// +kubebuilder:object:generate=false
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
// InferenceServiceDefaulter is responsible for setting default values on the InferenceService
// when created or updated.
//
// NOTE: The +kubebuilder:object:generate=false and +k8s:deepcopy-gen=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type InferenceServiceDefaulter struct {
}

// +kubebuilder:webhook:path=/mutate-inferenceservices,mutating=true,failurePolicy=fail,groups=serving.kserve.io,resources=inferenceservices,verbs=create;update,versions=v1beta1,name=inferenceservice.kserve-webhook-server.defaulter
var _ webhook.CustomDefaulter = &InferenceServiceDefaulter{}

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

func (d *InferenceServiceDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	isvc, err := convertToInferenceService(obj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceService")
		return err
	}
	mutatorLogger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "isvc", isvc.Spec.Predictor)
	cfg, err := config.GetConfig()
	if err != nil {
		mutatorLogger.Error(err, "unable to set up client config")
		return err
	}
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		mutatorLogger.Error(err, "unable to create clientSet")
		return err
	}
	// Todo: call api server only once to get all configs
	configMap, err := NewInferenceServicesConfig(clientSet)
	if err != nil {
		return err
	}
	deployConfig, err := NewDeployConfig(clientSet)
	if err != nil {
		return err
	}
	localModelConfig, err := NewLocalModelConfig(clientSet)
	if err != nil {
		return err
	}
	securityConfig, err := NewSecurityConfig(clientSet)
	if err != nil {
		return err
	}

	_, localModelDisabledForIsvc := isvc.ObjectMeta.Annotations[constants.DisableLocalModelKey]
	var models *v1alpha1.LocalModelCacheList
	if !localModelDisabledForIsvc && localModelConfig.Enabled {
		var c client.Client
		if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
			mutatorLogger.Error(err, "Failed to start client")
			return err
		}
		models = &v1alpha1.LocalModelCacheList{}
		if err := c.List(ctx, models); err != nil {
			mutatorLogger.Error(err, "Cannot List local models")
			return err
		}
	}

	// Pass a list of LocalModelCache resources to set the local model label if there is a match
	isvc.DefaultInferenceService(configMap, deployConfig, securityConfig, models)
	return nil
}

func (isvc *InferenceService) DefaultInferenceService(config *InferenceServicesConfig, deployConfig *DeployConfig, securityConfig *SecurityConfig, models *v1alpha1.LocalModelCacheList) {
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

	isvc.setLocalModelLabel(models)
	if securityConfig != nil && !securityConfig.AutoMountServiceAccountToken {
		disableAutomountServiceAccountToken(isvc)
	}
}

// disableAutomountServiceAccountToken sets AutomountServiceAccountToken to be false
// Usually serving runtimes do not need access to kubernetes apiserver, so we set it to false by default.
// This can be overridden by setting AutomountServiceAccountToken to true in the InferenceService spec
func disableAutomountServiceAccountToken(isvc *InferenceService) {
	if isvc.Spec.Predictor.AutomountServiceAccountToken == nil {
		isvc.Spec.Predictor.AutomountServiceAccountToken = proto.Bool(false)
	}
	if isvc.Spec.Transformer != nil && isvc.Spec.Transformer.AutomountServiceAccountToken == nil {
		isvc.Spec.Transformer.AutomountServiceAccountToken = proto.Bool(false)
	}
	if isvc.Spec.Explainer != nil && isvc.Spec.Explainer.AutomountServiceAccountToken == nil {
		isvc.Spec.Explainer.AutomountServiceAccountToken = proto.Bool(false)
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

	case isvc.Spec.Predictor.HuggingFace != nil:
		isvc.assignHuggingFaceRuntime()

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

func (isvc *InferenceService) assignHuggingFaceRuntime() {
	// assign protocol version 'v2' if not provided for backward compatibility
	if isvc.Spec.Predictor.HuggingFace.ProtocolVersion == nil {
		protocolV2 := constants.ProtocolV2
		isvc.Spec.Predictor.HuggingFace.ProtocolVersion = &protocolV2
	}
	isvc.Spec.Predictor.Model = &ModelSpec{
		ModelFormat:            ModelFormat{Name: constants.SupportedModelHuggingFace},
		PredictorExtensionSpec: isvc.Spec.Predictor.HuggingFace.PredictorExtensionSpec,
	}
	// remove huggingface spec
	isvc.Spec.Predictor.HuggingFace = nil
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
	switch isvc.Spec.Predictor.Model.ModelFormat.Name {
	case constants.SupportedModelXGBoost:
		modelClass = constants.MLServerModelClassXGBoost
	case constants.SupportedModelLightGBM:
		modelClass = constants.MLServerModelClassLightGBM
	case constants.SupportedModelMLFlow:
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
	if (constants.ProtocolV2 == *isvc.Spec.Predictor.Model.ProtocolVersion) || (constants.ProtocolGRPCV2 == *isvc.Spec.Predictor.Model.ProtocolVersion) {
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

// If there is a LocalModelCache resource, add the name of the LocalModelCache and sourceModelUri to the isvc,
// which is used by the local model controller to manage PV/PVCs.
func (isvc *InferenceService) setLocalModelLabel(models *v1alpha1.LocalModelCacheList) {
	if models == nil {
		return
	}
	// Todo: support multiple storage uris?
	var storageUri string
	var predictor ComponentImplementation
	if predictor = isvc.Spec.Predictor.GetImplementation(); predictor == nil {
		return
	}
	if predictor.GetStorageUri() == nil {
		return
	}
	storageUri = *isvc.Spec.Predictor.GetImplementation().GetStorageUri()
	var localModel *v1alpha1.LocalModelCache
	for i, model := range models.Items {
		if strings.HasPrefix(storageUri, model.Spec.SourceModelUri) {
			localModel = &models.Items[i]
			break
		}
	}
	if localModel == nil {
		return
	}
	if isvc.Labels == nil {
		isvc.Labels = make(map[string]string)
	}
	if isvc.Annotations == nil {
		isvc.Annotations = make(map[string]string)
	}
	isvc.Labels[constants.LocalModelLabel] = localModel.Name
	isvc.Annotations[constants.LocalModelSourceUriAnnotationKey] = localModel.Spec.SourceModelUri
	// TODO: node group needs to be retrieved from isvc node group annotation when we support multiple node groups
	isvc.Annotations[constants.LocalModelPVCNameAnnotationKey] = localModel.Name + "-" + localModel.Spec.NodeGroups[0]
	mutatorLogger.Info("LocalModelCache found", "model", localModel.Name, "namespace", isvc.Namespace, "isvc", isvc.Name)
}
