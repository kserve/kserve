package tfmodel

type TFSavedModel struct {
	metaGraphs [] *TFMetaGraph
}

func (m *TFSavedModel) MetaGraphs() [] *TFMetaGraph {
	return m.metaGraphs
}

func (m *TFSavedModel) SetMetaGraphs(metaGraphs [] *TFMetaGraph) {
	m.metaGraphs = metaGraphs
}

func (m *TFSavedModel) Accept(w Visitor) {
	w.VisitSavedModel(m)
	for _, x := range m.MetaGraphs() {
		x.Accept(w)
	}
}
