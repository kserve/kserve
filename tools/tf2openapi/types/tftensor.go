package types

/**
TFTensor represents a logical tensor. It contains the information from TensorInfo in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto] but is named after the user-facing input/output (hence being a logical
tensor and not an actual tensor).
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	fw "github.com/kserve/kserve/tools/tf2openapi/generated/framework"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
)

const B64KeySuffix string = "_bytes"

type TFTensor struct {
	// Name of the logical tensor
	Name string

	// Data type contained in this tensor
	DType TFDType

	// Length of the shape is rank when rank >= 0, nil otherwise
	Shape TFShape

	// If rank = -1, the shape is unknown. Otherwise, rank corresponds to the number of dimensions in this tensor
	Rank int64
}

type TFShape []int64

func NewTFTensor(name string, tensor *pb.TensorInfo) (TFTensor, error) {
	// TODO need to confirm whether there is a default shape when TensorShape is nil
	tfDType, err := NewTFDType(name, tensor.Dtype.String())
	if err != nil {
		return TFTensor{}, err
	}
	if tensor.TensorShape == nil || tensor.TensorShape.UnknownRank {
		return TFTensor{
			Name:  name,
			DType: tfDType,
			Rank:  -1,
		}, nil
	}
	if tensor.TensorShape.Dim == nil {
		return TFTensor{
			Name:  name,
			DType: tfDType,
			Rank:  0,
		}, nil
	}
	tfShape := NewTFShape(tensor.TensorShape.Dim)
	return TFTensor{
		// For both sparse & dense tensors
		Name:  name,
		DType: tfDType,
		Shape: tfShape,
		Rank:  int64(len(tfShape)),
	}, nil
}

func NewTFShape(dimensions []*fw.TensorShapeProto_Dim) TFShape {
	tfShape := TFShape{}
	for _, d := range dimensions {
		tfShape = append(tfShape, d.Size)
	}
	return tfShape
}

func (t *TFTensor) RowSchema() *openapi3.Schema {
	// tensor can only have row schema if it's batchable
	// ignore the 0th dimension: it is always -1 for batchable inputs
	return schema(1, t.Shape, t.Rank, t.DType)
}

func (t *TFTensor) ColSchema() *openapi3.Schema {
	if t.Rank == -1 {
		return openapi3.NewSchema()
	}
	return schema(0, t.Shape, t.Rank, t.DType)
}

func schema(dim int64, shape TFShape, rank int64, dType TFDType) *openapi3.Schema {
	if dim == rank {
		return dType.Schema()
	}
	if shape[dim] == -1 {
		return openapi3.NewArraySchema().WithItems(schema(dim+1, shape, rank, dType))
	}
	if shape[dim] == 0 {
		return openapi3.NewArraySchema().WithMaxItems(0)
	}
	return openapi3.NewArraySchema().WithMinItems(shape[dim]).WithMaxItems(shape[dim]).WithItems(schema(dim+1, shape, rank, dType))
}
