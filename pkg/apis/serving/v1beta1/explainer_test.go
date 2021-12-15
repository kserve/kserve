/*

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

package v1beta1

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestExplainerGetStorageUri(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	storagePath := "model"
	storageKey := "minioIO"
	expectedSecretSpec := "s3://<bucket-placeholder>/" + storagePath
	scenarios := map[string]struct {
		spec     ExplainerSpec
		expected *string
	}{
		"storageSpecTest": {
			spec: ExplainerSpec{
				ART: &ARTExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						Storage: &StorageSpec{
							Path:       &storagePath,
							StorageKey: &storageKey,
						},
					},
				},
			},
			expected: &expectedSecretSpec,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			generatedUri := scenario.spec.ART.ExplainerExtensionSpec.GetStorageUri()
			if !g.Expect(&generatedUri).To(gomega.Equal(&scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}
