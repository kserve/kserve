package tfmodel

import (
	"github.com/getkin/kin-openapi/openapi3"
	"log"
)

/** Implements the Visitor interface **/
type OpenAPIVisitor struct {
	savedModelRep *openapi3.Schema
	metaGraphRep  *openapi3.Schema
	sigDefRep     *openapi3.Schema
	tensorRep     *openapi3.Schema
}

func OpenAPIPrint(v OpenAPIVisitor) {

	swagger := &openapi3.Swagger{
		//Servers: openapi3.Servers{
		//	{
		//		URL: "http://example.com/api/",
		//	},
		//},
		Paths: openapi3.Paths{
			"models/${MODEL_NAME}[/versions/${MODEL_VERSION}]:predict": &openapi3.PathItem{
				Post: &openapi3.Operation{
					Parameters: openapi3.Parameters{
						{
							Value: &openapi3.Parameter{
								In:   "query",
								Name: "contentArg2",
								Content: openapi3.Content{
									"application/something_funny":
									openapi3.NewMediaType().WithSchema(v.savedModelRep),
								},
							},
						},
					},
				},
			},
		},
		Info: openapi3.Info{
			Title: "TFServing Predict Request API",
			Version: "3.0",
		},
	}
	jsonSerialized, _ := (*swagger).MarshalJSON()

	log.Println("************************")
	log.Println(string(jsonSerialized))
}

func (w *OpenAPIVisitor) VisitSavedModel(node *TFSavedModel) {
	w.savedModelRep = openapi3.NewOneOfSchema()
}

func (w *OpenAPIVisitor) VisitMetaGraph(node *TFMetaGraph) {
	w.metaGraphRep = openapi3.NewOneOfSchema()
	w.savedModelRep.OneOf = append(w.savedModelRep.OneOf, w.metaGraphRep.NewRef())
	log.Println("Metagraph")
	log.Println(w.metaGraphRep)
}

func (w *OpenAPIVisitor) VisitSignatureDef(node *TFSignatureDef) {
	log.Println("SigDef")
	multiTensorSchema := *openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))

	w.sigDefRep = openapi3.NewArraySchema().WithItems(&multiTensorSchema)
	// TODO may change for columnar format
	rowSchemaRef := openapi3.NewObjectSchema().WithProperty("instances", w.sigDefRep)

	w.metaGraphRep.OneOf = append(w.metaGraphRep.OneOf, rowSchemaRef.NewRef())
	log.Println(w.metaGraphRep)
}

func (w *OpenAPIVisitor) VisitTensor(node *TFTensor) {
	log.Println("Tensor")
	tensorConstraints := openapi3.NewStringSchema().NewRef()
	w.sigDefRep.Items.Value.Properties[node.Key] = tensorConstraints

	log.Println("metagraphrep", w.metaGraphRep)
}
