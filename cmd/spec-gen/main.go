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

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	kserve "github.com/kserve/kserve/pkg/openapi"
	"k8s.io/klog/v2"
	"k8s.io/kube-openapi/pkg/common"
	spec "k8s.io/kube-openapi/pkg/validation/spec"
)

// Generate OpenAPI spec definitions for InferenceService Resource
func main() {
	if len(os.Args) <= 1 {
		klog.Fatal("Supply a version")
	}
	version := os.Args[1]
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	oAPIDefs := kserve.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.MustCreateRef("#/definitions/" + common.EscapeJsonPointer(swaggify(name)))
	})
	defs := spec.Definitions{}
	for defName, val := range oAPIDefs {
		defs[swaggify(defName)] = val.Schema
	}
	swagger := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Swagger:     "2.0",
			Definitions: defs,
			Paths:       &spec.Paths{Paths: map[string]spec.PathItem{}},
			Info: &spec.Info{
				InfoProps: spec.InfoProps{
					Title:       "KServe",
					Description: "Python SDK for KServe",
					Version:     version,
				},
			},
		},
	}
	jsonBytes, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		klog.Fatal(err.Error())
	}
	fmt.Println(string(jsonBytes))
}

func swaggify(name string) string {
	name = strings.ReplaceAll(name, "github.com/kserve/kserve/pkg/apis/serving/", "")
	name = strings.ReplaceAll(name, "./pkg/apis/serving/", "")
	name = strings.ReplaceAll(name, "knative.dev/pkg/apis/duck/v1.", "knative/")
	name = strings.ReplaceAll(name, "knative.dev/pkg/apis.", "knative/")
	name = strings.ReplaceAll(name, "k8s.io/api/core/", "")
	name = strings.ReplaceAll(name, "k8s.io/apimachinery/pkg/apis/meta/", "")
	name = strings.ReplaceAll(name, "k8s.io/apimachinery/pkg/api/", "")
	name = strings.ReplaceAll(name, "/", ".")
	return name
}
