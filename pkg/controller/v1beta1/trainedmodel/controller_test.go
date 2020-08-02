/*
Copyright 2020 kubeflow.org.

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

package trainedmodel

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	. "github.com/onsi/ginkgo"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func createTestTrainedModel(modelName string, namespace string) *v1beta1.TrainedModel {
	memory, _ := resource.ParseQuantity("1G")
	instance := &v1beta1.TrainedModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelName,
			Namespace: namespace,
		},
		Spec: v1beta1.TrainedModelSpec{
			InferenceService: "parent",
			PredictorModel: v1beta1.ModelSpec{
				StorageURI: "s3://test/mnist/export",
				Framework:  "pytorch",
				Memory:     memory,
			},
		},
	}
	return instance
}

var _ = Describe("v1beta1 TrainedModel controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a new TrainedModel", func() {
		It("Should add a model file to the multi-model configmap", func() {
			//TODO need to be implemented
		})
	})

	Context("When updating a TrainedModel", func() {
		It("Should update a model file to the multi-model configmap", func() {
			//TODO need to be implemented
		})
	})

	Context("When deleting a TrainedModel", func() {
		It("Should remove a model file to the multi-model configmap", func() {
			//TODO need to be implemented
		})
	})
})
