package multimodelconfig

import (
	"encoding/json"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MultiModelConfig struct {
	fileName string
	fileContent ModelDefinition
}

type ModelDefinition struct {
	StorageUri string  `json:"storageUri"`
	Framework string  `json:"framework"`
	Memory string  `json:"memory"`
}
//create ConfigMap
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
func CreateMultiModelConfigMap(modelNamespace string, inferenceServiceName string, multiModelConfig... MultiModelConfig) (*v1.ConfigMap, error) {
	if data, err := convert(multiModelConfig...); err == nil {
		configMap := v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.DefaultMultiModelConfigMapName(inferenceServiceName),
				Namespace: modelNamespace,
			},
			Data: data,
		}
		return &configMap, nil
	} else {
		return nil, err
	}
}

func convert(from... MultiModelConfig) (map[string]string, error) {
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

//Update

//Delete