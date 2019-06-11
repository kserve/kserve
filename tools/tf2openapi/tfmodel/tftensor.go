package tfmodel

type TFTensor struct {
	dType DTypeInternal
	shape ShapeInternal
	key   string
}

func (m *TFTensor) Key() string {
	return m.key
}

func (m *TFTensor) SetKey(key string) {
	m.key = key
}

func (m *TFTensor) DType() DTypeInternal {
	return m.dType
}

func (m *TFTensor) SetDType(dType DTypeInternal) {
	m.dType = dType
}

func (m *TFTensor) Shape() ShapeInternal {
	return m.shape
}

func (m *TFTensor) SetShape(shape ShapeInternal) {
	m.shape = shape
}

func (m *TFTensor) Accept(w Visitor) {
	w.VisitTensor(m)
}

type ShapeInternal []int

type DTypeInternal int

const (
	// all the possible constants that can be JSON-ified according to
	// https://www.tensorflow.org/tfx/serving/api_rest#json_mapping
	DT_BOOL DTypeInternal = iota + 1
	DT_STRING
	DT_INT8
	DT_UINT8
	DTI_INT16
	DT_INT32
	DT_UINT32
	DT_INT64
	DT_UINT64
	DT_FLOAT
	DT_DOUBLE
)
