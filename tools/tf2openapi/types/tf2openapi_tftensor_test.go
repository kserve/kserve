package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func makeExpectedTFTensor() TFTensor {
	return TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1, 3},
		Rank:  2,
	}
}

func makeExpectedTFTensorWithUnknowns() TFTensor {
	return TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Rank:  -1,
	}
}

/* Fake protobuf structs to use as test inputs */
func makeDimsPb() []*framework.TensorShapeProto_Dim {
	return []*framework.TensorShapeProto_Dim{
		{
			Size: -1,
		},
		{
			Size: 3,
		},
	}
}

func makeTensorInfoPb() *pb.TensorInfo {
	return &pb.TensorInfo{
		Dtype: framework.DataType_DT_INT8,
		TensorShape: &framework.TensorShapeProto{
			Dim:         makeDimsPb(),
			UnknownRank: false,
		},
	}
}

func TestCreateTFTensorTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := makeTensorInfoPb()
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := makeExpectedTFTensor()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
}

func TestCreateTFTensorUnsupportedDType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := makeTensorInfoPb()
	tensorInfoPb.Dtype = framework.DataType_DT_COMPLEX128
	_, err := NewTFTensor("Logical name", tensorInfoPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFTensorUnknownShape(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := makeTensorInfoPb()
	tensorInfoPb.TensorShape = nil
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := makeExpectedTFTensorWithUnknowns()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
}

func TestCreateTFTensorUnknownRank(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := makeTensorInfoPb()
	tensorInfoPb.TensorShape.UnknownRank = true
	tensorInfoPb.TensorShape.Dim = []*framework.TensorShapeProto_Dim{}
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := makeExpectedTFTensorWithUnknowns()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
}

func TestCreateTFTensorKnownRankUnknownDims(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := makeTensorInfoPb()
	tensorInfoPb.TensorShape.Dim = nil
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := makeExpectedTFTensorWithUnknowns()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
}

func TestCreateTFShapeTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfShape := NewTFShape(makeDimsPb())
	expectedTFShape := TFShape{-1, 3}
	g.Expect(tfShape).Should(gomega.Equal(expectedTFShape))
}

func TestCreateTFShapeEmpty(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfShape := NewTFShape([]*framework.TensorShapeProto_Dim{})
	expectedTFShape := TFShape{}
	g.Expect(tfShape).Should(gomega.Equal(expectedTFShape))
}

func TestCreateTFDTypeTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType, err := NewTFDType("tensor_name", "DT_STRING")
	g.Expect(err).Should(gomega.BeNil())
	expectedTFDType := DtString
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
}

func TestCreateTFDTypeBinary(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType, err := NewTFDType("tensor_name_bytes", "DT_STRING")
	expectedTFDType := DtB64String
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
}

func TestCreateTFDTypeUnsupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	_, err := NewTFDType("tensor_name", "BAD TYPE")
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
