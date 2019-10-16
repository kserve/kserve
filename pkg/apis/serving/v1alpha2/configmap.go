package v1alpha2

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMap Keys
const (
	PredictorConfigKeyName       = "predictors"
	TransformerConfigKeyName     = "transformers"
	ExplainerConfigKeyName       = "explainers"
	InferenceCommonConfigKeyName = "inferenceCommon"
	// Use a very small percentage here so the minimum bound defined at
	// https://github.com/knative/serving/blob/1d263950f9f2fea85a4dd394948a029c328af9d9/pkg/reconciler/revision/resources/resourceboundary.go#L30
	DefaultQueueSideCarResourcePercentage = "0.1"
)

// +k8s:openapi-gen=false
type ExplainerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion  string   `json:"defaultImageVersion"`
	AllowedImageVersions []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type ExplainersConfig struct {
	AlibiExplainer ExplainerConfig `json:"alibi,omitempty"`
}

// +k8s:openapi-gen=false
type PredictorConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion    string   `json:"defaultImageVersion"`
	DefaultGpuImageVersion string   `json:"defaultGpuImageVersion"`
	AllowedImageVersions   []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type PredictorsConfig struct {
	Tensorflow PredictorConfig `json:"tensorflow,omitempty"`
	TensorRT   PredictorConfig `json:"tensorrt,omitempty"`
	Xgboost    PredictorConfig `json:"xgboost,omitempty"`
	SKlearn    PredictorConfig `json:"sklearn,omitempty"`
	PyTorch    PredictorConfig `json:"pytorch,omitempty"`
	ONNX       PredictorConfig `json:"onnx,omitempty"`
}

// +k8s:openapi-gen=false
type TransformerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion  string   `json:"defaultImageVersion"`
	AllowedImageVersions []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type TransformersConfig struct {
	Feast TransformerConfig `json:"feast,omitempty"`
}

// +k8s:openapi-gen=false
type CommonConfig struct {
	QueueSideCarResourcePercentage string `json:"queueSideCarResourcePercentage"`
}

// +k8s:openapi-gen=false
type InferenceServicesConfig struct {
	Transformers *TransformersConfig `json:"transformers"`
	Predictors   *PredictorsConfig   `json:"predictors"`
	Explainers   *ExplainersConfig   `json:"explainers"`
	Common       *CommonConfig       `json:"common"`
}

func GetInferenceServicesConfig(client client.Client) (*InferenceServicesConfig, error) {
	configMap := &v1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		return nil, err
	}

	endpointsConfigMap, err := NewInferenceServicesConfig(configMap)
	if err != nil {
		return nil, err
	}

	return endpointsConfigMap, nil
}

func NewInferenceServicesConfig(configMap *v1.ConfigMap) (*InferenceServicesConfig, error) {
	predictorsConfig, err := getPredictorsConfigs(configMap)
	if err != nil {
		return nil, err
	}
	transformersConfig, err := getTransformersConfigs(configMap)
	if err != nil {
		return nil, err
	}
	explainersConfig, err := getExplainersConfigs(configMap)
	if err != nil {
		return nil, err
	}
	commonConfig, err := getCommonConfigs(configMap)
	if err != nil {
		return nil, err
	}
	return &InferenceServicesConfig{
		Predictors:   predictorsConfig,
		Transformers: transformersConfig,
		Explainers:   explainersConfig,
		Common:       commonConfig,
	}, nil
}

func getPredictorsConfigs(configMap *v1.ConfigMap) (*PredictorsConfig, error) {
	predictorConfig := &PredictorsConfig{}
	if data, ok := configMap.Data[PredictorConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &predictorConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", PredictorConfigKeyName, err)
		}
	}
	return predictorConfig, nil
}

func getTransformersConfigs(configMap *v1.ConfigMap) (*TransformersConfig, error) {
	transformerConfig := &TransformersConfig{}
	if data, ok := configMap.Data[TransformerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &transformerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", TransformerConfigKeyName, err)
		}
	}
	return transformerConfig, nil
}

func getExplainersConfigs(configMap *v1.ConfigMap) (*ExplainersConfig, error) {
	explainerConfig := &ExplainersConfig{}
	if data, ok := configMap.Data[ExplainerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &explainerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", ExplainerConfigKeyName, err)
		}
	}
	return explainerConfig, nil
}

func getCommonConfigs(configMap *v1.ConfigMap) (*CommonConfig, error) {
	commonConfig := &CommonConfig{}
	if data, ok := configMap.Data[InferenceCommonConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &commonConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", InferenceCommonConfigKeyName, err)
		}
	}

	if commonConfig.QueueSideCarResourcePercentage == "" {
		commonConfig.QueueSideCarResourcePercentage = DefaultQueueSideCarResourcePercentage
	}

	return commonConfig, nil
}
