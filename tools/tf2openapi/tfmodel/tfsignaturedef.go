package tfmodel

/**
TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
)

type TFSignatureDef struct {
	Name    string
	Inputs  [] TFTensor
	Outputs [] TFTensor
}


func (sigDef *TFSignatureDef) Schema() *openapi3.Schema {
	// TODO add options for single tensor
	multiTensorSchema := *openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	sigDefSchema := openapi3.NewArraySchema().WithItems(&multiTensorSchema)
	for _, x := range sigDef.Inputs {
		sigDefSchema.Items.Value.Properties[x.Key] = x.Schema().NewRef()
	}
	return sigDefSchema
}