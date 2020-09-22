/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCustomPredictorValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PredictorSpec
		matcher types.GomegaMatcher
	}{
		"ValidStorageUri": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "s3://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidReplica": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas: GetIntReference(3),
					MaxReplicas: 2,
				},
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidContainerConcurrency": {
			spec: PredictorSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          GetIntReference(3),
					ContainerConcurrency: proto.Int64(-1),
				},
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
			res := customPredictor.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestCustomPredictorDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Predictors: PredictorsConfig{},
	}
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
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
						},
					},
				},
			},
			expected: PredictorSpec{
				PodSpec: PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  "STORAGE_URI",
									Value: "hdfs://modelzoo",
								},
							},
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
			customPredictor := NewCustomPredictor(&scenario.spec.PodSpec)
			customPredictor.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateCustomPredictorContainer(t *testing.T) {

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
		Predictors: PredictorsConfig{},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
		"ContainerSpecWithCustomImage": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "custom-predictor",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{
									Image: "custom-predictor:0.1.0",
									Args: []string{
										"--model_name",
										"someName",
										"--http_port",
										"8080",
									},
									Env: []v1.EnvVar{
										{
											Name:  "STORAGE_URI",
											Value: "hdfs://modelzoo",
										},
									},
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "custom-predictor:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--http_port",
					"8080",
				},
				Env: []v1.EnvVar{
					{
						Name:  "STORAGE_URI",
						Value: "hdfs://modelzoo",
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			predictor := scenario.isvc.Spec.Predictor.GetImplementation()
			predictor.Default(&config)
			res := predictor.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &scenario.isvc.Spec.Predictor.ComponentExtensionSpec, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}
