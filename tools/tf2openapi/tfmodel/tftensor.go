package tfmodel

type TFTensor struct {
	dType TFDType

	// Length of the shape is 0 when rank <= 0
	shape TFShape

	// If rank = -1, the shape is unknown. Otherwise, rank corresponds to the number of dimensions in this tensor
	rank  int64
	key   string
}

func (m *TFTensor) Rank() int64 {
	return m.rank
}

func (m *TFTensor) SetRank(rank int64) {
	m.rank = rank
}

func (m *TFTensor) Key() string {
	return m.key
}

func (m *TFTensor) SetKey(key string) {
	m.key = key
}

func (m *TFTensor) DType() TFDType {
	return m.dType
}

func (m *TFTensor) SetDType(dType TFDType) {
	m.dType = dType
}

func (m *TFTensor) Shape() TFShape {
	return m.shape
}

func (m *TFTensor) SetShape(shape TFShape) {
	m.shape = shape
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
