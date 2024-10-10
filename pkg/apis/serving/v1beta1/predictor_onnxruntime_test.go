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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
)

func TestOnnxRuntimeValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidModelExtension": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://my_model.onnx"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidModelExtension": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://my_model.txt"),
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.ONNX.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestONNXRuntimeDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultResources": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v1.0.0"),
					},
				},
			},
			expected: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("v1.0.0"),
						Container: v1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: v1.ResourceRequirements{
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
			scenario.spec.ONNX.Default(nil)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestONNXRuntimeSpec_GetContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	metadata := metav1.ObjectMeta{Name: constants.InferenceServiceContainerName}
	scenarios := map[string]struct {
		spec PredictorSpec
	}{
		"simple": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: v1.Container{
							Name:      constants.InferenceServiceContainerName,
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: v1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.ONNX.GetContainer(metadata, &scenario.spec.ComponentExtensionSpec, nil)
			if !g.Expect(res).To(gomega.Equal(&scenario.spec.ONNX.Container)) {
				t.Errorf("got %v, want %v", res, scenario.spec.ONNX.Container)
			}
		})
	}
}

func TestONNXRuntimeSpec_GetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected constants.InferenceServiceProtocol
	}{
		"default": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: v1.Container{
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: v1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			expected: constants.ProtocolV1,
		},
		"ProtocolSpecified": {
			spec: PredictorSpec{
				ONNX: &ONNXRuntimeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
						StorageURI:      proto.String("s3://modelzoo"),
						Container: v1.Container{
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: v1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			expected: constants.ProtocolV2,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.ONNX.GetProtocol()
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec.Triton, scenario.expected)
			}
		})
	}
}
