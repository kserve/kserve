package tfmodel

/** TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
type TFSavedModel struct {
	MetaGraphs [] *TFMetaGraph
}

func (m *TFSavedModel) Accept(w Visitor) {
	w.VisitSavedModel(m)
	for _, x := range m.MetaGraphs {
		x.Accept(w)
	}
}
