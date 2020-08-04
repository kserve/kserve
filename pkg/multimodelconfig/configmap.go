package multimodelconfig

import (
	"encoding/json"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("MultiModelConfig")

type Handler struct {
	updatedCfg map[string]string
	deletedCfg map[string]string
}

type ModelConfig struct {
	fileName    string
	fileContent *ModelDefinition
}

type ModelDefinition struct {
	StorageUri string `json:"storageUri"`
	Framework  string `json:"framework"`
}

func NewHandler(updatedCfg[]ModelConfig, deletedCfg []ModelConfig) (*Handler, error) {
	var handler Handler
	var err error
	if handler.updatedCfg, err = convert(updatedCfg...); err != nil {
		return nil, err
	}
	if handler.deletedCfg, err = convert(deletedCfg...); err != nil {
		return nil, err
	}
	return &handler, err
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
//      memory: 1G
//    }
//  example_model_name2.json: |
//    {
//      storageUri: s3://example-bucket/path/to/model_name2
//      framework: sklearn
//      memory: 2G
//    }

func (config *Handler) Process(configMap *v1.ConfigMap) {
	config.addOrUpdate(configMap)
	config.delete(configMap)
}

func (config *Handler) addOrUpdate(configMap *v1.ConfigMap) {
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

func (config *Handler) delete(configMap *v1.ConfigMap) {
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
