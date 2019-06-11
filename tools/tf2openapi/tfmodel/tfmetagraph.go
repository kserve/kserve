package tfmodel

type TFMetaGraph struct {
	signatureDefs [] *TFSignatureDef
}

func (m *TFMetaGraph) SignatureDefs() [] *TFSignatureDef {
	return m.signatureDefs
}

func (m *TFMetaGraph) SetSignatureDefs(signatureDefs [] *TFSignatureDef) {
	m.signatureDefs = signatureDefs
}

func (m *TFMetaGraph) Accept(w Visitor) {
	w.VisitMetaGraph(m)
	for _, x := range m.SignatureDefs() {
		x.Accept(w)
	}
}
