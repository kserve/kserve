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
	"strings"
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

func TestInvalidNameInSKLearnPredictor(t *testing.T) {
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-isvc",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						Container: corev1.Container{
							Name:  "invalid-name",
							Image: "dummy-image",
						},
						StorageURI: proto.String("gs://kfserving-examples/models/sklearn/1.0/model"),
					},
				},
			},
		},
	}
	err := validatePredictor(&isvc)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("Expected error for name field in SKLearn predictor, got: %v", err)
	}
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

func TestAutoscalerClassHPA(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc       *InferenceService
		errMatcher gomega.OmegaMatcher
	}{
		"Valid HPA CPU metrics with ScaleMetric": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ScaleMetric: ptr.To(MetricCPU),
							ScaleTarget: ptr.To(int32(80)),
						},
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
		"Valid HPA CPU metrics with Autoscaling spec": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
										Resource: &ResourceMetricSource{
											Name: ResourceMetricCPU,
											Target: MetricTarget{
												AverageUtilization: ptr.To(int32(80)),
											},
										},
									},
								},
							},
						},
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
		"Valid HPA Memory metrics with Autoscaling spec": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
										Resource: &ResourceMetricSource{
											Name: ResourceMetricMemory,
											Target: MetricTarget{
												AverageValue: NewMetricQuantity("1Gi"),
											},
										},
									},
								},
							},
						},
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
		"Invalid HPA CPU metrics without resource type": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
									},
								},
							},
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError("metricSpec.Resource is not set for resource metric source type"),
		},
		"Invalid HPA CPU metrics with wrong metric source": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ExternalMetricSourceType,
									},
								},
							},
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError("invalid HPA metric source type with value [External],valid metric source types are Resource"),
		},
		"Valid HPA CPU metrics with target utilization": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ScaleMetric:     ptr.To(MetricCPU),
							ScaleMetricType: ptr.To(UtilizationMetricType),
							ScaleTarget:     ptr.To(int32(80)),
						},
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
		"HPA CPU metrics with invalid target utilization": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ScaleMetric:     ptr.To(MetricCPU),
							ScaleMetricType: ptr.To(UtilizationMetricType),
							ScaleTarget:     ptr.To(int32(120)),
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError("the target utilization percentage should be a [1-100] integer"),
		},
		"Valid HPA Memory metrics with ScaleMetric": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "hpa",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ScaleMetric: ptr.To(MetricMemory),
							ScaleTarget: ptr.To(int32(10)),
						},
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
		"Invalid autoscaler class": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "test",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							ScaleMetric: ptr.To(MetricMemory),
							ScaleTarget: ptr.To(int32(10)),
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError("[test] is not a supported autoscaler class type"),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			validator := InferenceServiceValidator{}
			_, err := validator.ValidateCreate(t.Context(), scenario.isvc)
			g.Expect(err).Should(scenario.errMatcher)
		})
	}
}

