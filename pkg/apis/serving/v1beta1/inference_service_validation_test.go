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
	"github.com/kserve/kserve/pkg/constants"
	"testing"

	"github.com/golang/protobuf/proto"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeIsvcConfig() (*InferenceServicesConfig, *DeployConfig) {
	config := &InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "alibi",
				DefaultImageVersion: "v0.4.0",
			},
		},
	}
	deployConfig := &DeployConfig{
		DefaultDeploymentMode: "Serverless",
	}
	return config, deployConfig
}

func makeTestRawInferenceService() InferenceService {
	// Reset the deprecatedFieldName variable to avoid any
	deprecatedFieldName = ""

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
				Model: &ModelSpec{
					ModelFormat: ModelFormat{
						Name: constants.SupportedModelTensorflow,
					},
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
	// Reset the deprecatedFieldName variable to avoid any
	deprecatedFieldName = ""

	inferenceservice := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				Model: &ModelSpec{
					ModelFormat: ModelFormat{
						Name: constants.SupportedModelTensorflow,
					},
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
	warnings, err := isvc.ValidateCreate()
	g.Expect(warnings).Should(gomega.BeEmpty())
	g.Expect(err).Should(gomega.Succeed())
}

func TestInvalidAutoscalerClassType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/autoscalerClass"] = "test"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestValidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "70"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestInvalidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "101"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "abc"
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "0"
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "99.9"
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestInvalidAutoscalerHPAMetrics(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/metrics"] = "test"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
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
	config, deployConfig := makeIsvcConfig()
	isvc.DefaultInferenceService(config, deployConfig)
	isvc.Spec.Predictor.XGBoost = &XGBoostSpec{}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.ConsistOf(fmt.Sprintf(deprecationMsg, constants.SupportedModelTensorflow)))
}

func TestModelSpecAndCustomOverridesIsValid(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.PodSpec = PodSpec{ServiceAccountName: "test"}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Model = nil
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadParallelismValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.ContainerConcurrency = proto.Int64(-1)
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(ParallelismLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.MinReplicas = GetIntReference(-1)
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = GetIntReference(1)
	isvc.Spec.Predictor.MaxReplicas = -1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.MinReplicas = GetIntReference(2)
	isvc.Spec.Predictor.MaxReplicas = 1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	// Now test transformer and explainer, so set correct value for predictor
	isvc.Spec.Predictor.MinReplicas = GetIntReference(0)
	isvc.Spec.Predictor.MaxReplicas = 0

	isvc.Spec.Transformer = &TransformerSpec{}
	isvc.Spec.Transformer.PodSpec = PodSpec{
		Containers: []v1.Container{
			{
				Image: "some-image",
			},
		},
	}
	isvc.Spec.Transformer.MinReplicas = GetIntReference(-1)
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = GetIntReference(1)
	isvc.Spec.Transformer.MaxReplicas = -1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Transformer.MinReplicas = GetIntReference(2)
	isvc.Spec.Transformer.MaxReplicas = 1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	// Now test explainer, so ignore transformer
	isvc.Spec.Transformer = nil

	isvc.Spec.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			ExplainerExtensionSpec: ExplainerExtensionSpec{
				StorageURI: "gs://testbucket/testmodel",
			},
		},
	}
	isvc.Spec.Explainer.MinReplicas = GetIntReference(-1)
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = GetIntReference(1)
	isvc.Spec.Explainer.MaxReplicas = -1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Explainer.MinReplicas = GetIntReference(2)
	isvc.Spec.Explainer.MaxReplicas = 1
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestCustomOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Model = nil
	isvc.Spec.Predictor.PodSpec = PodSpec{
		Containers: []v1.Container{
			{
				Image: "some-image",
			},
		},
	}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Transformer = &TransformerSpec{}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Transformer)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Explainer)))
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestGoodExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			ExplainerExtensionSpec: ExplainerExtensionSpec{
				StorageURI: "gs://testbucket/testmodel",
			},
		},
	}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc-123"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "1abcde"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc.de"
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).ShouldNot(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestPMMLWorkersArguments(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				PMML: &PMMLSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
	}

	isvc.Spec.Predictor.PMML.Container.Args = []string{"--workers=2"}
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(fmt.Sprintf(MaxWorkersShouldBeLessThanMaxError, 1)))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.PMML.Container.Args = []string{"--workers=foo"}
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.MatchError(InvalidWorkerArgument))
	g.Expect(warnings).Should(gomega.BeEmpty())

	isvc.Spec.Predictor.PMML.Container.Args = []string{"--workers=1"}
	warnings, err = isvc.ValidateCreate()
	g.Expect(err).Should(gomega.Succeed())
	g.Expect(warnings).Should(gomega.BeEmpty())
}

func TestOldSchemaDeprecation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Predictor: PredictorSpec{
				SKLearn: &SKLearnSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("gs://testbucket/testmodel"),
					},
				},
			},
		},
	}

	config, deployConfig := makeIsvcConfig()
	isvc.DefaultInferenceService(config, deployConfig)
	warnings, err := isvc.ValidateCreate()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(warnings).Should(gomega.ConsistOf(fmt.Sprintf(deprecationMsg, constants.SupportedModelSKLearn)))
}
