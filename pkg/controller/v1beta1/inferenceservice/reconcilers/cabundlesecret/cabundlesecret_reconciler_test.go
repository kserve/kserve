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

package cabundlesecret

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetDesiredCaBundleSecretForUserNS(t *testing.T) {
	cabundleSecretData := make(map[string][]byte)

	// cabundle data
	cabundleSecretData["cabundle.crt"] = []byte("SAMPLE_CA_BUNDLE")
	targetNamespace := "test"
	testCases := []struct {
		name                   string
		namespace              string
		secretData             map[string][]byte
		expectedCopiedCaSecret *corev1.Secret
	}{
		{
			name:       "Do not create a ca secret,if CaBundleSecretName is '' in storageConfig of inference-config configmap",
			namespace:  targetNamespace,
			secretData: cabundleSecretData,
			expectedCopiedCaSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultGlobalCaBundleSecretName,
					Namespace: targetNamespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: cabundleSecretData,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getDesiredCaBundleSecretForUserNS(constants.DefaultGlobalCaBundleSecretName, tt.namespace, tt.secretData)
			if diff := cmp.Diff(tt.expectedCopiedCaSecret, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}
