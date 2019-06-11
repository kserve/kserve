package tfmodel

type TFSignatureDef struct {
	name    string
	inputs  [] *TFTensor
	outputs [] *TFTensor
}

func (m *TFSignatureDef) Name() string {
	return m.name
}

func (m *TFSignatureDef) SetName(name string) {
	m.name = name
}

func (m *TFSignatureDef) Outputs() []*TFTensor {
	return m.outputs
}

func (m *TFSignatureDef) SetOutputs(outputs []*TFTensor) {
	m.outputs = outputs
}

func (m *TFSignatureDef) Inputs() []*TFTensor {
	return m.inputs
}

func (m *TFSignatureDef) SetInputs(inputs []*TFTensor) {
	m.inputs = inputs
}

func (m *TFSignatureDef) Accept(w Visitor) {
	w.VisitSignatureDef(m)
	for _, x := range m.Inputs() {
		x.Accept(w)
	}
	for _, x := range m.Outputs() {
		x.Accept(w)
	}
}
