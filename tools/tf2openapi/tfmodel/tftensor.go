package tfmodel

/**
TFTensor represents a logical tensor. It contains the information from TensorInfo in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto] but is named after the user-facing input/output (hence being a logical
tensor and not an actual tensor).
 */
type TFTensor struct {
	//Name of the logical tensor
	Key   string

	// Data type contained in this tensor
	DType TFDType

	// Length of the shape is 0 when rank <= 0
	Shape TFShape

	// If rank = -1, the shape is unknown. Otherwise, rank corresponds to the number of dimensions in this tensor
	Rank int64
}

func (m *TFTensor) Accept(w Visitor) {
	w.VisitTensor(m)
}

type TFShape []int64

type TFDType int

const (
	// all the possible constants that can be JSON-ified according to
	// https://www.tensorflow.org/tfx/serving/api_rest#json_mapping
	DT_BOOL TFDType = iota
	DT_B64_STRING
	DT_STRING
	DT_INT8
	DT_UINT8
	DT_INT16
	DT_INT32
	DT_UINT32
	DT_INT64
	DT_UINT64
	DT_FLOAT
	DT_DOUBLE
)
