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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTorchServeValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			PyTorch: PredictorConfig{
				ContainerImage:      "pytorch/torchserve-kfs",
				DefaultImageVersion: "0.4.1",
				MultiModelServer:    false,
			},
		},
	}
	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("0.4.1"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"RejectGpuRuntimeVersionWithoutGpuResource": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("0.4.1-gpu"),
					},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeExcludesGPU)),
		},
		"RejectGpuGpuResourceWithoutGpuRuntime": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: proto.String("0.4.1"),
						Container: v1.Container{
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeIncludesGPU)),
		},
		"ValidStorageUri": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
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
			scenario.spec.PyTorch.Default(&config)
			res := scenario.spec.PyTorch.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestTorchServeDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			PyTorch: PredictorConfig{
				ContainerImage:      "pytorch/torchserve-kfs",
				DefaultImageVersion: "0.4.1",
				MultiModelServer:    false,
			},
		},
	}

	protocolV1 := constants.ProtocolV1

	defaultResource = v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("2Gi"),
		v1.ResourceCPU:    resource.MustParse("1"),
	}
	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultRuntimeVersion": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
					},
				},
			},
			expected: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("0.4.1"),
						ProtocolVersion: &protocolV1,
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
		"DefaultRuntimeVersionAndProtocol": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("0.4.1"),
						ProtocolVersion: &protocolV1,
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
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
						RuntimeVersion:  proto.String("0.4.1"),
					},
				},
			},
			expected: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion:  proto.String("0.4.1"),
						ProtocolVersion: &protocolV1,
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
			scenario.spec.PyTorch.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateTorchServeModelServingContainer(t *testing.T) {
	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

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
			PyTorch: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.2.0",
				MultiModelServer:    false,
			},
		},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
		"ContainerSpecWithoutRuntime": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
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
				Image:     "someOtherImage:0.2.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V1ServiceEnvelope,
					},
				},
			},
		},
		"ContainerSpecWithDefaultImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://someUri"),
								RuntimeVersion:  proto.String("latest"),
								ProtocolVersion: &protocolV1,
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "someOtherImage:latest",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V1ServiceEnvelope,
					},
				},
			},
		},
		"ContainerSpecWithv1Protocol": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://someUri"),
								RuntimeVersion:  proto.String("latest"),
								ProtocolVersion: &protocolV1,
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "someOtherImage:latest",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V1ServiceEnvelope,
					},
				},
			},
		},
		"ContainerSpecWithv2Protocol": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("gs://someUri"),
								RuntimeVersion:  proto.String("latest"),
								ProtocolVersion: &protocolV2,
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "someOtherImage:latest",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V2ServiceEnvelope,
					},
				},
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "customImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V1ServiceEnvelope,
					},
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pytorch",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						PyTorch: &TorchServeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "someOtherImage:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"torchserve",
					"--start",
					"--model-store=/mnt/models/model-store",
					"--ts-config=/mnt/models/config/config.properties",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "TS_SERVICE_ENVELOPE",
						Value: V1ServiceEnvelope,
					},
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

func TestTorchServeIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	multiModelServerCases := [2]bool{true, false}

	for _, mmsCase := range multiModelServerCases {
		config := InferenceServicesConfig{
			Predictors: PredictorsConfig{
				PyTorch: PredictorConfig{
					ContainerImage:      "pytorch/torchserve-kfs",
					DefaultImageVersion: "0.4.1",
					MultiModelServer:    mmsCase,
				},
			},
		}

		protocolV1 := constants.ProtocolV1
		protocolV2 := constants.ProtocolV2

		defaultResource = v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("2Gi"),
			v1.ResourceCPU:    resource.MustParse("1"),
		}
		scenarios := map[string]struct {
			spec     PredictorSpec
			expected bool
		}{
			"DefaultRuntimeVersion": {
				spec: PredictorSpec{
					PyTorch: &TorchServeSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV1,
						},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol": {
				spec: PredictorSpec{
					PyTorch: &TorchServeSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{},
					},
				},
				expected: mmsCase,
			},
			"DefaultRuntimeVersionAndProtocol2": {
				spec: PredictorSpec{
					PyTorch: &TorchServeSpec{
						PredictorExtensionSpec: PredictorExtensionSpec{
							ProtocolVersion: &protocolV2,
						},
					},
				},
				expected: mmsCase,
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				scenario.spec.PyTorch.Default(&config)
				res := scenario.spec.PyTorch.IsMMS(&config)
				if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
					t.Errorf("got %t, want %t", res, scenario.expected)
				}
			})
		}
	}
}

func TestTorchServeIsFrameworkSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	pytorch := "pytorch"
	unsupportedFramework := "framework"
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{
			PyTorch: PredictorConfig{
				ContainerImage:      "pytorch/torchserve-kfs",
				DefaultImageVersion: "0.4.1",
				SupportedFrameworks: []string{pytorch},
			},
		},
	}

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec      PredictorSpec
		framework string
		expected  bool
	}{
		"SupportedFrameworkV1": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
					},
				},
			},
			framework: pytorch,
			expected:  true,
		},
		"SupportedFrameworkV2": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			framework: pytorch,
			expected:  true,
		},
		"UnsupportedFrameworkV1": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV1,
					},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
		"UnsupportedFrameworkV2": {
			spec: PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
					},
				},
			},
			framework: unsupportedFramework,
			expected:  false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.PyTorch.Default(&config)
			res := scenario.spec.PyTorch.IsFrameworkSupported(scenario.framework, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
