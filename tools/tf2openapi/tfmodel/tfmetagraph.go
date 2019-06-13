package tfmodel

/**
TFMetaGraph contains meta information about the TensorFlow graph, i.e. the signature definitions of the graph.
It is the internal model representation for the MetaGraph defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
type TFMetaGraph struct {
	SignatureDefs [] TFSignatureDef
}

func (m *TFMetaGraph) Accept(w Visitor) {
	w.VisitMetaGraph(m)
	for _, x := range m.SignatureDefs {
		x.Accept(w)
	}
}
