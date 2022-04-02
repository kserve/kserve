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

func TestAlibiValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
	}
	scenarios := map[string]struct {
		spec    ExplainerSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type: "AnchorTabular",
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidStorageUri": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type: "AnchorTabular",
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						StorageURI: "s3://modelzoo",
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						StorageURI: "hdfs://modelzoo",
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Alibi.Default(&config)
			res := scenario.spec.Alibi.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestAlibiDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
	}
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     ExplainerSpec
		expected ExplainerSpec
	}{
		"DefaultRuntimeVersion": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{},
			},
			expected: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						RuntimeVersion: proto.String("v0.4.0"),
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
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
					},
				},
			},
			expected: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						RuntimeVersion: proto.String("v0.3.0"),
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
			scenario.spec.Alibi.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateAlibiModelServingContainer(t *testing.T) {

	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "100",
			},
			"memory": resource.MustParse("1Gi"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.Quantity{
				Format: "90",
			},
			"memory": resource.MustParse("1Gi"),
		},
	}
	var config = InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "0.4.0",
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
					Name:      "sklearn",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						Alibi: &AlibiExplainerSpec{
							Type: AlibiAnchorsTabularExplainer,
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI: "s3://explainer",
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "alibi:0.4.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--http_port",
					"8080",
					"--predictor_host",
					fmt.Sprintf("%s.%s", constants.DefaultPredictorServiceName("someName"), "default"),
					"--storage_uri",
					"/mnt/models",
					"AnchorTabular",
				},
			},
		},
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						Alibi: &AlibiExplainerSpec{
							Type: AlibiAnchorsTabularExplainer,
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI:     "s3://explainer",
								RuntimeVersion: proto.String("v0.4.0"),
								Container: v1.Container{
									Image:     "explainer:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "explainer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--http_port",
					"8080",
					"--predictor_host",
					fmt.Sprintf("%s.%s", constants.DefaultPredictorServiceName("someName"), "default"),
					"--storage_uri",
					"/mnt/models",
					"AnchorTabular",
				},
			},
		},
		"ContainerSpecWithContainerConcurrency": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(1),
						},
						SKLearn: &SKLearnSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://someUri"),
								RuntimeVersion: proto.String("0.1.0"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(2),
						},
						Alibi: &AlibiExplainerSpec{
							Type: AlibiAnchorsTabularExplainer,
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								StorageURI:     "s3://explainer",
								RuntimeVersion: proto.String("v0.4.0"),
								Container: v1.Container{
									Image:     "explainer:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "explainer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--http_port",
					"8080",
					"--predictor_host",
					fmt.Sprintf("%s.%s", constants.DefaultPredictorServiceName("someName"), "default"),
					"--workers",
					"2",
					"--storage_uri",
					"/mnt/models",
					"AnchorTabular",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			explainer := scenario.isvc.Spec.Explainer.GetImplementation()
			explainer.Default(&config)
			res := explainer.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &scenario.isvc.Spec.Explainer.ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}

func TestAlibiIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
	}

	// MMS is not supported by explainer
	mssCase := false
	scenarios := map[string]struct {
		spec     ExplainerSpec
		expected bool
	}{
		"AcceptGoodRuntimeVersion": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type: "AnchorTabular",
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						RuntimeVersion: proto.String("latest"),
					},
				},
			},
			expected: mssCase,
		},
		"ValidStorageUri": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type: "AnchorTabular",
					ExplainerExtensionSpec: ExplainerExtensionSpec{
						StorageURI: "s3://modelzoo",
					},
				},
			},
			expected: mssCase,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Alibi.Default(&config)
			res := scenario.spec.Alibi.IsMMS(&config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
