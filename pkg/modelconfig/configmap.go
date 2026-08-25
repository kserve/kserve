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

package modelconfig

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var (
	logger = log.Log.WithName("ModelConfig")
	json   = jsoniter.ConfigCompatibleWithStandardLibrary
)

type ModelConfig struct {
	Name string             `json:"modelName"`
	Spec v1alpha1.ModelSpec `json:"modelSpec"`
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

// multi-model ConfigMap
// apiVersion: v1
// kind: ConfigMap
// metadata:
//
//	name: models-config
//	namespace: <user-model-namespace>
//
// data:
//
//	models.json: |
//	  [
//	    {
//	      "modelName": "model1",
//	      "modelSpec": {
//	        "storageUri": "s3://example-bucket/path/to/model1",
//	        "framework": "sklearn",
//	        "memory": "1G"
//	      }
//	    },
//	    {
//	      "modelName": "model2",
//	      "modelSpec": {
//	        "storageUri": "s3://example-bucket/path/to/model2",
//	        "framework": "sklearn",
//	        "memory": "1G"
//	      }
//	    }
//	 ]
func (config *ConfigsDelta) Process(configMap *corev1.ConfigMap) (err error) {
	if len(config.updated) == 0 && len(config.deleted) == 0 {
		return nil
	}
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	data, err := decode(configMap.Data[constants.ModelConfigFileName])
	if err != nil {
		return fmt.Errorf("while updating %s err %w", configMap.Name, err)
	}

	// add/update models
	for name, spec := range config.updated {
		data[name] = spec
	}
	// delete models
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
		return fmt.Errorf("while updating %s err %w", configMap.Name, err)
	}
	configMap.Data[constants.ModelConfigFileName] = to
	return nil
}

func CreateEmptyModelConfig(isvc *v1beta1.InferenceService, shardId int) (*corev1.ConfigMap, error) {
	multiModelConfigMapName := constants.ModelConfigName(isvc.Name, shardId)
	// Create a modelConfig without any models in it
	multiModelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      multiModelConfigMapName,
			Namespace: isvc.Namespace,
			Labels:    isvc.Labels,
		},
		Data: map[string]string{
			constants.ModelConfigFileName: "[]",
		},
	}
	return multiModelConfigMap, nil
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
