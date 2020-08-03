package multimodelconfig

import (
	testify "github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestAddOrUpdateMultiModelConfigMap(t *testing.T) {
	testCases := map[string]struct {
		multiModelConfigs []MultiModelConfig
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"add to empty": {
			multiModelConfigs: []MultiModelConfig{
				{
					fileName: "example_model1.json",
					fileContent: ModelDefinition{
						StorageUri: "s3//model1",
						Framework:  "framework1",
						Memory:     "1G",
					},
				},
				{
					fileName: "example_model2.json",
					fileContent: ModelDefinition{
						StorageUri: "s3//model2",
						Framework:  "framework2",
						Memory:     "1G",
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
		},
		"add to non-empty": {
			multiModelConfigs: []MultiModelConfig{
				{
					fileName: "example_model3.json",
					fileContent: ModelDefinition{
						StorageUri: "s3//model3",
						Framework:  "framework3",
						Memory:     "1G",
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
					"example_model3.json": `{"storageUri":"s3//model3","framework":"framework3","memory":"1G"}`,
				},
			},
		},
		"update": {
			multiModelConfigs: []MultiModelConfig{
				{
					fileName: "example_model1.json",
					fileContent: ModelDefinition{
						StorageUri: "s3//new-model1",
						Framework:  "new-framework1",
						Memory:     "2G",
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//new-model1","framework":"new-framework1","memory":"2G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
		},
	}
	for _, tc := range testCases {
		AddOrUpdateMultiModelConfigMap(tc.configMap, tc.multiModelConfigs...)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}

func TestDeleteMultiModelConfigMap(t *testing.T) {
	testCases := map[string]struct {
		modelFileNames    []string
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"delete nil configmap": {
			modelFileNames: []string{"example_model1.json"},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
			},
		},
		"delete empty configmap": {
			modelFileNames: []string{"example_model1.json"},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{},
			},
		},
		"delete filename non-exist": {
			modelFileNames: []string{"example.json"},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
		},
		"delete filename ": {
			modelFileNames: []string{"example_model1.json"},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1","memory":"1G"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2","memory":"1G"}`,
				},
			},
		},
	}
	for _, tc := range testCases {
		DeleteMultiModelConfigMap(tc.configMap, tc.modelFileNames...)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}
