package tfmodel

import "fmt"

/** Implements the Visitor interface **/
type OpenAPIVisitor struct {
}

func (w *OpenAPIVisitor) VisitSavedModel(node *TFSavedModel)     { fmt.Println("Saved Model") }
func (w *OpenAPIVisitor) VisitMetaGraph(node *TFMetaGraph)       { fmt.Println("Meta Graph") }
func (w *OpenAPIVisitor) VisitSignatureDef(node *TFSignatureDef) { fmt.Println("Sig Def") }
func (w *OpenAPIVisitor) VisitTensor(node *TFTensor)             { fmt.Println("Tensor") }
