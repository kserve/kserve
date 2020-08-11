package multimodelconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"io"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
)

var logger = log.Log.WithName("MultiModelConfig")

type ModelConfig map[string]*v1beta1.ModelSpec

type ConfigsDelta struct {
	updated ModelConfig
	deleted ModelConfig
}

func NewConfigsDelta(updatedCfg ModelConfig, deletedCfg ModelConfig) *ConfigsDelta {
	return &ConfigsDelta{updated: updatedCfg, deleted: deletedCfg}
}

//multi-model ConfigMap
//apiVersion: v1
//kind: ConfigMap
//metadata:
//  name: models-config
//  namespace: <user-model-namespace>
//data:
//  example_models.json: |
//    {
//      example_model_name1: {
//        storageUri: s3://example-bucket/path/to/model_name1
//        framework: sklearn
//      },
//      example_model_name2: {
//        storageUri: s3://example-bucket/path/to/model_name2
//        framework: sklearn
//      }
//    }

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

func decode(from string) (to ModelConfig, err error) {
	dec := json.NewDecoder(strings.NewReader(from))
	for {
		if err = dec.Decode(&to); err == io.EOF {
			if len(to) == 0 {
				to = ModelConfig{}
			}
			err = nil
			break
		} else if err != nil {
			err = fmt.Errorf("fail to decode %s", from)
			break
		}
	}
	return to, err
}

func encode(from ModelConfig) (to string, err error) {
	buffer := new(bytes.Buffer)
	err = json.NewEncoder(buffer).Encode(from)
	if err == nil {
		to = strings.TrimSuffix(buffer.String(), "\n")
	}
	return to, err
}
