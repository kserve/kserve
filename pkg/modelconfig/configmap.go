package modelconfig

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("ModelConfig")
var json = jsoniter.ConfigCompatibleWithStandardLibrary

type ModelConfig struct {
	Name string             `json:"modelName"`
	Spec *v1beta1.ModelSpec `json:"modelSpec"`
}

type ModelConfigs []*ModelConfig

type ConfigsDelta struct {
	updated map[string]*ModelConfig
	deleted map[string]*ModelConfig
}

func NewConfigsDelta(updatedConfigs ModelConfigs, deletedConfigs ModelConfigs) *ConfigsDelta {
	return &ConfigsDelta{
		updated: slice2Map(updatedConfigs),
		deleted: slice2Map(deletedConfigs),
	}
}

//multi-model ConfigMap
//apiVersion: v1
//kind: ConfigMap
//metadata:
//  name: models-config
//  namespace: <user-model-namespace>
//data:
//  example_models.json: |
//    [
//      {
//        "modelName": "model1",
//        "modelSpec": {
//          "storageUri": "s3://example-bucket/path/to/model1",
//          "framework": "sklearn",
//          "memory": "1G"
//        }
//      },
//      {
//        "modelName": "model2",
//        "modelSpec": {
//          "storageUri": "s3://example-bucket/path/to/model2",
//          "framework": "sklearn",
//          "memory": "1G"
//        }
//      }
//   ]

func (config *ConfigsDelta) Process(configMap *v1.ConfigMap) (err error) {
	if err = config.apply(configMap); err != nil {
		return err
	}
	if err = config.delete(configMap); err != nil {
		return err
	}
	return nil
}

func (config *ConfigsDelta) apply(configMap *v1.ConfigMap) error {
	if len(config.updated) == 0 {
		return nil
	}
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	data, err := decode(configMap.Data[constants.ModelConfigFileName])
	if err != nil {
		return fmt.Errorf("while updating %s err %v", configMap.Name, err)
	}

	for name, spec := range config.updated {
		data[name] = spec
	}
	to, err := encode(data)
	if err != nil {
		return fmt.Errorf("while updating %s err %v", configMap.Name, err)
	}
	configMap.Data[constants.ModelConfigFileName] = to
	return nil
}

func (config *ConfigsDelta) delete(configMap *v1.ConfigMap) error {
	if len(config.deleted) == 0 || len(configMap.Data) == 0 {
		return nil
	}
	configData, ok := configMap.Data[constants.ModelConfigFileName]
	if !ok || len(configData) == 0 {
		return nil
	}

	data, err := decode(configData)
	if err != nil {
		return fmt.Errorf("while deleting %s err %v", configMap.Name, err)
	}

	for name, _ := range config.deleted {
		if _, ok := data[name]; ok {
			delete(data, name)
		} else {
			logger.Info("Model does not exist in ConfigMap.",
				"model", name, "ConfigMap", configMap.Name)
		}
	}
	to, err := encode(data)
	if err != nil {
		return fmt.Errorf("while deleting %s err %v", configMap.Name, err)
	}
	configMap.Data[constants.ModelConfigFileName] = to
	return nil
}

func slice2Map(from ModelConfigs) map[string]*ModelConfig {
	to := make(map[string]*ModelConfig)
	for _, config := range from {
		to[config.Name] = config
	}
	return to
}

func map2slice(from map[string]*ModelConfig) ModelConfigs {
	to := make(ModelConfigs, 0, len(from))
	for _, config := range from {
		to = append(to, config)
	}
	return to
}

func decode(from string) (map[string]*ModelConfig, error) {
	modelConfigs := ModelConfigs{}
	if len(from) != 0 {
		if err := json.Unmarshal([]byte(from), &modelConfigs); err != nil {
			return nil, err
		}
	}
	return slice2Map(modelConfigs), nil
}

func encode(from map[string]*ModelConfig) (string, error) {
	modelConfigs := map2slice(from)
	to, err := json.Marshal(&modelConfigs)
	return string(to), err
}
