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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeTestKFService() KFService {
	kfservice := KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: KFServiceSpec{
			Default: ServicesSpec{
				Predict: ModelSpec{
					Tensorflow: &TensorflowSpec{ModelURI: "gs://testbucket/testmodel"},
				},
			},
		},
	}
	kfservice.Default()
	return kfservice
}

func TestValidModelURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	for _, prefix := range SupportedModelSourceURIPrefixList {
		kfsvc := makeTestKFService()
		kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = prefix + "foo/bar"
		g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	}
}

func TestEmptyModelURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = ""
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestLocalPathModelURIPrefixOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "some/relative/path"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "/some/absolute/path"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "/"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "foo"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestAzureBlobOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://kfserving.blob.core.windows.net/tensorrt/simple_string/"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://kfserving.blob.core.windows.net/tensorrt/simple_string"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://kfserving.blob.core.windows.net/tensorrt/"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://kfserving.blob.core.windows.net/tensorrt"
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
}

func TestAzureBlobNoAccountFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://blob.core.windows.net/tensorrt/simple_string/"
	g.Expect(kfsvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestAzureBlobNoContainerFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "https://foo.blob.core.windows.net/"
	g.Expect(kfsvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestUnkownModelURIPrefixFails(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.ModelURI = "blob://foo/bar"
	g.Expect(kfsvc.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestRejectMultipleModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Custom = &CustomSpec{Container: v1.Container{}}
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneModelSpecViolatedError))
}

func TestRejectModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow = nil
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(AtLeastOneModelSpecViolatedError))
}
func TestRejectMultipleCanaryModelSpecs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Canary = &ServicesSpec{
		Predict: ModelSpec{
			Custom:     &CustomSpec{Container: v1.Container{}},
			Tensorflow: kfsvc.Spec.Default.Predict.Tensorflow,
		},
	}
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(ExactlyOneModelSpecViolatedError))
}

func TestRejectCanaryModelSpecMissing(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Canary = &ServicesSpec{}
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(AtLeastOneModelSpecViolatedError))
}
func TestRejectBadCanaryTrafficValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Canary = &kfsvc.Spec.Default
	kfsvc.Spec.CanaryTrafficPercent = -1
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(TrafficBoundsExceededError))
	kfsvc.Spec.CanaryTrafficPercent = 101
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(TrafficBoundsExceededError))
}

func TestRejectTrafficProvidedWithoutCanary(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.CanaryTrafficPercent = 1
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(TrafficProvidedWithoutCanaryError))
}

func TestBadReplicaValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow.MinReplicas = -1
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasLowerBoundExceededError))
	kfsvc.Spec.Default.Predict.Tensorflow.MinReplicas = 1
	kfsvc.Spec.Default.Predict.Tensorflow.MaxReplicas = -1
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(MaxReplicasLowerBoundExceededError))
	kfsvc.Spec.Default.Predict.Tensorflow.MinReplicas = 2
	kfsvc.Spec.Default.Predict.Tensorflow.MaxReplicas = 1
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError(MinReplicasShouldBeLessThanMaxError))
}

func TestCustomBadFields(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow = nil
	kfsvc.Spec.Default.Predict.Custom = &CustomSpec{
		Container: v1.Container{
			Name:      "foo",
			Image:     "custom:0.1",
			Stdin:     true,
			StdinOnce: true,
		},
	}
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.MatchError("Custom container validation error: must not set the field(s): stdin, stdinOnce"))
}

func TestCustomOK(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := makeTestKFService()
	kfsvc.Spec.Default.Predict.Tensorflow = nil
	kfsvc.Spec.Default.Predict.Custom = &CustomSpec{
		Container: v1.Container{
			Image: "custom:0.1",
		},
	}
	err := kfsvc.ValidateCreate()
	fmt.Println(err)
	g.Expect(kfsvc.ValidateCreate()).Should(gomega.Succeed())
}
