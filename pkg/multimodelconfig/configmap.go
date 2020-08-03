package multimodelconfig

import (
	"encoding/json"
	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

type MultiModelConfig struct {
	fileName    string
	fileContent ModelDefinition
}

type ModelDefinition struct {
	StorageUri string `json:"storageUri"`
	Framework  string `json:"framework"`
	Memory     string `json:"memory"`
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

func AddOrUpdateMultiModelConfigMap(configMap *v1.ConfigMap, multiModelConfig ...MultiModelConfig) error {
	if data, err := convert(multiModelConfig...); err == nil {
		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		for filename, modeldef := range data {
			configMap.Data[filename] = modeldef
		}
		return nil
	} else {
		return err
	}
}

func DeleteMultiModelConfigMap(configMap *v1.ConfigMap, modelFileNames ...string) {
	if configMap.Data == nil || len(configMap.Data) == 0 {
		klog.Warningf("configmap %s data is empty, cannot remove models", configMap.Name)
		return
	}
	for _, filename := range modelFileNames {
		if _, ok := configMap.Data[filename]; ok {
			delete(configMap.Data, filename)
		} else {
			klog.Warningf("%s does not exist in configmap %s", filename, configMap.Name)
		}
	}
}

func convert(from ...MultiModelConfig) (map[string]string, error) {
	to := map[string]string{}
	for _, mmcfg := range from {
		if b, err := json.Marshal(&(mmcfg.fileContent)); err == nil {
			to[mmcfg.fileName] = string(b)
		} else {
			return nil, err
		}
	}
	return to, nil
}