func TestAutoscalerClassKEDA(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc       *InferenceService
		errMatcher gomega.OmegaMatcher
	}{
		"Valid KEDA CPU metrics with Autoscaling spec": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "keda",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
										Resource: &ResourceMetricSource{
											Name: ResourceMetricCPU,
											Target: MetricTarget{
												Type:               UtilizationMetricType,
												AverageUtilization: ptr.To(int32(80)),
											},
										},
									},
								},
							},
						},
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
		"Valid KEDA Memory metrics with Autoscaling spec": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "keda",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
										Resource: &ResourceMetricSource{
											Name: ResourceMetricMemory,
											Target: MetricTarget{
												Type:         AverageValueMetricType,
												AverageValue: NewMetricQuantity("1Gi"),
											},
										},
									},
								},
							},
						},
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
		"Invalid KEDA CPU metrics without resource type": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "keda",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ResourceMetricSourceType,
									},
								},
							},
						},
						Tensorflow: &TFServingSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:     proto.String("gs://testbucket/testmodel"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			},
			errMatcher: gomega.MatchError("metricSpec.Resource is not set for resource metric source type"),
		},
		"Valid KEDA CPU metrics with external metric source": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "keda",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: ExternalMetricSourceType,
										External: &ExternalMetricSource{
											Metric: ExternalMetrics{
												Backend: PrometheusBackend,
												Query:   "avg(vllm_requests_running)",
											},
											Target: MetricTarget{
												Type:  ValueMetricType,
												Value: NewMetricQuantity("10"),
											},
										},
									},
								},
							},
						},
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
		"Valid KEDA CPU metrics with pod metric source": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "Standard",
						"serving.kserve.io/autoscalerClass": "keda",
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						ComponentExtensionSpec: ComponentExtensionSpec{
							AutoScaling: &AutoScalingSpec{
								Metrics: []MetricsSpec{
									{
										Type: PodMetricSourceType,
										PodMetric: &PodMetricSource{
											Metric: PodMetrics{
												Backend: OpenTelemetryBackend,
												Query:   "avg(vllm_requests_running)",
											},
											Target: MetricTarget{
												Type:  ValueMetricType,
												Value: NewMetricQuantity("10"),
											},
										},
									},
								},
							},
						},
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
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			validator := InferenceServiceValidator{}
			_, err := validator.ValidateCreate(t.Context(), scenario.isvc)
			g.Expect(err).Should(scenario.errMatcher)
		})
	}
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.XGBoost = &XGBoostSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError("customizing deploymentStrategy is only supported for raw deployment mode"))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestModelSpecAndCustomOverridesIsValid(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.PodSpec = PodSpec{ServiceAccountName: "test"}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow = nil
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadParallelismValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.ContainerConcurrency = proto.Int64(-1)
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ParallelismLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(-1))
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Predictor.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Predictor.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Transformer.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Transformer.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = ptr.To(int32(1))
	isvc.Spec.Explainer.MaxReplicas = -1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = ptr.To(int32(2))
	isvc.Spec.Explainer.MaxReplicas = 1
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err = validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Transformer = &TransformerSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Transformer)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc-123"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "1abcde"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc.de"
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
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
	warnings, err := validator.ValidateCreate(t.Context(), &isvc)
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
	ociStorageUri := "oci://test"
	scenarios := map[string]struct {
		isvc     *InferenceService
		expected gomega.OmegaMatcher
	}{
		"When TENSOR_PARALLEL_SIZE set as an environment variable, it should return an error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1-1",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(DisallowedWorkerSpecTensorParallelSizeEnvError, "foo-1-1")),
		},
		"When PIPELINE_PARALLEL_SIZE set as an environment variable, it should return an error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1-2",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(DisallowedWorkerSpecPipelineParallelSizeEnvError, "foo-1-2")),
		},
		"When workerSpec.TensorParallelSize set less than 1, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-2-1",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecTensorParallelSizeValueError, "foo-2-1", "0")),
		},
		"When WorkerSpec.PipelineParallelSize set less than 1, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-2-2",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
							PipelineParallelSize: intPtr(0),
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecPipelineParallelSizeValueError, "foo-2-2", "0")),
		},
		"When unknownGPUResource set in Predictor.Model, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-3-1",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(InvalidUnknownGPUTypeError, "foo-3-1")),
		},
		"When unknownGPUResource set in Predictor.WorkerSpec, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-3-2",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(InvalidUnknownGPUTypeError, "foo-3-2")),
		},
		"When customGPUResourceTypes set to annotations, unknownGPUResource in Predictor.Model do not return an error and function correctly": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-3-3",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass:                     string(constants.AutoscalerClassNone),
						constants.CustomGPUResourceTypesAnnotationKey: "[\"unknownGPU.com/gpu\"]",
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
			expected: gomega.BeNil(),
		},
		"When customGPUResourceTypes set to annotations, unknownGPUResource in Predictor.WorkerSpec do not return an error and function correctly": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-3-4",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass:                     string(constants.AutoscalerClassNone),
						constants.CustomGPUResourceTypesAnnotationKey: "[\"unknownGPU.com/gpu\"]",
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
			expected: gomega.BeNil(),
		},
		"When unsupported storageURI set, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-4-1",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
			expected: gomega.Equal(fmt.Errorf(InvalidNotSupportedStorageURIProtocolError, "foo-4-1", "s3")),
		},
		"When using OCI storageURI set, validation should succeed": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-4-3",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: &ociStorageUri,
							},
						},
						WorkerSpec: &WorkerSpec{},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When none autoscaler is not set, then it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-4-2",
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
			expected: gomega.Equal(fmt.Errorf(InvalidAutoScalerError, "foo-4-2", constants.AutoscalerClassHPA)),
		},
		"When multiple containers set in WorkerSpec, it should return error": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-9",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
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
		DeploymentMode: string(constants.Knative),
	}
	updatedIsvc := oldIsvc.DeepCopy()
	updatedIsvc.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Standard),
	}
	validator := InferenceServiceValidator{}
	warnings, err := validator.ValidateUpdate(t.Context(), &oldIsvc, updatedIsvc)
	// Annotation does not match status, update should be rejected
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).ShouldNot(gomega.Succeed())

	updatedIsvc1 := oldIsvc.DeepCopy()
	updatedIsvc1.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Knative),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvc, updatedIsvc1)
	// Annotation matches status, update is accepted
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Legacy status "RawDeployment" should accept normalized annotation "Standard"
	oldIsvcLegacy := makeTestInferenceService()
	oldIsvcLegacy.Status = InferenceServiceStatus{
		DeploymentMode: string(constants.LegacyRawDeployment),
	}
	updatedIsvcLegacy := oldIsvcLegacy.DeepCopy()
	updatedIsvcLegacy.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Standard),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcLegacy, updatedIsvcLegacy)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Legacy status "Serverless" should accept normalized annotation "Knative"
	oldIsvcLegacySl := makeTestInferenceService()
	oldIsvcLegacySl.Status = InferenceServiceStatus{
		DeploymentMode: string(constants.LegacyServerless),
	}
	updatedIsvcLegacySl := oldIsvcLegacySl.DeepCopy()
	updatedIsvcLegacySl.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Knative),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcLegacySl, updatedIsvcLegacySl)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Legacy status "RawDeployment" with different mode "Knative" should still be rejected
	oldIsvcLegacyReject := makeTestInferenceService()
	oldIsvcLegacyReject.Status = InferenceServiceStatus{
		DeploymentMode: string(constants.LegacyRawDeployment),
	}
	updatedIsvcLegacyReject := oldIsvcLegacyReject.DeepCopy()
	updatedIsvcLegacyReject.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Knative),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcLegacyReject, updatedIsvcLegacyReject)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).ShouldNot(gomega.Succeed())

	// Test: Mismatched deploymentMode should be allowed during deletion (DeletionTimestamp set)
	// This allows finalizer cleanup when annotation differs from status
	deletingIsvc := oldIsvc.DeepCopy()
	deletingIsvc.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Standard), // Mismatches status (Knative)
	}
	now := metav1.Now()
	deletingIsvc.DeletionTimestamp = &now
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvc, deletingIsvc)
	// During deletion, deploymentMode mismatch should be allowed
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Empty Status.DeploymentMode with a non-Standard annotation should be accepted.
	// This is the first-reconcile finalizer patch case: the controller adds a finalizer via
	// r.Patch(...) before updateStatus runs, so oldIsvc.Status.DeploymentMode is "". The
	// validator must treat that as "no prior state" and not synthesize "Standard" from it.
	// Regression test for issue #5793.
	oldIsvcEmptyStatus := makeTestInferenceService()
	oldIsvcEmptyStatus.Status = InferenceServiceStatus{
		DeploymentMode: "",
	}
	updatedIsvcKnativeAnnotation := oldIsvcEmptyStatus.DeepCopy()
	updatedIsvcKnativeAnnotation.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Knative),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcEmptyStatus, updatedIsvcKnativeAnnotation)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Empty Status.DeploymentMode with a legacy Serverless annotation should also be accepted.
	updatedIsvcLegacySlAnnotation := oldIsvcEmptyStatus.DeepCopy()
	updatedIsvcLegacySlAnnotation.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.LegacyServerless),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcEmptyStatus, updatedIsvcLegacySlAnnotation)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Empty Status.DeploymentMode with a Standard annotation should also be accepted.
	updatedIsvcStandardAnnotation := oldIsvcEmptyStatus.DeepCopy()
	updatedIsvcStandardAnnotation.Annotations = map[string]string{
		constants.DeploymentMode: string(constants.Standard),
	}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcEmptyStatus, updatedIsvcStandardAnnotation)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())

	// Test: Empty Status.DeploymentMode with no deploymentMode annotation should be accepted.
	updatedIsvcNoAnnotation := oldIsvcEmptyStatus.DeepCopy()
	updatedIsvcNoAnnotation.Annotations = map[string]string{}
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvcEmptyStatus, updatedIsvcNoAnnotation)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())
}

