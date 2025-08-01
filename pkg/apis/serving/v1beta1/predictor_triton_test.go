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

	"google.golang.org/protobuf/proto"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
)

func TestTritonValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
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
			res := scenario.spec.Triton.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestTritonDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

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
		"DefaultResources": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("20.05-py3"),
					},
				},
			},
			expected: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("20.05-py3"),
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
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Triton.Default(config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestTritonSpec_GetContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	metadata := metav1.ObjectMeta{Name: constants.InferenceServiceContainerName}
	scenarios := map[string]struct {
		spec PredictorSpec
	}{
		"simple": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Name:      constants.InferenceServiceContainerName,
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.Triton.GetContainer(metadata, &scenario.spec.ComponentExtensionSpec, nil)
			if !g.Expect(res).To(gomega.Equal(&scenario.spec.Triton.Container)) {
				t.Errorf("got %v, want %v", res, scenario.spec.Triton.Container)
			}
		})
	}
}

func TestTritonSpec_Default(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

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
		expected *TritonSpec
	}{
		"simple": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			expected: &TritonSpec{
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: proto.String("s3://modelzoo"),
					Container: corev1.Container{
						Name:  constants.InferenceServiceContainerName,
						Image: "image:0.1",
						Args:  nil,
						Env:   nil,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"cpu":    resource.MustParse("1"),
								"memory": resource.MustParse("2Gi"),
							},
							Requests: corev1.ResourceList{
								"memory": resource.MustParse("2Gi"),
								"cpu":    resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Triton.Default(config)
			if !g.Expect(scenario.spec.Triton).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec.Triton, scenario.expected)
			}
		})
	}
}

func TestTritonSpec_GetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected constants.InferenceServiceProtocol
	}{
		"default": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			expected: constants.ProtocolV2,
		},
		"ProtocolSpecified": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
						StorageURI:      proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Image:     "image:0.1",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
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
			res := scenario.spec.Triton.GetProtocol()
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec.Triton, scenario.expected)
			}
		})
	}
}
