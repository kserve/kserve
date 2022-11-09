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

package v1alpha1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestTrainedModelList_TotalRequestedMemory(t *testing.T) {
	list := TrainedModelList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items: []TrainedModel{
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-model-1",
				},
				Spec: TrainedModelSpec{
					Model: ModelSpec{
						StorageURI: "http://example.com/",
						Framework:  "sklearn",
						Memory:     resource.MustParse("1Gi"),
					},
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-model-2",
				},
				Spec: TrainedModelSpec{
					Model: ModelSpec{
						StorageURI: "http://example.com/",
						Framework:  "sklearn",
						Memory:     resource.MustParse("1Gi"),
					},
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-model-3",
				},
				Spec: TrainedModelSpec{
					Model: ModelSpec{
						StorageURI: "http://example.com/",
						Framework:  "sklearn",
						Memory:     resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
	res := list.TotalRequestedMemory()
	expected := resource.MustParse("3Gi")
	if res != expected {
		fmt.Println(fmt.Errorf("expected %v got %v", expected, res))
	}
}
