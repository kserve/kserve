/*

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
	inferenceservice.applyDefaultsEndpoint(&inferenceservice.Spec.Default, c)
	return inferenceservice
}

func TestValidStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	for _, prefix := range SupportedStorageURIPrefixList {
		isvc := makeTestInferenceService()
		isvc.Spec.Default.Predictor.Tensorflow.StorageURI = prefix + "foo/bar"
		g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	}
}

func TestEmptyStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = ""
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestLocalPathStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "some/relative/path"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "/some/absolute/path"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "/"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "foo"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestAzureBlobOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/triton/simple_string/"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/triton/simple_string"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/triton/"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://kfserving.blob.core.windows.net/triton"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestAzureBlobNoAccountFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://blob.core.windows.net/triton/simple_string/"
	g.Expect(isvc.validate(c)).ShouldNot(gomega.Succeed())
}

func TestAzureBlobNoContainerFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://foo.blob.core.windows.net/"
	g.Expect(isvc.validate(c)).ShouldNot(gomega.Succeed())
}

func TestHttpStorageURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "https://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "http://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestUnkownStorageURIPrefixFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow.StorageURI = "blob://foo/bar"
	g.Expect(isvc.validate(c)).ShouldNot(gomega.Succeed())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Custom = &CustomSpec{Container: v1.Container{}}
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Tensorflow = nil
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
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
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}

func TestRejectCanaryModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Canary = &EndpointSpec{
		Predictor: PredictorSpec{},
	}
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOnePredictorViolatedError))
}

func TestRejectBadCanaryTrafficValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Canary = &isvc.Spec.Default
	isvc.Spec.CanaryTrafficPercent = GetIntReference(-1)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(TrafficBoundsExceededError))
	isvc.Spec.CanaryTrafficPercent = GetIntReference(101)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(TrafficBoundsExceededError))
}

func TestRejectTrafficProvidedWithoutCanary(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.CanaryTrafficPercent = GetIntReference(1)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(TrafficProvidedWithoutCanaryError))
}

func TestBadParallelismValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.Parallelism = -1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ParallelismLowerBoundExceededError))
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Predictor.MinReplicas = GetIntReference(-1)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Default.Predictor.MinReplicas = GetIntReference(1)
	isvc.Spec.Default.Predictor.MaxReplicas = -1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Default.Predictor.MinReplicas = GetIntReference(2)
	isvc.Spec.Default.Predictor.MaxReplicas = 1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	// Now test transformer and explainer, so set correct value for predictor
	isvc.Spec.Default.Predictor.MinReplicas = GetIntReference(0)
	isvc.Spec.Default.Predictor.MaxReplicas = 0

	isvc.Spec.Default.Transformer = &TransformerSpec{}
	isvc.Spec.Default.Transformer.Custom = &CustomSpec{
		v1.Container{
			Image: "custom:0.1",
		},
	}
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	isvc.Spec.Default.Transformer.MinReplicas = GetIntReference(-1)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Default.Transformer.MinReplicas = GetIntReference(1)
	isvc.Spec.Default.Transformer.MaxReplicas = -1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Default.Transformer.MinReplicas = GetIntReference(2)
	isvc.Spec.Default.Transformer.MaxReplicas = 1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
	// Now test explainer, so ignore transformer
	isvc.Spec.Default.Transformer = nil

	isvc.Spec.Default.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			StorageURI: "gs://testbucket/testmodel",
		},
	}
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	isvc.Spec.Default.Explainer.MinReplicas = GetIntReference(-1)
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	isvc.Spec.Default.Explainer.MinReplicas = GetIntReference(1)
	isvc.Spec.Default.Explainer.MaxReplicas = -1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	isvc.Spec.Default.Explainer.MinReplicas = GetIntReference(2)
	isvc.Spec.Default.Explainer.MaxReplicas = 1
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
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
	g.Expect(isvc.validate(c)).Should(gomega.MatchError("Custom container validation error: must not set the field(s): stdin, stdinOnce"))
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
	err := isvc.validate(c)
	fmt.Println(err)
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestRejectBadTransformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Transformer = &TransformerSpec{}
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOneTransformerViolatedError))
}

func TestRejectBadExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Explainer = &ExplainerSpec{}
	g.Expect(isvc.validate(c)).Should(gomega.MatchError(ExactlyOneExplainerViolatedError))
}

func TestGoodExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Spec.Default.Explainer = &ExplainerSpec{
		Alibi: &AlibiExplainerSpec{
			StorageURI: "gs://testbucket/testmodel",
		},
	}
	isvc.applyDefaultsEndpoint(&isvc.Spec.Default, c)
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc-123"
	g.Expect(isvc.validate(c)).Should(gomega.Succeed())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "1abcde"
	g.Expect(isvc.validate(c)).ShouldNot(gomega.Succeed())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := makeTestInferenceService()
	isvc.Name = "abc.de"
	g.Expect(isvc.validate(c)).ShouldNot(gomega.Succeed())
}
