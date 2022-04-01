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
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestTensorflowValidation(t *testing.T) {
	g, config := gomega.NewGomegaWithT(t), InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				DefaultTimeout:         60,
				MultiModelServer:       false,
			},
		},
	}
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"RejectGpuRuntimeVersionWithoutGpuResource": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest-gpu"),
					},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeExcludesGPU)),
		},
		"RejectGpuGpuResourceWithoutGpuRuntime": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("latest"),
						Container: v1.Container{
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeIncludesGPU)),
		},
		"ValidStorageUri": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
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
			scenario.spec.Tensorflow.Default(&config)
			res := scenario.spec.Tensorflow.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestTensorflowDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				DefaultTimeout:         60,
				MultiModelServer:       false,
			},
		},
	}
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	defaultGpuResource := v1.ResourceList{
		v1.ResourceCPU:                  resource.MustParse("1"),
		v1.ResourceMemory:               resource.MustParse("2Gi"),
		constants.NvidiaGPUResourceType: resource.MustParse("1"),
	}
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultRuntimeVersion": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("1.14.0"),
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
		"DefaultGpuRuntimeVersion": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						Container: v1.Container{
							Resources: v1.ResourceRequirements{
								Requests: defaultGpuResource,
								Limits:   defaultGpuResource,
							},
						},
					},
				},
			},
			expected: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("1.14.0-gpu"),
						Container: v1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: v1.ResourceRequirements{
								Requests: defaultGpuResource,
								Limits:   defaultGpuResource,
							},
						},
					},
				},
			},
		},
		"DefaultResources": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("1.14.0"),
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
			scenario.spec.Tensorflow.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateTFServingContainer(t *testing.T) {

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
			Tensorflow: PredictorConfig{
				ContainerImage:      "tfserving",
				DefaultImageVersion: "1.14.0",
				DefaultTimeout:      60,
				MultiModelServer:    false,
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
					Name: "tfserving",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tfserving:1.14.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Command:   []string{"/usr/bin/tensorflow_model_server"},
				Args: []string{
					"--port=" + TensorflowServingGRPCPort,
					"--rest_api_port=" + TensorflowServingRestPort,
					"--model_name=someName",
					"--model_base_path=/mnt/models",
					"--rest_api_timeout_in_ms=60000",
				},
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tfserving",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Image:     "tfserving:2.0.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tfserving:2.0.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Command:   []string{"/usr/bin/tensorflow_model_server"},
				Args: []string{
					"--port=" + TensorflowServingGRPCPort,
					"--rest_api_port=" + TensorflowServingRestPort,
					"--model_name=someName",
					"--model_base_path=/mnt/models",
					"--rest_api_timeout_in_ms=60000",
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tfserving",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("2.0.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "tfserving:2.0.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Command:   []string{"/usr/bin/tensorflow_model_server"},
				Args: []string{
					"--port=" + TensorflowServingGRPCPort,
					"--rest_api_port=" + TensorflowServingRestPort,
					"--model_name=someName",
					"--model_base_path=/mnt/models",
					"--rest_api_timeout_in_ms=60000",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(metav1.ObjectMeta{Name: "someName"}, &scenario.isvc.Spec.Predictor.ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestTensorflowIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				Tensorflow: PredictorConfig{
					ContainerImage:         "tfserving",
					DefaultImageVersion:    "1.14.0",
					DefaultGpuImageVersion: "1.14.0-gpu",
					DefaultTimeout:         60,
					MultiModelServer:       mmsCase,
				},
			},
		}
		defaultResource = v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		}
		defaultGpuResource := v1.ResourceList{
			v1.ResourceCPU:                  resource.MustParse("1"),
			v1.ResourceMemory:               resource.MustParse("2Gi"),
			constants.NvidiaGPUResourceType: resource.MustParse("1"),
		}
		scenarios := map[string]struct {
			spec     PredictorSpec
			expected bool
		}{
			"DefaultRuntimeVersion": {
				spec: PredictorSpec{
					Tensorflow: &TFServingSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultGpuRuntimeVersion": {
				spec: PredictorSpec{
					Tensorflow: &TFServingSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							Container: v1.Container{
								Resources: v1.ResourceRequirements{
									Requests: defaultGpuResource,
									Limits:   defaultGpuResource,
								},
							},
						},
					},
				},
				expected: mmsCase,
			},
			"DefaultResources": {
				spec: PredictorSpec{
					Tensorflow: &TFServingSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				scenario.spec.Tensorflow.Default(&config)
				res := scenario.spec.Tensorflow.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestTensorflowIsFrameworkSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorflow := "tensorflow"
	unsupportedFramework := "framework"
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tfserving",
				DefaultImageVersion:    "1.14.0",
				DefaultGpuImageVersion: "1.14.0-gpu",
				DefaultTimeout:         60,
				SupportedFrameworks:    []string{tensorflow},
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
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			framework: tensorflow,
			expected:  true,
		},
		"UnsupportedFramework": {
			spec: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Tensorflow.Default(&config)
			res := scenario.spec.Tensorflow.IsFrameworkSupported(scenario.framework, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
