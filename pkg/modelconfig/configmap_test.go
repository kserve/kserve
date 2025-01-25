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
	"sort"
	"testing"

	testify "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
)

func TestProcessAddOrUpdate(t *testing.T) {
	log.SetLogger(zap.New())
	testCases := map[string]struct {
		modelConfigs ModelConfigs
		configMap    *corev1.ConfigMap
		expected     string
	}{
		"add to nil data": {
			modelConfigs: ModelConfigs{
				ModelConfig{
					Name: "model1",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
		},
		"add to empty data": {
			modelConfigs: ModelConfigs{
				ModelConfig{
					Name: "model1",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
		},
		"add to empty data value": {
			modelConfigs: ModelConfigs{
				ModelConfig{
					Name: "model1",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//model1", Framework: "framework1"},
				},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
		},
		"add to non-empty data": {
			modelConfigs: ModelConfigs{
				ModelConfig{
					Name: "model2",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//model2", Framework: "framework2"},
				},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}},` +
				`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
		},
		"update": {
			modelConfigs: ModelConfigs{
				ModelConfig{
					Name: "model1",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//new-model1", Framework: "new-framework1"},
				},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}},` +
						`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//new-model1","framework":"new-framework1","memory":"0"}},` +
				`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
		},
	}
	for _, tc := range testCases {
		mConfig := NewConfigsDelta(tc.modelConfigs, nil)
		err := mConfig.Process(tc.configMap)
		require.NoError(t, err)
		data, err := getSortedConfigData(tc.configMap.Data[constants.ModelConfigFileName])
		require.NoError(t, err)
		expected, _ := getSortedConfigData(tc.expected)
		testify.Equal(t, expected, data)
	}
}

func TestProcessDelete(t *testing.T) {
	log.SetLogger(zap.New())
	testCases := map[string]struct {
		modelConfigs []string
		configMap    *corev1.ConfigMap
		expected     string
	}{
		"delete nil data": {
			modelConfigs: []string{"model1"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
			},
			expected: "[]",
		},
		"delete empty data": {
			modelConfigs: []string{"model1"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			},
			expected: "[]",
		},
		"delete empty data value": {
			modelConfigs: []string{"model1"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			},
			expected: "[]",
		},
		"delete filename non-exist": {
			modelConfigs: []string{"model1"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
			expected: `[{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
		},
		"delete filename exist": {
			modelConfigs: []string{"model1"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}}]`,
				},
			},
			expected: "[]",
		},
	}
	for _, tc := range testCases {
		mConfig := NewConfigsDelta(nil, tc.modelConfigs)
		err := mConfig.Process(tc.configMap)
		require.NoError(t, err)
		data, err := getSortedConfigData(tc.configMap.Data[constants.ModelConfigFileName])
		require.NoError(t, err)
		expected, _ := getSortedConfigData(tc.expected)
		testify.Equal(t, expected, data)
	}
}

func TestProcess(t *testing.T) {
	log.SetLogger(zap.New())
	testCases := map[string]struct {
		updated   ModelConfigs
		deleted   []string
		configMap *corev1.ConfigMap
		expected  string
	}{
		"process configmap": {
			updated: ModelConfigs{
				ModelConfig{
					Name: "model1",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//new-model1", Framework: "new-framework1"},
				},
				ModelConfig{
					Name: "model3",
					Spec: v1alpha1.ModelSpec{StorageURI: "s3//model3", Framework: "framework3"},
				},
			},
			deleted: []string{"model2"},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test"},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"framework1","memory":"0"}},` +
						`{"modelName":"model2","modelSpec":{"storageUri":"s3//model2","framework":"framework2","memory":"0"}}]`,
				},
			},
			expected: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//new-model1","framework":"new-framework1","memory":"0"}},` +
				`{"modelName":"model3","modelSpec":{"storageUri":"s3//model3","framework":"framework3","memory":"0"}}]`,
		},
	}
	for _, tc := range testCases {
		mConfig := NewConfigsDelta(tc.updated, tc.deleted)
		err := mConfig.Process(tc.configMap)
		require.NoError(t, err)
		data, err := getSortedConfigData(tc.configMap.Data[constants.ModelConfigFileName])
		require.NoError(t, err)
		expected, _ := getSortedConfigData(tc.expected)
		testify.Equal(t, expected, data)
	}
}

func getSortedConfigData(input string) (output ModelConfigs, err error) {
	if err = json.Unmarshal([]byte(input), &output); err != nil {
		return nil, err
	}
	sort.Slice(output, func(i, j int) bool {
		return output[i].Name < output[j].Name
	})
	return output, nil
}

func TestCreateEmptyModelConfig(t *testing.T) {
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			Annotations: map[string]string{
				constants.DeploymentMode: string(constants.ModelMeshDeployment),
			},
		},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor: v1beta1.PredictorSpec{
				Tensorflow: &v1beta1.TFServingSpec{
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
	}
	shardStrategy := memory.MemoryStrategy{}
	shardId := shardStrategy.GetShard(isvc)[0]
	expected := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.ModelConfigName(isvc.Name, shardId),
			Namespace: isvc.Namespace,
			Labels:    isvc.Labels,
		},
		Data: map[string]string{
			constants.ModelConfigFileName: "[]",
		},
	}

	configMap, err := CreateEmptyModelConfig(isvc, shardId)
	require.NoError(t, err)
	testify.Equal(t, expected, configMap)
}
