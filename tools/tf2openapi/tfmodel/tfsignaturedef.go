package tfmodel

/** TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
type TFSignatureDef struct {
	Name    string
	Inputs  [] *TFTensor
	Outputs [] *TFTensor
}

func (m *TFSignatureDef) Accept(w Visitor) {
	w.VisitSignatureDef(m)
	for _, x := range m.Inputs {
		x.Accept(w)
	}
	for _, x := range m.Outputs {
		x.Accept(w)
	}
}
