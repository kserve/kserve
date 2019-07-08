package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func expectedTFTensor() TFTensor {
	return TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1, 3},
		Rank:  2,
	}
}

func expectedTFTensorWithUnknowns() TFTensor {
	return TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Rank:  -1,
	}
}

/* Fake protobuf structs to use as test inputs */
func dimsPb() []*framework.TensorShapeProto_Dim {
	return []*framework.TensorShapeProto_Dim{
		{Size: -1},
		{Size: 3},
	}
}

func tensorInfoPb() *pb.TensorInfo {
	return &pb.TensorInfo{
		Dtype: framework.DataType_DT_INT8,
		TensorShape: &framework.TensorShapeProto{
			Dim:         dimsPb(),
			UnknownRank: false,
		},
	}
}

func TestCreateTFTensorTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := tensorInfoPb()
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := expectedTFTensor()
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFTensorUnsupportedDType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := tensorInfoPb()
	tensorInfoPb.Dtype = framework.DataType_DT_COMPLEX128
	_, err := NewTFTensor("Logical name", tensorInfoPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFTensorUnknownShape(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := tensorInfoPb()
	tensorInfoPb.TensorShape = nil
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := expectedTFTensorWithUnknowns()
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFTensorUnknownRank(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := tensorInfoPb()
	tensorInfoPb.TensorShape.UnknownRank = true
	tensorInfoPb.TensorShape.Dim = []*framework.TensorShapeProto_Dim{}
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := expectedTFTensorWithUnknowns()
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFTensorKnownRankUnknownDims(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tensorInfoPb := tensorInfoPb()
	tensorInfoPb.TensorShape.Dim = nil
	tfTensor, err := NewTFTensor("Logical name", tensorInfoPb)
	expectedTFTensor := expectedTFTensorWithUnknowns()
	g.Expect(tfTensor).Should(gomega.Equal(expectedTFTensor))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFShapeTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfShape := NewTFShape(dimsPb())
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
	expectedTFDType := DtString
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFDTypeBinary(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType, err := NewTFDType("tensor_name_bytes", "DT_STRING")
	expectedTFDType := DtB64String
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFDTypeUnsupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	_, err := NewTFDType("tensor_name", "BAD TYPE")
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
