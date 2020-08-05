package multimodelconfig

import (
	testify "github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestProcess_addOrUpdate(t *testing.T) {
	testCases := map[string]struct {
		modelConfigs []ModelConfig
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"add to empty": {
			modelConfigs: []ModelConfig{
				{
					fileName: "example_model1.json",
					fileContent: &ModelDefinition{
						StorageUri: "s3//model1",
						Framework:  "framework1",
					},
				},
				{
					fileName: "example_model2.json",
					fileContent: &ModelDefinition{
						StorageUri: "s3//model2",
						Framework:  "framework2",
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
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
		},
		"add to non-empty": {
			modelConfigs: []ModelConfig{
				{
					fileName: "example_model3.json",
					fileContent: &ModelDefinition{
						StorageUri: "s3//model3",
						Framework:  "framework3",
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
					"example_model3.json": `{"storageUri":"s3//model3","framework":"framework3"}`,
				},
			},
		},
		"update": {
			modelConfigs: []ModelConfig{
				{
					fileName: "example_model1.json",
					fileContent: &ModelDefinition{
						StorageUri: "s3//new-model1",
						Framework:  "new-framework1",
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//new-model1","framework":"new-framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
		},
	}
	for _, tc := range testCases {
		mConfig, err := NewConfigsDelta(tc.modelConfigs, []ModelConfig{})
		testify.Nil(t, err)
		mConfig.Process(tc.configMap)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}

func TestProcess_delete(t *testing.T) {
	log.SetLogger(log.ZapLogger(false))
	testCases := map[string]struct {
		modelConfigs      []ModelConfig
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"delete nil configmap": {
			modelConfigs: []ModelConfig{
				{"example_model1.json", nil},
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
			},
		},
		"delete empty configmap": {
			modelConfigs: []ModelConfig{
				{"example_model1.json", nil},
			},
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
			modelConfigs: []ModelConfig{
				{"example.json", nil},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
		},
		"delete filename ": {
			modelConfigs: []ModelConfig{
				{"example_model1.json", nil},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model1.json": `{"storageUri":"s3//model1","framework":"framework1"}`,
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"example_model2.json": `{"storageUri":"s3//model2","framework":"framework2"}`,
				},
			},
		},
	}
	for _, tc := range testCases {
		handler, err := NewConfigsDelta([]ModelConfig{}, tc.modelConfigs)
		testify.Nil(t, err)
		handler.Process(tc.configMap)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}
