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

	"github.com/golang/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
		"ValidStorageUri": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("hdfs://modelzoo"),
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
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
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Triton: PredictorConfig{
				ContainerImage:      "tritonserver",
				DefaultImageVersion: "20.03-py3",
				MultiModelServer:    true,
			},
		},
	}
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultRuntimeVersion": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("20.03-py3"),
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
			scenario.spec.Triton.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateTritonContainer(t *testing.T) {

	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "100",
			},
		},
		Requests: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "90",
			},
		},
	}
	var config = InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Triton: PredictorConfig{
				ContainerImage:      "tritonserver",
				DefaultImageVersion: "20.03-py3",
				MultiModelServer:    true,
			},
		},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
		"ContainerSpecWithDefaultImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "triton",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Triton: &TritonSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("20.03-py3"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tritonserver:20.03-py3",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"tritonserver",
					"--model-store=/mnt/models",
					"--grpc-port=9000",
					"--http-port=8080",
					"--allow-grpc=true",
					"--allow-http=true",
				},
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "triton",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Triton: &TritonSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Image:     "tritonserver:19.10-py3",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tritonserver:19.10-py3",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"tritonserver",
					"--model-store=/mnt/models",
					"--grpc-port=9000",
					"--http-port=8080",
					"--allow-grpc=true",
					"--allow-http=true",
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "triton",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(4),
						},
						Triton: &TritonSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("20.03-py3"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tritonserver:20.03-py3",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"tritonserver",
					"--model-store=/mnt/models",
					"--grpc-port=9000",
					"--http-port=8080",
					"--allow-grpc=true",
					"--allow-http=true",
					"--http-thread-count=4",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(metav1.ObjectMeta{Name: "someName"}, &scenario.isvc.Spec.Predictor.DeepCopy().ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestTritonIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				Triton: PredictorConfig{
					ContainerImage:      "tritonserver",
					DefaultImageVersion: "20.03-py3",
					MultiModelServer:    mmsCase,
				},
			},
		}
		defaultResource = v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		}
		scenarios := map[string]struct {
			spec     PredictorSpec
			expected bool
		}{
			"DefaultRuntimeVersion": {
				spec: PredictorSpec{
					Triton: &TritonSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultResources": {
				spec: PredictorSpec{
					Triton: &TritonSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							RuntimeVersion: proto.String("20.05-py3"),
						},
					},
				},
				expected: mmsCase,
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				scenario.spec.Triton.Default(&config)
				res := scenario.spec.Triton.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestTritonIsFrameworkSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorrt := "tensorrt"
	unsupportedFramework := "framework"
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Triton: PredictorConfig{
				ContainerImage:      "tritonserver",
				DefaultImageVersion: "20.08-py3",
				SupportedFrameworks: []string{tensorrt},
			},
		},
	}
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec      PredictorSpec
		framework string
		expected  bool
	}{
		"SupportedFramework": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			framework: tensorrt,
			expected:  true,
		},
		"UnsupportedFramework": {
			spec: PredictorSpec{
				Triton: &TritonSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Triton.Default(&config)
			res := scenario.spec.Triton.IsFrameworkSupported(scenario.framework, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
