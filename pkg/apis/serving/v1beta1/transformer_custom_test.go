package v1beta1

import (
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestTransformerValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    TransformerSpec
		matcher types.GomegaMatcher
	}{
		"ValidStorageUri": {
			spec: TransformerSpec{
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: TransformerSpec{
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidReplica": {
			spec: TransformerSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas: GetIntReference(3),
					MaxReplicas: 2,
				},
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidContainerConcurrency": {
			spec: TransformerSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          GetIntReference(3),
					ContainerConcurrency: proto.Int64(-1),
				},
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestTransformerDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Transformers: TransformersConfig{},
	}
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	scenarios := map[string]struct {
		spec     TransformerSpec
		expected TransformerSpec
	}{
		"DefaultResources": {
			spec: TransformerSpec{
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
				},
			},
			expected: TransformerSpec{
				CustomTransformer: &CustomTransformer{
					PodTemplateSpec: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
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
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.CustomTransformer.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %q, want %q", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestCreateTransformerContainer(t *testing.T) {

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
		Transformers: TransformersConfig{},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc                  InferenceService
		expectedContainerSpec *v1.Container
	}{
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
					Transformer: &TransformerSpec{
						CustomTransformer: &CustomTransformer{
							PodTemplateSpec: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "transformer:0.1.0",
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
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "transformer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--predictor_host",
					"someName-predictor-default.default",
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
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Transformer: &TransformerSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ContainerConcurrency: proto.Int64(2),
						},
						CustomTransformer: &CustomTransformer{
							PodTemplateSpec: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "transformer:0.1.0",
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
				},
			},
			expectedContainerSpec: &v1.Container{
				Image:     "transformer:0.1.0",
				Name:      constants.InferenceServiceContainerName,
				Resources: requestedResource,
				Args: []string{
					"--model_name",
					"someName",
					"--predictor_host",
					"someName-predictor-default.default",
					"--http_port",
					"8080",
					"--workers",
					"2",
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
			transformer := scenario.isvc.Spec.Transformer.GetTransformer()[0]
			transformer.Default(&config)
			res := transformer.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, scenario.isvc.Spec.Transformer.ContainerConcurrency, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}
