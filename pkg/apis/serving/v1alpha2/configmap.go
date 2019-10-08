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

	DefaultImageVersion string `json:"defaultImageVersion"`

	AllowedImageVersions []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type TransformersConfig struct {
	Feast TransformerConfig `json:"feast,omitempty"`
}

// +k8s:openapi-gen=false
type InferenceEndpointsConfigMap struct {
	Transformers *TransformersConfig `json:"transformers,omitempty"`
	Predictors   *PredictorsConfig   `json:"predictors,omitempty"`
	Explainers   *ExplainersConfig   `json:"explainers,omitempty"`
}

func GetInferenceEndpointsConfigMap(client client.Client) (*InferenceEndpointsConfigMap, error) {
	configMap := &v1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		return nil, err
	}

	endpointsConfigMap, err := NewInferenceEndpointsConfigMap(configMap)
	if err != nil {
		return nil, err
	}

	return endpointsConfigMap, nil
}

func NewInferenceEndpointsConfigMap(configMap *v1.ConfigMap) (*InferenceEndpointsConfigMap, error) {
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
	return &InferenceEndpointsConfigMap{
		Predictors:   predictorsConfig,
		Transformers: transformersConfig,
		Explainers:   explainersConfig,
	}, nil
}

func getPredictorsConfigs(configMap *v1.ConfigMap) (*PredictorsConfig, error) {
	predictorConfig := &PredictorsConfig{}
	if data, ok := configMap.Data[constants.PredictorConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &predictorConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return predictorConfig, nil
}

func getTransformersConfigs(configMap *v1.ConfigMap) (*TransformersConfig, error) {
	transformerConfig := &TransformersConfig{}
	if data, ok := configMap.Data[constants.TransformerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &transformerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return transformerConfig, nil
}

func getExplainersConfigs(configMap *v1.ConfigMap) (*ExplainersConfig, error) {
	explainerConfig := &ExplainersConfig{}
	if data, ok := configMap.Data[constants.ExplainerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &explainerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return explainerConfig, nil
}
