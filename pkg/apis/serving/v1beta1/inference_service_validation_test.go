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
	"context"
	"fmt"
	"testing"

	"github.com/kserve/kserve/pkg/constants"

	"google.golang.org/protobuf/proto"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func makeTestRawInferenceService() InferenceService {
	inferenceservice := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			Annotations: map[string]string{
				"serving.kserve.io/deploymentMode":              "RawDeployment",
				"serving.kserve.io/autoscalerClass":             "hpa",
				"serving.kserve.io/metrics":                     "cpu",
				"serving.kserve.io/targetUtilizationPercentage": "75",
			},
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI:     proto.String("gs://testbucket/testmodel"),
						RuntimeVersion: proto.String("0.14.0"),
					},
				},
			},
		},
	}
	return inferenceservice
}

func makeTestInferenceService() InferenceService {
	inferenceservice := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI:     proto.String("gs://testbucket/testmodel"),
						RuntimeVersion: proto.String("0.14.0"),
					},
				},
			},
		},
	}
	return inferenceservice
}

func TestValidAutoscalerClassTypeAndHPAMetrics(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())
}

func TestInvalidAutoscalerClassType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/autoscalerClass"] = "test"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestValidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "70"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestInvalidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "101"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "abc"
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "0"
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "99.9"
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestInvalidAutoscalerHPAMetrics(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/metrics"] = "test"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.XGBoost = &XGBoostSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestCustomizeDeploymentStrategyUnsupportedForServerless(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.PodSpec = PodSpec{ServiceAccountName: "test"}
	isvc.Spec.Predictor.DeploymentStrategy = &appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError("customizing deploymentStrategy is only supported for raw deployment mode"))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestModelSpecAndCustomOverridesIsValid(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.PodSpec = PodSpec{ServiceAccountName: "test"}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow = nil
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadParallelismValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.ContainerConcurrency = proto.Int64(-1)
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ParallelismLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(-1))
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Predictor.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Predictor.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	// Now test transformer and explainer, so set correct value for predictor
	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(0))
	isvc.Spec.Predictor.MaxReplicas = 0

	isvc.Spec.Transformer = &TransformerSpec{}
	isvc.Spec.Transformer.PodSpec = PodSpec{
		Containers: []corev1.Container{
			{
				Image: "some-image",
			},
		},
	}
	isvc.Spec.Transformer.MinReplicas = ptr.To(int32(-1))
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Transformer.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Transformer.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	// Now test explainer, so ignore transformer
	isvc.Spec.Transformer = nil

	isvc.Spec.Explainer = &ExplainerSpec{
		ART: &ARTExplainerSpec{
			ExplainerExtensionSpec: ExplainerExtensionSpec{
				StorageURI: "gs://testbucket/testmodel",
			},
		},
	}
	isvc.Spec.Explainer.MinReplicas = ptr.To(int32(-1))
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Explainer.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Explainer.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestCustomOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow = nil
	isvc.Spec.Predictor.PodSpec = PodSpec{
		Containers: []corev1.Container{
			{
				Name:  constants.InferenceServiceContainerName,
				Image: "some-image",
			},
		},
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.PodSpec = PodSpec{
		Containers: []corev1.Container{
			{
				Image: "some-image",
			},
		},
	}
	validator = InferenceServiceValidator{}
	warnings, err = validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Transformer = &TransformerSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Transformer)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Explainer)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestGoodExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{
		ART: &ARTExplainerSpec{
			ExplainerExtensionSpec: ExplainerExtensionSpec{
				StorageURI: "gs://testbucket/testmodel",
			},
		},
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc-123"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "1abcde"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc.de"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestValidateTwoPredictorImplementationCollocation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Model: &ModelSpec{
					ModelFormat: ModelFormat{
						Name: "huggingface",
					},
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "test/predictor:latest",
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "test/transformer:latest",
						},
					},
				},
			},
		},
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestValidateCollocationStorageURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc       *InferenceService
		errMatcher gomega.OmegaMatcher
	}{
		"Collocation with transformer has storage uri specified": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "test/predictor:latest",
									Env: []corev1.EnvVar{
										{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "gs://test/model",
										},
									},
								},
								{
									Name:  constants.TransformerContainerName,
									Image: "test/transformer:latest",
									Env: []corev1.EnvVar{
										{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "gs://test/model",
										},
									},
								},
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError(StorageUriPresentInTransformerError),
		},
		"Collocation with no storage uri specified": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "test/predictor:latest",
								},
								{
									Name:  constants.TransformerContainerName,
									Image: "test/transformer:latest",
								},
							},
						},
					},
				},
			},
			errMatcher: gomega.BeNil(),
		},
		"Collocation with predictor has storage uri specified": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "test/predictor:latest",
									Env: []corev1.EnvVar{
										{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "gs://test/model",
										},
									},
								},
								{
									Name:  constants.TransformerContainerName,
									Image: "test/transformer:latest",
								},
							},
						},
					},
				},
			},
			errMatcher: gomega.BeNil(),
		},
		"Predictor with no collocation": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.BeNil(),
		},
		"Custom predictor with no collocation": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "test/predictor:latest",
									Env: []corev1.EnvVar{
										{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "gs://test/model",
										},
									},
								},
							},
						},
					},
				},
			},
			errMatcher: gomega.BeNil(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := validateCollocationStorageURI(scenario.isvc.Spec.Predictor)
			g.Expect(err).Should(scenario.errMatcher)
		})
	}
}

