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

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestInvalidAutoscalerClassType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/autoscalerClass"] = "test"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestValidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "70"
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestInvalidTargetUtilizationPercentage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "101"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "abc"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "0"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())

	isvc.ObjectMeta.Annotations["serving.kserve.io/targetUtilizationPercentage"] = "99.9"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestInvalidAutoscalerHPAMetrics(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestRawInferenceService()
	isvc.ObjectMeta.Annotations["serving.kserve.io/metrics"] = "test"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestValidStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	for _, prefix := range SupportedStorageURIPrefixList {
		isvc := makeTestInferenceService()
		isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String(prefix + "foo/bar")
		g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	}
}

func TestEmptyStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestLocalPathStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("some/relative/path")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("/some/absolute/path")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("/")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("foo")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestAzureBlobOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://kfserving.blob.core.windows.net/triton/simple_string/")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://kfserving.blob.core.windows.net/triton/simple_string")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://kfserving.blob.core.windows.net/triton/")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://kfserving.blob.core.windows.net/triton")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestAzureBlobNoAccountFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://blob.core.windows.net/triton/simple_string/")
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestAzureBlobNoContainerFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://foo.blob.core.windows.net/")
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestHttpStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("https://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("http://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz")
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestUnknownStorageURIPrefixFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow.StorageURI = proto.String("blob://foo/bar")
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.XGBoost = &XGBoostSpec{}
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
}

func TestModelSpecAndCustomOverridesIsValid(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.PodSpec = PodSpec{ServiceAccountName: "test"}
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow = nil
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneErrorFor(&isvc.Spec.Predictor)))
}

func TestBadParallelismValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.ContainerConcurrency = proto.Int64(-1)
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(ParallelismLowerBoundExceededError))
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.MinReplicas = GetIntReference(-1)
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Predictor.MinReplicas = GetIntReference(1)
	isvc.Spec.Predictor.MaxReplicas = -1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Predictor.MinReplicas = GetIntReference(2)
	isvc.Spec.Predictor.MaxReplicas = 1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
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
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Transformer.MinReplicas = GetIntReference(1)
	isvc.Spec.Transformer.MaxReplicas = -1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Transformer.MinReplicas = GetIntReference(2)
	isvc.Spec.Transformer.MaxReplicas = 1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
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
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Explainer.MinReplicas = GetIntReference(1)
	isvc.Spec.Explainer.MaxReplicas = -1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Explainer.MinReplicas = GetIntReference(2)
	isvc.Spec.Explainer.MaxReplicas = 1
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
}

func TestCustomOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Predictor.Tensorflow = nil
	isvc.Spec.Predictor.PodSpec = PodSpec{
		Containers: []v1.Container{
			{
				Image: "some-image",
			},
		},
	}
	err := isvc.ValidateCreate()
	fmt.Println(err)
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Transformer = &TransformerSpec{}
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Transformer)))
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Explainer = &ExplainerSpec{}
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneErrorFor(isvc.Spec.Explainer)))
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
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc-123"
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "1abcde"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc.de"
	g.Expect(isvc.ValidateCreate()).ShouldNot(gomega.Succeed())
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
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(fmt.Sprintf(MaxWorkersShouldBeLessThanMaxError, 1)))
	isvc.Spec.Predictor.PMML.Container.Args = []string{"--workers=foo"}
	g.Expect(isvc.ValidateCreate()).Should(gomega.MatchError(InvalidWorkerArgument))
	isvc.Spec.Predictor.PMML.Container.Args = []string{"--workers=1"}
	g.Expect(isvc.ValidateCreate()).Should(gomega.Succeed())
}
