package modelconfig

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	testify "github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestProcess_addOrUpdate(t *testing.T) {
	log.SetLogger(log.ZapLogger(true))
	testCases := map[string]struct {
		modelConfigs      ModelConfigs
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"add to nil data": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
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
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
		},
		"add to empty data": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
		},
		"add to empty data value": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
		},
		"add to non-empty data": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model2",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model2", Framework: "framework2"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}},` +
						`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
		},
		"update": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//new-model1", Framework: "new-framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}},` +
						`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//new-model1","framework":"new-framework1","memory":"0"}},` +
						`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
		},
	}
	for _, tc := range testCases {
		mConfig := NewConfigsDelta(tc.modelConfigs, ModelConfigs{})
		err := mConfig.Process(tc.configMap)
		testify.Nil(t, err)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}

func TestProcess_delete(t *testing.T) {
	log.SetLogger(log.ZapLogger(true))
	testCases := map[string]struct {
		modelConfigs      ModelConfigs
		configMap         *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
	}{
		"delete nil data": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
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
			},
		},
		"delete empty data": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			},
		},
		"delete empty data value": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
		},
		"delete filename non-exist": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
		},
		"delete filename exist": {
			modelConfigs: ModelConfigs{
				&ModelConfig{
					Name: "model1",
					Spec: &v1beta1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test",
				},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
		},
	}
	for _, tc := range testCases {
		mConfig := NewConfigsDelta(ModelConfigs{}, tc.modelConfigs)
		err := mConfig.Process(tc.configMap)
		testify.Nil(t, err)
		testify.Equal(t, tc.expectedConfigMap, tc.configMap)
	}
}
