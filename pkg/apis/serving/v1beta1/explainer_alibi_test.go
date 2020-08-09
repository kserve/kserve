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

func TestAlibiValidation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    ExplainerSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type:           "AnchorTabular",
					RuntimeVersion: "latest",
				},
			},
			matcher: gomega.BeNil(),
		},
		"ValidStorageUri": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					Type:       "AnchorTabular",
					StorageURI: "s3://modelzoo",
				},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidStorageUri": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					StorageURI: "hdfs://modelzoo",
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidReplica": {
			spec: ExplainerSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas: GetIntReference(3),
					MaxReplicas: 2,
				},
				Alibi: &AlibiExplainerSpec{
					StorageURI: "hdfs://modelzoo",
				},
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidContainerConcurrency": {
			spec: ExplainerSpec{
				ComponentExtensionSpec: ComponentExtensionSpec{
					MinReplicas:          GetIntReference(3),
					ContainerConcurrency: proto.Int64(-1),
				},
				Alibi: &AlibiExplainerSpec{
					StorageURI: "hdfs://modelzoo",
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

func TestAlibiDefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: &ExplainersConfig{
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
					RuntimeVersion: "v0.4.0",
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
		"DefaultResources": {
			spec: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					RuntimeVersion: "v0.3.0",
				},
			},
			expected: ExplainerSpec{
				Alibi: &AlibiExplainerSpec{
					RuntimeVersion: "v0.3.0",
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
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Alibi.Default(&config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %q, want %q", scenario.spec, scenario.expected)
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
		Explainers: &ExplainersConfig{
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
								RuntimeVersion: "0.1.0",
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						Alibi: &AlibiExplainerSpec{
							Type:       AlibiAnchorsTabularExplainer,
							StorageURI: "s3://explainer",
							Container: v1.Container{
								Resources: requestedResource,
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
					"--predictor_host",
					"someName-predictor-default.default",
					"--http_port",
					"8080",
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
							Type:           AlibiAnchorsTabularExplainer,
							StorageURI:     "s3://explainer",
							RuntimeVersion: "v0.4.0",
							Container: v1.Container{
								Image:     "explainer:0.1.0",
								Resources: requestedResource,
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
					"--predictor_host",
					"someName-predictor-default.default",
					"--http_port",
					"8080",
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
								RuntimeVersion: "0.1.0",
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
					Explainer: &ExplainerSpec{
						Alibi: &AlibiExplainerSpec{
							Type:           AlibiAnchorsTabularExplainer,
							StorageURI:     "s3://explainer",
							RuntimeVersion: "v0.4.0",
							Container: v1.Container{
								Image:     "explainer:0.1.0",
								Resources: requestedResource,
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
					"--predictor_host",
					"someName-predictor-default.default",
					"--http_port",
					"8080",
					"--workers",
					"1",
					"--storage_uri",
					"/mnt/models",
					"AnchorTabular",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			explainer, _ := scenario.isvc.Spec.Explainer.GetExplainer()
			explainer.Default(&config)
			res := explainer.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, scenario.isvc.Spec.Predictor.ContainerConcurrency, &config)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedContainerSpec)) {
				t.Errorf("got %q, want %q", res, scenario.expectedContainerSpec)
			}
		})
	}
}
