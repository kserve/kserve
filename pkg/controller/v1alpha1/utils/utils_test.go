/*
Copyright 2024 The KServe Authors.

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

package utils

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/kserve/kserve/pkg/constants"
)

func TestInferenceServiceConfigNamespace(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("uses KSERVE_CONFIG_NAMESPACE when set", func(t *testing.T) {
		t.Setenv(KServeConfigNamespaceEnv, "app-ns")
		g.Expect(InferenceServiceConfigNamespace()).To(gomega.Equal("app-ns"))
	})

	t.Run("falls back to KServeNamespace when unset", func(t *testing.T) {
		t.Setenv(KServeConfigNamespaceEnv, "")
		g.Expect(InferenceServiceConfigNamespace()).To(gomega.Equal(constants.KServeNamespace))
	})
}
