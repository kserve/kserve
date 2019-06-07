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

package v1alpha1

import (
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

func TestAcceptGoodRuntimeVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorflow := TensorflowSpec{
		RuntimeVersion: DefaultTensorflowRuntimeVersion,
	}
	g.Expect(tensorflow.Validate()).Should(gomega.Succeed())
}

func TestRejectBadRuntimeVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorflow := TensorflowSpec{
		RuntimeVersion: "",
	}
	g.Expect(tensorflow.Validate()).Should(gomega.MatchError(InvalidTensorflowRuntimeVersionError))
}

func TestRejectGPUResourcesExcluded(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorflow := TensorflowSpec{
		RuntimeVersion: DefaultTensorflowRuntimeVersionGPU,
	}
	g.Expect(tensorflow.Validate()).Should(gomega.MatchError(InvalidTensorflowRuntimeExcludesGPU))
}

func TestRejectGPUResourcesIncluded(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorflow := TensorflowSpec{
		RuntimeVersion: DefaultTensorflowRuntimeVersion,
		Resources: v1.ResourceRequirements{
			Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
		},
	}
	g.Expect(tensorflow.Validate()).Should(gomega.MatchError(InvalidTensorflowRuntimeIncludesGPU))
}