func TestValidateUpdateDuringDeletion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validator := InferenceServiceValidator{}
	pvcStorageUri := "pvc://my-pvc/model"

	oldIsvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multinode-isvc",
			Namespace: "default",
			Annotations: map[string]string{
				constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
				constants.DeploymentMode:  string(constants.LegacyRawDeployment),
			},
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Model: &ModelSpec{
					ModelFormat: ModelFormat{Name: "huggingface"},
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: &pvcStorageUri,
					},
				},
				WorkerSpec: &WorkerSpec{},
			},
		},
	}

	// Without DeletionTimestamp, this ISVC should be rejected (autoscalerClass: external is invalid for multinode)
	warnings, err := validator.ValidateUpdate(t.Context(), &oldIsvc, oldIsvc.DeepCopy())
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).ShouldNot(gomega.Succeed())

	// With DeletionTimestamp set, the same ISVC should be accepted so finalizers can be removed
	deletingIsvc := oldIsvc.DeepCopy()
	now := metav1.Now()
	deletingIsvc.DeletionTimestamp = &now
	warnings, err = validator.ValidateUpdate(t.Context(), &oldIsvc, deletingIsvc)
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())
}

func TestValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validator := InferenceServiceValidator{}

	t.Run("Valid InferenceService object", func(t *testing.T) {
		isvc := makeTestInferenceService()
		warnings, err := validator.ValidateDelete(t.Context(), &isvc)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(warnings).Should(gomega.BeEmpty())
	})

	t.Run("Invalid object type", func(t *testing.T) {
		// Use a valid runtime.Object type but not an InferenceService
		notIsvc := &corev1.Pod{}
		warnings, err := validator.ValidateDelete(t.Context(), notIsvc)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(warnings).Should(gomega.BeEmpty())
	})
}

