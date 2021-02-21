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

package v1alpha2

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInferenceServiceConversion(t *testing.T) {
	scenarios := map[string]struct {
		v1alpha2spec *InferenceService
		v1beta1Spec  *v1beta1.InferenceService
	}{
		"customPredictor": {
			v1alpha2spec: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-predictor",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Default: EndpointSpec{
						Predictor: PredictorSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Custom: &CustomSpec{
								Container: v1.Container{
									Name:  "kfserving-container",
									Image: "custom-predictor:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-predictor",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "kfserving-container",
									Image: "custom-predictor:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"customPredictorWithWorker": {
			v1alpha2spec: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-predictor",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Default: EndpointSpec{
						Predictor: PredictorSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
							},
							Custom: &CustomSpec{
								Container: v1.Container{
									Name:  "kfserving-container",
									Image: "custom-predictor:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
									Args: []string{
										"--workers",
										"1",
									},
								},
							},
						},
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-predictor",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: GetIntReference(1),
							MaxReplicas: 3,
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "kfserving-container",
									Image: "custom-predictor:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
									Args: []string{
										"--workers",
										"1",
									},
								},
							},
						},
					},
				},
			},
		},
		"transformer": {
			v1alpha2spec: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conversionTest",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Default: EndpointSpec{
						Predictor: PredictorSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Tensorflow: &TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Transformer: &TransformerSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Custom: &CustomSpec{
								Container: v1.Container{
									Name:  "kfserving-container",
									Image: "transformer:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conversionTest",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.13.0"),
							},
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "kfserving-container",
									Image: "transformer:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"explainer": {
			v1alpha2spec: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explainerConversionTest",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Default: EndpointSpec{
						Predictor: PredictorSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Tensorflow: &TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Explainer: &ExplainerSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Alibi: &AlibiExplainerSpec{
								Type:           AlibiAnchorsTabularExplainer,
								StorageURI:     "s3://test/mnist/explainer",
								RuntimeVersion: "0.4.0",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("2Gi"),
									},
									Limits: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explainerConversionTest",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.13.0"),
							},
						},
					},
					Explainer: &v1beta1.ExplainerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						Alibi: &v1beta1.AlibiExplainerSpec{
							Type:           v1beta1.AlibiAnchorsTabularExplainer,
							StorageURI:     "s3://test/mnist/explainer",
							RuntimeVersion: proto.String("0.4.0"),
							Container: v1.Container{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("2Gi"),
									},
									Limits: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			dst := &v1beta1.InferenceService{}
			scenario.v1alpha2spec.ConvertTo(dst)
			if cmp.Diff(scenario.v1beta1Spec, dst) != "" {
				t.Errorf("diff: %s", cmp.Diff(scenario.v1beta1Spec, dst))
			}
			v1alpha2ExpectedSpec := &InferenceService{}
			v1alpha2ExpectedSpec.ConvertFrom(scenario.v1beta1Spec)
			if cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec) != "" {
				t.Errorf("diff: %s", cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec))
			}
		})
	}
}

func TestInferenceServiceNilExplainerConversion(t *testing.T) {
	scenarios := map[string]struct {
		v1alpha2spec *InferenceService
		v1beta1Spec  *v1beta1.InferenceService
	}{
		"explainer": {
			v1alpha2spec: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explainerConversionTest",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Default: EndpointSpec{
						Predictor: PredictorSpec{
							DeploymentSpec: DeploymentSpec{
								MinReplicas: GetIntReference(1),
								MaxReplicas: 3,
								Parallelism: 1,
							},
							Tensorflow: &TensorflowSpec{
								StorageURI:     "s3://test/mnist/export",
								RuntimeVersion: "1.13.0",
							},
						},
						Explainer: nil,
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explainerConversionTest",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.13.0"),
							},
						},
					},
					Explainer: &v1beta1.ExplainerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						ART: &v1beta1.ARTExplainerSpec{
							Type: v1beta1.ARTSquareAttackExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								RuntimeVersion: proto.String("0.4.0"),
								Container: v1.Container{
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			v1alpha2ExpectedSpec := &InferenceService{}
			v1alpha2ExpectedSpec.ConvertFrom(scenario.v1beta1Spec)
			if cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec) != "" {
				t.Errorf("diff: %s", cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec))
			}
		})
	}
}
