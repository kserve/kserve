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
