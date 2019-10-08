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
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTensorflowDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	isvc := InferenceService{
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
	isvc.Spec.Canary = isvc.Spec.Default.DeepCopy()
	isvc.Spec.Canary.Predictor.Tensorflow.RuntimeVersion = "1.11"
	isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests = v1.ResourceList{v1.ResourceMemory: resource.MustParse("3Gi")}
	isvc.Default()

	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal(DefaultTensorflowRuntimeVersion))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(DefaultCPU))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(DefaultMemory))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Limits[v1.ResourceCPU]).To(gomega.Equal(DefaultCPU))
	g.Expect(isvc.Spec.Default.Predictor.Tensorflow.Resources.Limits[v1.ResourceMemory]).To(gomega.Equal(DefaultMemory))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.RuntimeVersion).To(gomega.Equal("1.11"))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests[v1.ResourceCPU]).To(gomega.Equal(DefaultCPU))
	g.Expect(isvc.Spec.Canary.Predictor.Tensorflow.Resources.Requests[v1.ResourceMemory]).To(gomega.Equal(resource.MustParse("3Gi")))
}