func TestValidateMultiNodeVariables(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	s3StorageUri := "s3://test"
	pvcStorageUri := "pvc://test"
	scenarios := map[string]struct {
		isvc     *InferenceService
		expected gomega.OmegaMatcher
	}{
		"When TENSOR_PARALLEL_SIZE set in the environment, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
								Container: corev1.Container{
									Env: []corev1.EnvVar{
										{Name: constants.TensorParallelSizeEnvName, Value: "2"},
									},
								},
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(DisallowedWorkerSpecTensorParallelSizeEnvError, "foo-1")),
		},
		"When PIPELINE_PARALLEL_SIZE set in the environment, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-2",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
								Container: corev1.Container{
									Env: []corev1.EnvVar{
										{Name: constants.PipelineParallelSizeEnvName, Value: "3"},
									},
								},
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(DisallowedWorkerSpecPipelineParallelSizeEnvError, "foo-2")),
		},
		"When workerSpec.TensorParallelSize set less than 1, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-3",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec:            PodSpec{},
							TensorParallelSize: intPtr(0),
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecTensorParallelSizeValueError, "foo-3", "0")),
		},
		"When WorkerSpec.PipelineParallelSize set less than 2, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-4",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec:              PodSpec{},
							PipelineParallelSize: intPtr(1),
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecPipelineParallelSizeValueError, "foo-4", "1")),
		},
		"When unknownGPUResource set in Predictor.Model, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-5",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
								Container: corev1.Container{
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											"unknownGPU.com/gpu": resource.MustParse("1"),
										},
										Requests: corev1.ResourceList{
											"unknownGPU.com/gpu": resource.MustParse("1"),
										},
									},
								},
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidUnknownGPUTypeError, "foo-5")),
		},
		"When unknownGPUResource set in Predictor.WorkerSpec, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-6",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec: PodSpec{
								Containers: []corev1.Container{
									{
										Resources: corev1.ResourceRequirements{
											Limits: corev1.ResourceList{
												"unknownGPU.com/gpu": resource.MustParse("1"),
											},
											Requests: corev1.ResourceList{
												"unknownGPU.com/gpu": resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidUnknownGPUTypeError, "foo-6")),
		},
		"When unsupported storageURI set, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-7",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &s3StorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidNotSupportedStorageURIProtocolError, "foo-7", "s3")),
		},
		"When external autoscaler is not set, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-8",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassHPA),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidAutoScalerError, "foo-8", constants.AutoscalerClassHPA)),
		},
		"When multiple containers set in WorkerSpec, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-9",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &pvcStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec: PodSpec{
								Containers: []corev1.Container{
									{},
									{},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(DisallowedMultipleContainersInWorkerSpecError, "foo-9")),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := validateMultiNodeVariables(scenario.isvc)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestDeploymentModeUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	oldIsvc := makeTestInferenceService()
	oldIsvc.Status = InferenceServiceStatus{
		DeploymentMode: "Serverless",
	}
	updatedIsvc := oldIsvc.DeepCopy()
	updatedIsvc.Annotations = map[string]string{
		constants.DeploymentMode: "RawDeployment",
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateUpdate(context.Background(), &oldIsvc, updatedIsvc)
	// Annotation does not match status, update should be rejected
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).ShouldNot(gomega.Succeed())

	updatedIsvc1 := oldIsvc.DeepCopy()
	updatedIsvc1.Annotations = map[string]string{
		constants.DeploymentMode: "Serverless",
	}
	warnings, err = validator.ValidateUpdate(context.Background(), &oldIsvc, updatedIsvc1)
	// Annotation matches status, update is accepted
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())
}

func intPtr(i int) *int {
	return &i
}
