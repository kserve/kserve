package multimodelconfig

import (
	"encoding/json"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("MultiModelConfig")

type ConfigsDelta struct {
	updatedCfg map[string]string
	deletedCfg map[string]string
}

type ModelConfig struct {
	fileName    string
	fileContent *ModelDefinition
}

//ModelDefinition will be replaced by ModelSpec
//after we merge https://github.com/kubeflow/kfserving/pull/991 to master
type ModelDefinition struct {
	StorageUri string `json:"storageUri"`
	Framework  string `json:"framework"`
}

func NewConfigsDelta(updatedCfg[]ModelConfig, deletedCfg []ModelConfig) (*ConfigsDelta, error) {
	updatedcfgs, err := convert(updatedCfg...)
	if err != nil {
		return nil, err
	}
	deletedcfgs, err := convert(deletedCfg...)
	if err != nil {
		return nil, err
	}
	return &ConfigsDelta{ updatedCfg: updatedcfgs, deletedCfg: deletedcfgs }, err
}

//multi-model ConfigMap
//apiVersion: v1
//kind: ConfigMap
//metadata:
//  name: models-config
//  namespace: <user-model-namespace>
//data:
//  example_model_name1.json: |
//    {
//      storageUri: s3://example-bucket/path/to/model_name1
//      framework: sklearn
//    }
//  example_model_name2.json: |
//    {
//      storageUri: s3://example-bucket/path/to/model_name2
//      framework: sklearn
//    }

func (config *ConfigsDelta) Process(configMap *v1.ConfigMap) {
	config.apply(configMap)
	config.delete(configMap)
}

func (config *ConfigsDelta) apply(configMap *v1.ConfigMap) {
	if isEmpty(config.updatedCfg) {
		return
	}
	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}
	for name, content := range config.updatedCfg {
		configMap.Data[name] = content
	}
}

func (config *ConfigsDelta) delete(configMap *v1.ConfigMap) {
	if isEmpty(config.deletedCfg) {
		return
	}
	if configMap.Data == nil || len(configMap.Data) == 0 {
		logger.Info("Cannot remove models from empty configmap",
			"configmap name", configMap.Name)
		return
	}
	for filename, _:= range config.deletedCfg {
		if _, ok := configMap.Data[filename]; ok {
			delete(configMap.Data, filename)
		} else {
			logger.Info("Model filename does not exist in configmap",
				"model file name", filename, "configmap name", configMap.Name)
		}
	}
}

func isEmpty(input map[string]string) bool {
	return input == nil || len(input) == 0
}

func convert(from ...ModelConfig) (map[string]string, error) {
	to := map[string]string{}
	for _, mmcfg := range from {
		if mmcfg.fileContent == nil {
			to[mmcfg.fileName] = ""
		} else {
			if b, err := json.Marshal(&(mmcfg.fileContent)); err == nil {
				to[mmcfg.fileName] = string(b)
			} else {
				return nil, err
			}
		}
	}
	return to, nil
}