func TestValidateScalingKedaCompExtension(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	validCPU := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: ResourceMetricCPU,
						Target: MetricTarget{
							Type:               UtilizationMetricType,
							AverageUtilization: ptr.To(int32(80)),
						},
					},
				},
			},
		},
	}
	validMemory := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: ResourceMetricMemory,
						Target: MetricTarget{
							Type:         AverageValueMetricType,
							AverageValue: NewMetricQuantity("2Gi"),
						},
					},
				},
			},
		},
	}
	invalidScaleMetric := &ComponentExtensionSpec{
		ScaleMetric: ptr.To(MetricCPU),
	}
	missingResource := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
				},
			},
		},
	}
	invalidCPUType := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: ResourceMetricCPU,
						Target: MetricTarget{
							Type: AverageValueMetricType,
						},
					},
				},
			},
		},
	}
	invalidMemoryType := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: ResourceMetricMemory,
						Target: MetricTarget{
							Type: ValueMetricType,
						},
					},
				},
			},
		},
	}
	invalidMemoryValue := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: ResourceMetricMemory,
						Target: MetricTarget{
							Type:         AverageValueMetricType,
							AverageValue: NewMetricQuantity("512Ki"),
						},
					},
				},
			},
		},
	}
	unsupportedResource := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ResourceMetricSourceType,
					Resource: &ResourceMetricSource{
						Name: "disk",
						Target: MetricTarget{
							Type: UtilizationMetricType,
						},
					},
				},
			},
		},
	}
	invalidExternal := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ExternalMetricSourceType,
					External: &ExternalMetricSource{
						Metric: ExternalMetrics{
							Backend: PrometheusBackend,
							Query:   "",
						},
						Target: MetricTarget{
							Type:  ValueMetricType,
							Value: nil,
						},
					},
				},
			},
		},
	}
	validExternal := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: ExternalMetricSourceType,
					External: &ExternalMetricSource{
						Metric: ExternalMetrics{
							Backend: PrometheusBackend,
							Query:   "avg(requests)",
						},
						Target: MetricTarget{
							Type:  ValueMetricType,
							Value: NewMetricQuantity("10"),
						},
					},
				},
			},
		},
	}
	invalidPodMetric := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: PodMetricSourceType,
					PodMetric: &PodMetricSource{
						Metric: PodMetrics{
							Backend: OpenTelemetryBackend,
							Query:   "",
						},
						Target: MetricTarget{
							Type:  ValueMetricType,
							Value: nil,
						},
					},
				},
			},
		},
	}
	validPodMetric := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: PodMetricSourceType,
					PodMetric: &PodMetricSource{
						Metric: PodMetrics{
							Backend: OpenTelemetryBackend,
							Query:   "avg(requests)",
						},
						Target: MetricTarget{
							Type:  ValueMetricType,
							Value: NewMetricQuantity("5"),
						},
					},
				},
			},
		},
	}
	unknownMetricType := &ComponentExtensionSpec{
		AutoScaling: &AutoScalingSpec{
			Metrics: []MetricsSpec{
				{
					Type: "UnknownType",
				},
			},
		},
	}

	tests := []struct {
		name    string
		spec    *ComponentExtensionSpec
		wantErr string
	}{
		{"valid cpu", validCPU, ""},
		{"valid memory", validMemory, ""},
		{"invalid: ScaleMetric set", invalidScaleMetric, "ScaleMetric is not supported for KEDA"},
		{"invalid: missing resource", missingResource, "metricSpec.Resource is not set for resource metric source type"},
		{"invalid: cpu wrong type", invalidCPUType, "the cpu target value type should be Utilization"},
		{"invalid: memory wrong type", invalidMemoryType, "the memory target value type should be AverageValue or Utilization"},
		{"invalid: memory value too low", invalidMemoryValue, "the memory target value should be greater than 1 MiB"},
		{"invalid: unsupported resource", unsupportedResource, "resource type disk is not supported"},
		{"invalid: external metric missing query/value", invalidExternal, "the query should not be empty"},
		{"valid: external metric", validExternal, ""},
		{"invalid: pod metric missing query/value", invalidPodMetric, "the query should not be empty"},
		{"valid: pod metric", validPodMetric, ""},
		{"invalid: unknown metric type", unknownMetricType, "unknown KEDA metric type with value [UnknownType].Valid types are Resource,External,PodMetric"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScalingKedaCompExtension(tt.spec)
			if tt.wantErr == "" {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(tt.wantErr)))
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestValidateStorageUriSpec(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		storageUri *StorageUri
		expected   gomega.OmegaMatcher
	}{
		"ValidStorageUriSpec": {
			storageUri: &StorageUri{
				Uri:       "gs://bucket/model",
				MountPath: "/mnt/models",
			},
			expected: gomega.BeNil(),
		},
		"ValidStorageUriSpecWithRootPath": {
			storageUri: &StorageUri{
				Uri:       "s3://bucket/model",
				MountPath: "/",
			},
			expected: gomega.MatchError("storage path cannot be empty"),
		},
		"EmptyUri": {
			storageUri: &StorageUri{
				Uri:       "",
				MountPath: "/mnt/models",
			},
			expected: gomega.MatchError("storage URI cannot be empty"),
		},
		"RelativePath": {
			storageUri: &StorageUri{
				Uri:       "gs://bucket/model",
				MountPath: "mnt/models",
			},
			expected: gomega.MatchError("storage path must be absolute: mnt/models"),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := validateStorageURISpec(scenario.storageUri)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestValidateStorageUri(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		storageUris []StorageUri
		expected    gomega.OmegaMatcher
	}{
		"EmptyList": {
			storageUris: []StorageUri{},
			expected:    gomega.BeNil(),
		},
		"SingleValidStorageUri": {
			storageUris: []StorageUri{
				{
					Uri:       "gs://bucket/model1",
					MountPath: "/mnt/models",
				},
			},
			expected: gomega.BeNil(),
		},
		"MultipleValidStorageUrisWithCommonParent": {
			storageUris: []StorageUri{
				{
					Uri:       "gs://bucket/model1",
					MountPath: "/mnt/models/model1",
				},
				{
					Uri:       "s3://bucket/model2",
					MountPath: "/mnt/models/model2",
				},
			},
			expected: gomega.BeNil(),
		},
		"MultipleStorageUrisWithoutCommonParent": {
			storageUris: []StorageUri{
				{
					Uri:       "gs://bucket/model1",
					MountPath: "/mnt/models",
				},
				{
					Uri:       "s3://bucket/model2",
					MountPath: "/opt/models",
				},
			},
			expected: gomega.MatchError(gomega.ContainSubstring("storage paths must have a common parent directory")),
		},
		"InvalidStorageUriInList": {
			storageUris: []StorageUri{
				{
					Uri:       "gs://bucket/model1",
					MountPath: "/mnt/models",
				},
				{
					Uri:       "",
					MountPath: "/mnt/models/model2",
				},
			},
			expected: gomega.MatchError("storage URI cannot be empty"),
		},
		"RelativePathInList": {
			storageUris: []StorageUri{
				{
					Uri:       "gs://bucket/model1",
					MountPath: "/mnt/models",
				},
				{
					Uri:       "s3://bucket/model2",
					MountPath: "mnt/models/model2",
				},
			},
			expected: gomega.MatchError("storage path must be absolute: mnt/models/model2"),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := validateMultipleStorageURIsSpec(scenario.storageUris)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestValidateBlockedEnvVars(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		isvc      *InferenceService
		expectErr bool
	}{
		"PYTHONPATH in predictor model env should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: corev1.Container{
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/custom"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in predictor custom container should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name: "kserve-container",
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/injected"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in workerSpec container should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "huggingface"},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec: PodSpec{
								Containers: []corev1.Container{
									{
										Name: "worker",
										Env: []corev1.EnvVar{
											{Name: "PYTHONPATH", Value: "/bad"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in transformer container should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{
								{
									Name: "transformer",
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/evil"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"allowed env vars should pass": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: corev1.Container{
									Env: []corev1.EnvVar{
										{Name: "MODEL_NAME", Value: "my-model"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		"PYTHONPATH in explainer ART container should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
					},
					Explainer: &ExplainerSpec{
						ART: &ARTExplainerSpec{
							ExplainerExtensionSpec: ExplainerExtensionSpec{
								Container: corev1.Container{
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/injected"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in predictor initContainer should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
						PodSpec: PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "init-setup",
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/injected"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in workerSpec initContainer should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "huggingface"},
						},
						WorkerSpec: &WorkerSpec{
							PodSpec: PodSpec{
								InitContainers: []corev1.Container{
									{
										Name: "init-worker",
										Env: []corev1.EnvVar{
											{Name: "PYTHONPATH", Value: "/injected"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in explainer initContainer should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
					},
					Explainer: &ExplainerSpec{
						PodSpec: PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "init-explainer",
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/evil"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"PYTHONPATH in transformer initContainer should be rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "init-transform",
									Env: []corev1.EnvVar{
										{Name: "PYTHONPATH", Value: "/evil"},
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"no env vars should pass": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-isvc"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
						},
					},
				},
			},
			expectErr: false,
		},
	}

	for name, tc := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := validateBlockedEnvVars(tc.isvc)
			if tc.expectErr {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err.Error()).To(gomega.ContainSubstring("PYTHONPATH"))
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestValidateConfidential(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validResourceId := "kbs:///default/key/model-key"

	scenarios := map[string]struct {
		isvc             InferenceService
		expectedErr      gomega.OmegaMatcher
		expectedWarnings gomega.OmegaMatcher
	}{
		"confidential nil": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://bucket/model"),
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.BeEmpty(),
		},
		"confidential disabled": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://bucket/model"),
								Confidential: &ConfidentialSpec{
									Enabled: false,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.BeEmpty(),
		},
		"confidential enabled with storageUri and resourceId": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://bucket/model"),
								Confidential: &ConfidentialSpec{
									Enabled:    true,
									ResourceId: &validResourceId,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.BeEmpty(),
		},
		"confidential enabled with storageUri no resourceId": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://bucket/model"),
								Confidential: &ConfidentialSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.BeEmpty(),
		},
		"confidential enabled without storageUri": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								Confidential: &ConfidentialSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.MatchError("confidential model serving requires storageUri to be set"),
			expectedWarnings: gomega.BeEmpty(),
		},
		"confidential enabled with OCI URI warns": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("oci://registry/model:latest"),
								Confidential: &ConfidentialSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.ContainElement(gomega.ContainSubstring("OCI URIs")),
		},
		"confidential enabled with PVC URI warns": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("pvc://my-pvc/model-dir"),
								Confidential: &ConfidentialSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.BeNil(),
			expectedWarnings: gomega.ContainElement(gomega.ContainSubstring("PVC URIs")),
		},
		"confidential with malformed resourceId": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://bucket/model"),
								Confidential: &ConfidentialSpec{
									Enabled:    true,
									ResourceId: proto.String("invalid-resource-id"),
								},
							},
						},
					},
				},
			},
			expectedErr:      gomega.MatchError(gomega.ContainSubstring("must be in the format kbs:///<repo>/<type>/<tag>")),
			expectedWarnings: gomega.BeEmpty(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			validator := InferenceServiceValidator{}
			warnings, err := validator.ValidateCreate(t.Context(), &scenario.isvc)
			g.Expect(err).To(scenario.expectedErr)
			g.Expect(warnings).To(scenario.expectedWarnings)
		})
	}
}

func TestValidateCanarySpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validator := InferenceServiceValidator{}

	makeISVC := func(canaries []CanarySpec) *InferenceService {
		return &InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo", Namespace: "default",
				Annotations: map[string]string{
					constants.DeploymentMode: string(constants.Standard),
				},
			},
			Spec: InferenceServiceSpec{
				Predictor: PredictorSpec{
					ComponentExtensionSpec: ComponentExtensionSpec{
						MinReplicas: proto.Int32(4),
					},
					Model: &ModelSpec{
						ModelFormat: ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{
							StorageURI: proto.String("gs://test/model-v1"),
						},
					},
				},
				Canary: canaries,
			},
		}
	}

	validCanary := func(name string, traffic int32) CanarySpec {
		return CanarySpec{
			TrafficPercent: traffic,
			Predictor: PredictorSpec{
				Name: name,
				Model: &ModelSpec{
					ModelFormat: ModelFormat{Name: "sklearn"},
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://test/model-" + name),
					},
				},
			},
		}
	}

	scenarios := map[string]struct {
		isvc       *InferenceService
		errMatcher gomega.OmegaMatcher
	}{
		"No canaries passes validation": {
			isvc:       makeISVC(nil),
			errMatcher: gomega.BeNil(),
		},
		"Single valid canary": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 10)}),
			errMatcher: gomega.BeNil(),
		},
		"Multiple valid canaries": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 25), validCanary("v3", 25)}),
			errMatcher: gomega.BeNil(),
		},
		"Traffic sum = 100 accepted": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 50), validCanary("v3", 50)}),
			errMatcher: gomega.BeNil(),
		},
		"Traffic percent = 0 accepted (dark launch)": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 0)}),
			errMatcher: gomega.BeNil(),
		},
		"Canary rejected in Knative mode": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo", Namespace: "default",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://test/model-v1"),
							},
						},
					},
					Canary: []CanarySpec{validCanary("v2", 10)},
				},
			},
			errMatcher: gomega.MatchError(gomega.ContainSubstring("only supported in Standard deployment mode")),
		},
		"Missing predictor.name rejected": {
			isvc: makeISVC([]CanarySpec{{
				TrafficPercent: 10,
				Predictor: PredictorSpec{
					Model: &ModelSpec{
						ModelFormat:            ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
					},
				},
			}}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("predictor.name is required")),
		},
		"Duplicate predictor.name rejected": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 10), validCanary("v2", 20)}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("duplicated")),
		},
		"Canary name conflicts with stable name rejected": {
			isvc: func() *InferenceService {
				isvc := makeISVC([]CanarySpec{validCanary("stable", 10)})
				isvc.Spec.Predictor.Name = "stable"
				return isvc
			}(),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("conflicts with stable")),
		},
		"Traffic sum > 100 rejected": {
			isvc:       makeISVC([]CanarySpec{validCanary("v2", 60), validCanary("v3", 50)}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("must be <= 100")),
		},
		"Canary with maxReplicas rejected": {
			isvc: makeISVC([]CanarySpec{{
				TrafficPercent: 10,
				Predictor: PredictorSpec{
					Name:                   "v2",
					ComponentExtensionSpec: ComponentExtensionSpec{MaxReplicas: 5},
					Model: &ModelSpec{
						ModelFormat:            ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
					},
				},
			}}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("must not set maxReplicas")),
		},
		"Canary minReplicas equal to stable accepted": {
			isvc: makeISVC([]CanarySpec{{
				TrafficPercent: 10,
				Predictor: PredictorSpec{
					Name:                   "v2",
					ComponentExtensionSpec: ComponentExtensionSpec{MinReplicas: proto.Int32(4)},
					Model: &ModelSpec{
						ModelFormat:            ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
					},
				},
			}}),
			errMatcher: gomega.BeNil(),
		},
		"Canary minReplicas exceeds stable rejected": {
			isvc: makeISVC([]CanarySpec{{
				TrafficPercent: 10,
				Predictor: PredictorSpec{
					Name:                   "v2",
					ComponentExtensionSpec: ComponentExtensionSpec{MinReplicas: proto.Int32(5)},
					Model: &ModelSpec{
						ModelFormat:            ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
					},
				},
			}}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("must not exceed stable")),
		},
		"Canary minReplicas equal to default stable accepted": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo", Namespace: "default",
					Annotations: map[string]string{
						constants.DeploymentMode: string(constants.Standard),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat:            ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/model-v1")},
						},
					},
					Canary: []CanarySpec{{
						TrafficPercent: 10,
						Predictor: PredictorSpec{
							Name:                   "v2",
							ComponentExtensionSpec: ComponentExtensionSpec{MinReplicas: proto.Int32(1)},
							Model: &ModelSpec{
								ModelFormat:            ModelFormat{Name: "sklearn"},
								PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
							},
						},
					}},
				},
			},
			errMatcher: gomega.BeNil(),
		},
		"Canary minReplicas exceeds default stable rejected": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo", Namespace: "default",
					Annotations: map[string]string{
						constants.DeploymentMode: string(constants.Standard),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat:            ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/model-v1")},
						},
					},
					Canary: []CanarySpec{{
						TrafficPercent: 10,
						Predictor: PredictorSpec{
							Name:                   "v2",
							ComponentExtensionSpec: ComponentExtensionSpec{MinReplicas: proto.Int32(2)},
							Model: &ModelSpec{
								ModelFormat:            ModelFormat{Name: "sklearn"},
								PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
							},
						},
					}},
				},
			},
			errMatcher: gomega.MatchError(gomega.ContainSubstring("must not exceed stable")),
		},
		"Canary minReplicas accepted when nil (derived from traffic)": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo", Namespace: "default",
					Annotations: map[string]string{
						constants.DeploymentMode: string(constants.Standard),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat:            ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/model-v1")},
						},
					},
					Canary: []CanarySpec{validCanary("v2", 10)},
				},
			},
			errMatcher: gomega.BeNil(),
		},
		"Canary with workerSpec rejected": {
			isvc: makeISVC([]CanarySpec{{
				TrafficPercent: 10,
				Predictor: PredictorSpec{
					Name:       "v2",
					WorkerSpec: &WorkerSpec{},
					Model: &ModelSpec{
						ModelFormat:            ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: proto.String("gs://test/v2")},
					},
				},
			}}),
			errMatcher: gomega.MatchError(gomega.ContainSubstring("must not set workerSpec")),
		},
		"Canary rejected when stable uses custom container (no model)": {
			isvc: &InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo", Namespace: "default",
					Annotations: map[string]string{
						constants.DeploymentMode: string(constants.Standard),
					},
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Name:  "kserve-container",
								Image: "custom-server:latest",
							}},
						},
					},
					Canary: []CanarySpec{validCanary("v2", 10)},
				},
			},
			errMatcher: gomega.MatchError(gomega.ContainSubstring("requires a stable predictor with a model")),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			_, err := validator.ValidateCreate(t.Context(), scenario.isvc)
			g.Expect(err).Should(scenario.errMatcher)
		})
	}
}

