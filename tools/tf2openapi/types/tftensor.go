package types

/**
TFTensor represents a logical tensor. It contains the information from TensorInfo in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto] but is named after the user-facing input/output (hence being a logical
tensor and not an actual tensor).
 */

import (
	fw "github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"strings"
)

const B64KeySuffix string = "_bytes"

type TFTensor struct {
	//Name of the logical tensor
	Key string

	// Data type contained in this tensor
	DType TFDType

	// Length of the shape is 0 when rank <= 0
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

func NewTFTensor(key string, tensor *pb.TensorInfo) TFTensor {
	if tensor.TensorShape == nil || tensor.TensorShape.UnknownRank {
		return TFTensor{
			Key:   key,
			DType: NewTFDType(tensor.Dtype.String(), key),
			Rank:  -1,
		}
	}
	tfShape := NewTFShape(tensor.TensorShape.Dim)
	return TFTensor{
		Key:   key,
		DType: NewTFDType(tensor.Dtype.String(), key),
		Shape: tfShape,
		Rank:  int64(len(tfShape)),
	}

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

func NewTFDType(dType string, name string) TFDType {
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
		panic("Unsupported data type for generating payloads")
	}
	return tfDType
}

func (t *TFTensor) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
