/*
Copyright 2019 kubeflow.org.

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

	"github.com/go-openapi/spec"
	kfsvcv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"k8s.io/klog"
	"k8s.io/kube-openapi/pkg/common"
)

// Generate OpenAPI spec definitions for KFService Resource
func main() {
	if len(os.Args) <= 1 {
		klog.Fatal("Supply a version")
	}
	version := os.Args[1]
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	oAPIDefs := kfsvcv1alpha1.GetOpenAPIDefinitions(func(name string) spec.Ref {
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
					Title: "KFServing",
					Description: "Python SDK for KFServing",
					Version: version,
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
	name = strings.Replace(name, "github.com/kubeflow/kfserving/pkg/apis/serving", "kfserving", -1)
	parts := strings.Split(name, "/")
	hostParts := strings.Split(parts[0], ".")
	// reverses something like k8s.io to io.k8s
	for i, j := 0, len(hostParts)-1; i < j; i, j = i+1, j-1 {
		hostParts[i], hostParts[j] = hostParts[j], hostParts[i]
	}
	parts[0] = strings.Join(hostParts, ".")
	return strings.Join(parts, ".")
}

