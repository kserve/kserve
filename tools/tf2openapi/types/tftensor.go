package types

/**
TFTensor represents a logical tensor. It contains the information from TensorInfo in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto] but is named after the user-facing input/output (hence being a logical
tensor and not an actual tensor).
*/
import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	fw "github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"strings"
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

type TFDType int

const (
	// all the possible constants that can be JSON-ified according to
	// https://www.tensorflow.org/tfx/serving/api_rest#json_mapping
	// along with a representation for B64 strings
	DtBool TFDType = iota
	DtB64String
	DtString
	DtInt8
	DtUInt8
	DtInt16
	DtInt32
	DtUInt32
	DtInt64
	DtUInt64
	DtFloat
	DtDouble
)

func NewTFTensor(name string, tensor *pb.TensorInfo) (TFTensor, error) {
	// TODO need to confirm whether there is a default shape when TensorShape is nil
	tfDType, err := NewTFDType(name, tensor.Dtype.String())
	if err != nil {
		return TFTensor{}, err
	}

	if tensor.TensorShape == nil || tensor.TensorShape.UnknownRank || tensor.TensorShape.Dim == nil {
		return TFTensor{
			Name:  name,
			DType: tfDType,
			Rank:  -1,
		}, nil
	}
	// If rank is known and the tensor is a scalar, len(Dim) = 0 so Dim is not nil
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

func stringType(name string) TFDType {
	if strings.HasSuffix(name, B64KeySuffix) {
		return DtB64String
	}
	return DtString
}

func NewTFDType(name string, dType string) (TFDType, error) {
	tfDType, ok := map[string]TFDType{
		"DT_BOOL":   DtBool,
		"DT_INT8":   DtInt8,
		"DT_UINT8":  DtUInt8,
		"DT_INT16":  DtInt16,
		"DT_INT32":  DtInt32,
		"DT_UINT32": DtUInt32,
		"DT_INT64":  DtInt64,
		"DT_UINT64": DtUInt64,
		"DT_FLOAT":  DtFloat,
		"DT_DOUBLE": DtDouble,
		"DT_STRING": stringType(name),
	}[dType]
	if !ok {
		return TFDType(0), fmt.Errorf("tensor (%s) contains unsupported data type (%s) for generating payloads", name, dType)
	}
	return tfDType, nil
}

// Corresponds to how each entry in an "instances" or "inputs" object should look
func (t *TFTensor) Schema(row bool) *openapi3.Schema {
	if row {
		// Ignore the 0th dimension because it is always -1 in row fmt to represent batchable input
		return Schema(1, t.Shape, t.Rank, t.DType)
	}
	if t.Rank == -1 {
		// accept any schema if rank is unknown
		return openapi3.NewSchema()
	}
	return Schema(0, t.Shape, t.Rank, t.DType)
}

func Schema(dim int64, shape TFShape, rank int64, dType TFDType) *openapi3.Schema {
	if dim == rank {
		return dType.Schema()
	} else {
		if shape[dim] == -1 {
			// unknown length in this dimension
			return openapi3.NewArraySchema().WithItems(Schema(dim+1, shape, rank, dType))
		} else {
			return openapi3.NewArraySchema().WithLength(shape[dim]).WithItems(Schema(dim+1, shape, rank, dType))
		}
	}
}

func (t *TFDType) Schema() *openapi3.Schema {
	schema, ok := map[TFDType]*openapi3.Schema{
		DtBool:      openapi3.NewBoolSchema(),
		DtString:    openapi3.NewStringSchema(),
		DtB64String: openapi3.NewObjectSchema().WithProperty("b64", openapi3.NewStringSchema()),
		// JSON should be a decimal number for Ints and UInts
		DtInt8:   openapi3.NewFloat64Schema(),
		DtUInt8:  openapi3.NewFloat64Schema(),
		DtInt16:  openapi3.NewFloat64Schema(),
		DtInt32:  openapi3.NewFloat64Schema(),
		DtUInt32: openapi3.NewFloat64Schema(),
		DtInt64:  openapi3.NewFloat64Schema(),
		DtUInt64: openapi3.NewFloat64Schema(),
		// OpenAPI does NOT support NaN, Inf, -Inf
		// unlike TFServing which permits using these values as numbers for Floats and Doubles
		// (https://www.tensorflow.org/tfx/serving/api_rest#json_conformance)
		DtFloat:  openapi3.NewFloat64Schema(),
		DtDouble: openapi3.NewFloat64Schema(),
	}[*t]
	if !ok {
		panic("Unsupported data type for generating payloads")
	}
	return schema
}
