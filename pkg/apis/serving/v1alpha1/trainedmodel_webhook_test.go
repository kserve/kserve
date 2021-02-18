package v1alpha1

import (
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func makeTestTrainModel() TrainedModel {
	tm := TrainedModel{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrainedModel",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: TrainedModelSpec{
			InferenceService: "Parent",
			Model: ModelSpec{
				StorageURI: "example.com/sklearn/iris",
				Framework:  "sklearn",
			},
		},
	}

	quantity, err := resource.ParseQuantity("100Mi")
	if err == nil {
		tm.Spec.Model.Memory = quantity
	}

	return tm
}

func TestTrainedModelCreation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	g.Expect(tm.ValidateCreate()).Should(gomega.Succeed())
}

func TestUpdateMutableSpec(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	old := tm.DeepCopyObject()
	tm.Spec.InferenceService = "parent2"
	tm.Spec.Model.StorageURI = "example2.com/sklearn/iris"
	tm.Spec.Model.Framework = "sklearn2"
	g.Expect(tm.ValidateUpdate(old)).Should(gomega.Succeed())
}

func TestUpdateWithInvalidName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	old := tm.DeepCopyObject()
	tm.Name = "1abc"
	g.Expect(tm.ValidateUpdate(old)).ShouldNot(gomega.Succeed())
}

func TestUpdateImmutableMemory(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	old := tm.DeepCopyObject()
	tm.Spec.Model.Memory = resource.Quantity{Format: "300Mi"}
	g.Expect(tm.ValidateUpdate(old)).ShouldNot(gomega.Succeed())
}

func TestGoodName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	tm.Name = "abc-123"
	g.Expect(tm.ValidateCreate()).Should(gomega.Succeed())
}

func TestRejectBadNameStartWithNumber(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	tm.Name = "1abcde"
	g.Expect(tm.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestRejectBadNameIncludeDot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	tm.Name = "abc.de"
	g.Expect(tm.ValidateCreate()).ShouldNot(gomega.Succeed())
}

func TestDeleteTrainedModel(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tm := makeTestTrainModel()
	g.Expect(tm.ValidateDelete()).Should(gomega.Succeed())
}
