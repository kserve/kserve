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

func GetInferenceServiceConfigMap(client client.Client) (*v1.ConfigMap, error) {
	configMap := &v1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

func GetPredictorConfigs(configMap *v1.ConfigMap) (*PredictorsConfig, error) {
	predictorConfig := &PredictorsConfig{}
	if data, ok := configMap.Data[constants.PredictorConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &predictorConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return predictorConfig, nil
}

func GetTransformerConfigs(configMap *v1.ConfigMap) (*TransformersConfig, error) {
	transformerConfig := &TransformersConfig{}
	if data, ok := configMap.Data[constants.TransformerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &transformerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return transformerConfig, nil
}

func GetExplainerConfigs(configMap *v1.ConfigMap) (*ExplainersConfig, error) {
	explainerConfig := &ExplainersConfig{}
	if data, ok := configMap.Data[constants.ExplainerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &explainerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall json string due to %v ", err)
		}
	}
	return explainerConfig, nil
}
