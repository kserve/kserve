package tfmodel

/**
TFMetaGraph contains meta information about the TensorFlow graph, i.e. the signature definitions of the graph.
It is the internal model representation for the MetaGraph defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
)

type TFMetaGraph struct {
	SignatureDefs [] TFSignatureDef
}

func (metaGraph *TFMetaGraph) Schema() *openapi3.Schema {
	metaGraphSchema := openapi3.NewOneOfSchema()
	for _, x := range metaGraph.SignatureDefs {
		// TODO may change for columnar format
		rowSchemaRef := openapi3.NewObjectSchema().WithProperty("instances", x.Schema()).NewRef()
		metaGraphSchema.OneOf = append(metaGraphSchema.OneOf, rowSchemaRef)
	}
	return metaGraphSchema
}
