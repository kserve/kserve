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
	DtUint64
	DtFloat
	DtDouble
)

func NewTFTensor(key string, tensor pb.TensorInfo) TFTensor {
	if tensor.TensorShape.UnknownRank {
		return TFTensor{
			Key:   key,
			DType: NewTFDType(tensor.Dtype.String(), key),
			Rank:  -1,
		}
	} else {
		tfShape := NewTFShape(tensor.TensorShape.Dim)
		return TFTensor{
			Key:   key,
			DType: NewTFDType(tensor.Dtype.String(), key),
			Shape: tfShape,
			Rank:  int64(len(tfShape)),
		}
	}
}

func NewTFShape(dimensions []*fw.TensorShapeProto_Dim) TFShape {
	tfShape := TFShape{}
	for _, d := range dimensions {
		tfShape = append(tfShape, d.Size)
	}
	return tfShape
}

func NewTFDType(dType string, name string) TFDType {
	switch dType {
	case "DT_BOOL":
		return DtBool
	case "DT_STRING":
		if strings.HasSuffix(name, B64KeySuffix) {
			return DtB64String
		} else {
			return DtString
		}
	case "DT_INT8":
		return DtInt8
	case "DT_UINT8":
		return DtUInt8
	case "DT_INT16":
		return DtInt16
	case "DT_INT32":
		return DtInt32
	case "DT_UINT32":
		return DtUInt32
	case "DT_INT64":
		return DtInt64
	case "DT_UINT64":
		return DtUint64
	case "DT_FLOAT":
		return DtFloat
	case "DT_DOUBLE":
		return DtDouble
	default:
		panic("Unsupported data type for generating payloads")
	}
}

func (t *TFTensor) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
