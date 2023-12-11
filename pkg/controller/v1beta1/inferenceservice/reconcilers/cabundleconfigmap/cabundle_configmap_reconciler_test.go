/*
Copyright 2023 The KServe Authors.

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

package cabundleconfigmap

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetDesiredCaBundleConfigMapForUserNS(t *testing.T) {
	cabundleConfigMapData := make(map[string]string)

	// cabundle data
	cabundleConfigMapData["cabundle.crt"] = "SAMPLE_CA_BUNDLE"
	targetNamespace := "test"
	testCases := []struct {
		name                      string
		namespace                 string
		configMapData             map[string]string
		expectedCopiedCaConfigMap *corev1.ConfigMap
	}{
		{
			name:          "Do not create a ca bundle configmap,if CaBundleConfigMapName is '' in storageConfig of inference-config configmap",
			namespace:     targetNamespace,
			configMapData: cabundleConfigMapData,
			expectedCopiedCaConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultGlobalCaBundleConfigMapName,
					Namespace: targetNamespace,
				},
				Data: cabundleConfigMapData,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getDesiredCaBundleConfigMapForUserNS(constants.DefaultGlobalCaBundleConfigMapName, tt.namespace, tt.configMapData)
			if diff := cmp.Diff(tt.expectedCopiedCaConfigMap, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}
