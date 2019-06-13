package tfmodel

/**
Interface for the Visitor Pattern [https://en.wikipedia.org/wiki/Visitor_pattern]
*/
type Visitor interface {
	VisitSavedModel(node *TFSavedModel)
	VisitMetaGraph(node *TFMetaGraph)
	VisitSignatureDef(node *TFSignatureDef)
	VisitTensor(node *TFTensor)
}
