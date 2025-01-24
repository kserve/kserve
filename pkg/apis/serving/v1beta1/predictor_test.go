/*
Copyright 2022 The KServe Authors.

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
	"testing"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
)

func makeTestPredictorSpec() *PredictorSpec {
	return &PredictorSpec{
		PyTorch: &TorchServeSpec{
			PredictorExtensionSpec: PredictorExtensionSpec{
				RuntimeVersion: proto.String("0.4.1"),
			},
		},
	}
}

func TestGetPredictorImplementations(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	spec := makeTestPredictorSpec()
	implementations := spec.GetPredictorImplementations()
	g.Expect(implementations).ShouldNot(gomega.BeEmpty())
	g.Expect(implementations[0]).Should(gomega.Equal(spec.PyTorch))

	spec.PyTorch = nil
	implementations = spec.GetPredictorImplementations()
	g.Expect(implementations).Should(gomega.BeEmpty())

	spec.PodSpec.Containers = []v1.Container{
		{
			Name:  "Test-Container",
			Image: "test/predictor",
		},
	}
	implementations = spec.GetPredictorImplementations()
	g.Expect(implementations).ShouldNot(gomega.BeEmpty())
	g.Expect(implementations[0]).Should(gomega.Equal(NewCustomPredictor(&spec.PodSpec)))
}

func TestGetPredictorImplementation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	spec := makeTestPredictorSpec()
	expected := spec.PyTorch
	implementation := spec.GetPredictorImplementation()
	g.Expect(*implementation).Should(gomega.Equal(expected))

	spec.PyTorch = nil
	implementation = spec.GetPredictorImplementation()
	g.Expect(implementation).Should(gomega.BeNil())
}
