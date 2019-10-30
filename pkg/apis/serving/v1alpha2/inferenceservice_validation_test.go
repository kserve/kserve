/*
Copyright 2019 kubeflow.org.

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
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeTestInferenceService() InferenceService {
	inferenceservice := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Tensorflow: &TensorflowSpec{StorageURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	inferenceservice.Default(c)
	return inferenceservice
}

func TestValidStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	for _, prefix := range SupportedStorageURIPrefixList {
		isvc := makeTestInferenceService()
		isvc.Spec.Default.Predictor.Tensorflow.StorageURI = prefix + "foo/bar"
		g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	}
}

func TestEmptyStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = ""
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
}

func TestLocalPathStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "some/relative/path"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "/some/absolute/path"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "/"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "foo"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
}

func TestAzureBlobOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/tensorrt/simple_string/"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/tensorrt/simple_string"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/tensorrt/"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/tensorrt"
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
}

func TestAzureBlobNoAccountFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://blob.core.windows.net/tensorrt/simple_string/"
	g.Expect(isvc.ValidateCreate(c)).ShouldNot(gomega.Succeed())
}

func TestAzureBlobNoContainerFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://foo.blob.core.windows.net/"
	g.Expect(isvc.ValidateCreate(c)).ShouldNot(gomega.Succeed())
}

func TestUnkownStorageURIPrefixFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "blob://foo/bar"
	g.Expect(isvc.ValidateCreate(c)).ShouldNot(gomega.Succeed())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Custom = &CustomSpec{Container: v1.Container{}}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow = nil
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}
func TestRejectMultipleCanaryModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Canary = &EndpointSpec{
		Predictor: PredictorSpec{
			Custom:     &CustomSpec{Container: v1.Container{}},
			Tensorflow: isvc.Spec.Default.Predictor.Tensorflow,
		},
	}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}

func TestRejectCanaryModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Canary = &EndpointSpec{
		Predictor: PredictorSpec{},
	}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}
func TestRejectBadCanaryTrafficValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Canary = &isvc.Spec.Default
	isvc.Spec.CanaryTrafficPercent = -1
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(TrafficBoundsExceededError))
	isvc.Spec.CanaryTrafficPercent = 101
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(TrafficBoundsExceededError))
}

func TestRejectTrafficProvidedWithoutCanary(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.CanaryTrafficPercent = 1
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(TrafficProvidedWithoutCanaryError))
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.MinReplicas = -1
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Default.Predictor.MinReplicas = 1
	isvc.Spec.Default.Predictor.MaxReplicas = -1
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Default.Predictor.MinReplicas = 2
	isvc.Spec.Default.Predictor.MaxReplicas = 1
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
}

func TestCustomBadFields(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow = nil
	isvc.Spec.Default.Predictor.Custom = &CustomSpec{
		v1.Container{
			Name:      "foo",
			Image:     "custom:0.1",
			Stdin:     true,
			StdinOnce: true,
		},
	}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError("Custom container validation error: must not set the field(s): stdin, stdinOnce"))
}

func TestCustomOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow = nil
	isvc.Spec.Default.Predictor.Custom = &CustomSpec{
		v1.Container{
			Image: "custom:0.1",
		},
	}
	err := isvc.ValidateCreate(c)
	fmt.Println(err)
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Transformer = &TransformerSpec{}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOneTransformerViolatedError))
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Explainer = &ExplainerSpec{}
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError(ExactlyOneExplainerViolatedError))
}

func TestGoodExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			StorageURI: "gs://testbucket/testmodel",
		},
	}
	isvc.Default(c)
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.Succeed())
}

func TestValidateExplainerResource(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			StorageURI: "gs://testbucket/testmodel",
		},
	}
	isvc.Default(c)
	isvc.Spec.Default.Explainer.Alibi.Resources.Limits["cpu"] = resource.MustParse("1")
	isvc.Spec.Default.Explainer.Alibi.Resources.Requests["cpu"] = resource.MustParse("2")
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError("Unexpected error: [resources.requests: Invalid value: \"2\": must be less than or equal to cpu limit]"))
}

func TestValidPredictorResource(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Default(c)
	isvc.Spec.Default.Predictor.Tensorflow.Resources.Limits["cpu"] = resource.MustParse("1")
	isvc.Spec.Default.Predictor.Tensorflow.Resources.Requests["cpu"] = resource.MustParse("2")
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError("Unexpected error: [resources.requests: Invalid value: \"2\": must be less than or equal to cpu limit]"))
}

func TestValidTransformerResource(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Transformer = &TransformerSpec{}
	isvc.Spec.Default.Transformer.Custom = &CustomSpec{
		v1.Container{
			Image: "custom:0.1",
		},
	}
	isvc.Default(c)
	isvc.Spec.Default.Transformer.Custom.Container.Resources.Limits["cpu"] = resource.MustParse("1")
	isvc.Spec.Default.Transformer.Custom.Container.Resources.Requests["cpu"] = resource.MustParse("2")
	g.Expect(isvc.ValidateCreate(c)).Should(gomega.MatchError("Unexpected error: [resources.requests: Invalid value: \"2\": must be less than or equal to cpu limit]"))
}
