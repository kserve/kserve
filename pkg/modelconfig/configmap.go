/*
Copyright 2020 kubeflow.org.

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
	Name string            `json:"modelName"`
	Spec v1beta1.ModelSpec `json:"modelSpec"`
}

type ModelConfigs []ModelConfig

type ConfigsDelta struct {
	updated map[string]ModelConfig
	deleted []string
}

func NewConfigsDelta(updatedConfigs ModelConfigs, deletedConfigs []string) *ConfigsDelta {
	return &ConfigsDelta{
		updated: slice2Map(updatedConfigs),
		deleted: deletedConfigs,
	}
}

//multi-model ConfigMap
//apiVersion: v1
//kind: ConfigMap
//metadata:
//  name: models-config
//  namespace: <user-model-namespace>
//data:
//  models.json: |
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
	if len(config.updated) == 0 && len(config.deleted) == 0 {
		return nil
	}
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	data, err := decode(configMap.Data[constants.ModelConfigFileName])
	if err != nil {
		return fmt.Errorf("while updating %s err %v", configMap.Name, err)
	}

	//add/update models
	for name, spec := range config.updated {
		data[name] = spec
	}
	//delete models
	for _, name := range config.deleted {
		if _, ok := data[name]; ok {
			delete(data, name)
		} else {
			logger.Info("Model does not exist in ConfigMap.",
				"model", name, "ConfigMap", configMap.Name)
		}
	}

	to, err := encode(data)
	if err != nil {
		return fmt.Errorf("while updating %s err %v", configMap.Name, err)
	}
	configMap.Data[constants.ModelConfigFileName] = to
	return nil
}

func slice2Map(from ModelConfigs) map[string]ModelConfig {
	to := make(map[string]ModelConfig)
	for _, config := range from {
		to[config.Name] = config
	}
	return to
}

func map2Slice(from map[string]ModelConfig) ModelConfigs {
	to := make(ModelConfigs, 0, len(from))
	for _, config := range from {
		to = append(to, config)
	}
	return to
}

func decode(from string) (map[string]ModelConfig, error) {
	modelConfigs := ModelConfigs{}
	if len(from) != 0 {
		if err := json.Unmarshal([]byte(from), &modelConfigs); err != nil {
			return nil, err
		}
	}
	return slice2Map(modelConfigs), nil
}

func encode(from map[string]ModelConfig) (string, error) {
	modelConfigs := map2Slice(from)
	to, err := json.Marshal(&modelConfigs)
	return string(to), err
}
