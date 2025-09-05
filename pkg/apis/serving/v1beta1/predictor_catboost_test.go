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

package v1beta1

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
)

func TestCatBoostValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.CatBoost.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestCatBoostDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}

	config := &InferenceServicesConfig{
		Resource: ResourceConfig{
			CPULimit:      "1",
			MemoryLimit:   "2Gi",
			CPURequest:    "1",
			MemoryRequest: "2Gi",
		},
	}

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultRuntimeVersion": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						// RuntimeVersion:  proto.String("v0.1.0"),
						ProtocolVersion: &protocolV1,
						Container: corev1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
		"DefaultRuntimeVersionWithCustomImage": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						Container: corev1.Container{
							Image: "customImage:latest",
						},
					},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
						Container: corev1.Container{
							Name:  constants.InferenceServiceContainerName,
							Image: "customImage:latest",
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
		"ProtocolV2": {
			spec: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
						Container: corev1.Container{
							Image: "customImage:latest",
						},
					},
				},
			},
			expected: PredictorSpec{
				CatBoost: &CatBoostSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
						Container: corev1.Container{
							Name:  constants.InferenceServiceContainerName,
							Image: "customImage:latest",
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.CatBoost.Default(config)
			g.Expect(scenario.spec.CatBoost.Container).To(gomega.Equal(scenario.expected.CatBoost.Container))
			g.Expect(*scenario.spec.CatBoost.ProtocolVersion).To(gomega.Equal(*scenario.expected.CatBoost.ProtocolVersion))
		})
	}
}
