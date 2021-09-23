package types

/**
TFMetaGraph contains meta information about the TensorFlow graph, i.e. the signature definitions of the graph.
It is the internal model representation for the MetaGraph defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
)

type TFMetaGraph struct {
	SignatureDefs []TFSignatureDef
	Tags          []string
}

// Known error messages
const (
	SignatureDefNotFoundError = "SignatureDef (%s) not found in specified MetaGraph"
)

func NewTFMetaGraph(metaGraph *pb.MetaGraphDef) (TFMetaGraph, error) {
	tfMetaGraph := TFMetaGraph{
		SignatureDefs: []TFSignatureDef{},
		Tags:          metaGraph.MetaInfoDef.Tags,
	}
	for key, definition := range metaGraph.SignatureDef {
		tfSigDef, err := NewTFSignatureDef(key, definition.MethodName, definition.Inputs, definition.Outputs)
		if err != nil {
			return TFMetaGraph{}, err
		}
		tfMetaGraph.SignatureDefs = append(tfMetaGraph.SignatureDefs, tfSigDef)
	}
	return tfMetaGraph, nil
}

func (t *TFMetaGraph) Schema(sigDefKey string) (*openapi3.Schema, *openapi3.Schema, error) {
	for _, sigDef := range t.SignatureDefs {
		if sigDefKey == sigDef.Key {
			return sigDef.Schema()
		}
	}
	return &openapi3.Schema{}, &openapi3.Schema{}, fmt.Errorf(SignatureDefNotFoundError, sigDefKey)
}