func TestValidatePredictorNameChange(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validator := InferenceServiceValidator{}

	makeISVC := func(name, storageURI string) *InferenceService {
		return &InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo", Namespace: "default",
				Annotations: map[string]string{
					constants.DeploymentMode: string(constants.Standard),
				},
			},
			Spec: InferenceServiceSpec{
				Predictor: PredictorSpec{
					Name: name,
					Model: &ModelSpec{
						ModelFormat: ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: PredictorExtensionSpec{
							StorageURI: proto.String(storageURI),
						},
					},
				},
			},
		}
	}

	t.Run("Name change with model change accepted (promotion)", func(t *testing.T) {
		oldISVC := makeISVC("", "gs://test/model-v1")
		newISVC := makeISVC("v2", "gs://test/model-v2")
		_, err := validator.ValidateUpdate(t.Context(), oldISVC, newISVC)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
	})

	t.Run("Name change without model change rejected", func(t *testing.T) {
		oldISVC := makeISVC("", "gs://test/model-v1")
		newISVC := makeISVC("v2", "gs://test/model-v1")
		_, err := validator.ValidateUpdate(t.Context(), oldISVC, newISVC)
		g.Expect(err).Should(gomega.MatchError(gomega.ContainSubstring("predictor.name change requires a model change")))
	})

	t.Run("Model change without name change accepted (normal update)", func(t *testing.T) {
		oldISVC := makeISVC("v2", "gs://test/model-v1")
		newISVC := makeISVC("v2", "gs://test/model-v2")
		_, err := validator.ValidateUpdate(t.Context(), oldISVC, newISVC)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
	})

	t.Run("No changes accepted", func(t *testing.T) {
		oldISVC := makeISVC("v2", "gs://test/model-v1")
		newISVC := makeISVC("v2", "gs://test/model-v1")
		_, err := validator.ValidateUpdate(t.Context(), oldISVC, newISVC)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
	})
}
