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

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

func TestTensorflowDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kfsvc := TFExampleKFService.DeepCopy()
	kfsvc.Spec.Canary = &CanarySpec{ModelSpec: *kfsvc.Spec.Default.DeepCopy()}
	kfsvc.Spec.Canary.Tensorflow.RuntimeVersion = "1.11"
	kfsvc.Spec.Canary.Tensorflow.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	kfsvc.Default()

	g.Expect(kfsvc.Spec.Default.Tensorflow.RuntimeVersion).To(gomega.Equal(DefaultTensorflowServingVersion))
	g.Expect(kfsvc.Spec.Default.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(DefaultCPURequests))
	g.Expect(kfsvc.Spec.Default.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(DefaultMemoryRequests))
	g.Expect(kfsvc.Spec.Canary.Tensorflow.RuntimeVersion).To(gomega.Equal("1.11"))
	g.Expect(kfsvc.Spec.Canary.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(DefaultCPURequests))
	g.Expect(kfsvc.Spec.Canary.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}
