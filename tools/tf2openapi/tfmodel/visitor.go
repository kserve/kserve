package tfmodel

type Visitor interface {
	VisitSavedModel(node *TFSavedModel)
	VisitMetaGraph(node *TFMetaGraph)
	VisitSignatureDef(node *TFSignatureDef)
	VisitTensor(node *TFTensor)
}
