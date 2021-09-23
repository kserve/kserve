package types

import (
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kserve/kserve/tools/tf2openapi/generated/framework"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
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
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "Logical name", "DT_COMPLEX128")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
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
	expectedTFTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Rank:  0,
	}
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

func TestTFTensorRowSchemaTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := expectedTFTensor()
	schema := tfTensor.RowSchema()
	expectedSchema := &openapi3.Schema{
		Type:     "array",
		MaxItems: func(u uint64) *uint64 { return &u }(3),
		MinItems: 3,
		Items: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type: "number",
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorRowSchemaScalarPerInstance(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1},
		Rank:  1,
	}
	schema := tfTensor.RowSchema()
	expectedSchema := &openapi3.Schema{
		Type: "number",
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorRowSchemaNested(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1, 1, 2},
		Rank:  3,
	}
	schema := tfTensor.RowSchema()
	expectedSchema := &openapi3.Schema{
		Type:     "array",
		MaxItems: func(u uint64) *uint64 { return &u }(1),
		MinItems: 1,
		Items: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:     "array",
				MaxItems: func(u uint64) *uint64 { return &u }(2),
				MinItems: 2,
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: "number",
					},
				},
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorRowSchemaZeroDimSize(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1, 0, 3},
		Rank:  3,
	}
	schema := tfTensor.RowSchema()
	expectedSchema := &openapi3.Schema{
		Type:     "array",
		MaxItems: func(u uint64) *uint64 { return &u }(0),
		MinItems: 0,
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorRowSchemaUnknownDimSize(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{-1, -1, 2},
		Rank:  3,
	}
	schema := tfTensor.RowSchema()
	expectedSchema := &openapi3.Schema{
		Type: "array",
		Items: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:     "array",
				MaxItems: func(u uint64) *uint64 { return &u }(2),
				MinItems: 2,
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: "number",
					},
				},
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorColSchemaUnknownRank(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Rank:  -1,
	}
	schema := tfTensor.ColSchema()
	expectedSchema := &openapi3.Schema{}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorColSchemaTypicalRowEquiv(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := expectedTFTensor()
	schema := tfTensor.ColSchema()
	expectedSchema := &openapi3.Schema{
		Type: "array",
		Items: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:     "array",
				MaxItems: func(u uint64) *uint64 { return &u }(3),
				MinItems: 3,
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: "number",
					},
				},
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFTensorColSchemaNonBatchable(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfTensor := TFTensor{
		Name:  "Logical name",
		DType: DtInt8,
		Shape: TFShape{1, 2},
		Rank:  2,
	}
	schema := tfTensor.ColSchema()
	expectedSchema := &openapi3.Schema{
		Type:     "array",
		MaxItems: func(u uint64) *uint64 { return &u }(1),
		MinItems: 1,
		Items: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:     "array",
				MaxItems: func(u uint64) *uint64 { return &u }(2),
				MinItems: 2,
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: "number",
					},
				},
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}
