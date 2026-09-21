/*
Copyright 2026 The KServe Authors.

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

package crdvalidation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	pkgtest "github.com/kserve/kserve/pkg/testing"
)

func TestGeneratedCRDsDoNotExposeWVAScaling(t *testing.T) {
	crdRoot := filepath.Join(pkgtest.ProjectRoot(), "config", "crd", "full", "llmisvc")
	crdFiles := []string{
		"serving.kserve.io_llminferenceservices.yaml",
		"serving.kserve.io_llminferenceserviceconfigs.yaml",
	}

	for _, file := range crdFiles {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(crdRoot, file)) // #nosec G304 -- file is selected from the fixed generated-CRD list above.
			if err != nil {
				t.Fatalf("read generated CRD: %v", err)
			}

			var crd apiextensionsv1.CustomResourceDefinition
			if err := yaml.Unmarshal(data, &crd); err != nil {
				t.Fatalf("parse generated CRD: %v", err)
			}

			for _, version := range crd.Spec.Versions {
				if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
					continue
				}
				assertNoWVAScalingProperty(
					t,
					version.Schema.OpenAPIV3Schema,
					"spec",
					version.Name,
				)
			}
		})
	}
}

func assertNoWVAScalingProperty(
	t *testing.T,
	schema *apiextensionsv1.JSONSchemaProps,
	path string,
	version string,
) {
	t.Helper()
	for name, property := range schema.Properties {
		propertyPath := path + "." + name
		if name == "wva" && strings.HasSuffix(path, ".scaling") {
			t.Errorf("generated CRD version %s exposes %s", version, propertyPath)
		}
		assertNoWVAScalingProperty(t, &property, propertyPath, version)
	}

	if schema.Items != nil && schema.Items.Schema != nil {
		assertNoWVAScalingProperty(t, schema.Items.Schema, path+"[]", version)
	}
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		assertNoWVAScalingProperty(t, schema.AdditionalProperties.Schema, path+"{}", version)
	}
}
