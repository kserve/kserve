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

package v1alpha1

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

func TestMarshalServingRuntime(t *testing.T) {
	endpoint := "endpoint"
	version := "1.0"
	v := ServingRuntime{
		Spec: ServingRuntimeSpec{
			GrpcDataEndpoint: &endpoint,
			ServingRuntimePodSpec: ServingRuntimePodSpec{
				Containers: []v1.Container{
					{
						Args:    []string{"arg1", "arg2"},
						Command: []string{"command", "command2"},
						Env: []v1.EnvVar{
							{Name: "name", Value: "value"},
							{
								Name: "fromSecret",
								ValueFrom: &v1.EnvVarSource{
									SecretKeyRef: &v1.SecretKeySelector{Key: "mykey"},
								},
							},
						},
						Image:           "image",
						Name:            "name",
						ImagePullPolicy: "IfNotPresent",
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
				},
			},
			SupportedModelFormats: []SupportedModelFormat{
				{
					Name:    "name",
					Version: &version,
				},
			},
		},
	}

	b, err := yaml.Marshal(v)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(b))
}
