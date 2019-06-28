package types

/**
TFMetaGraph contains meta information about the TensorFlow graph, i.e. the signature definitions of the graph.
It is the internal model representation for the MetaGraph defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

type TFMetaGraph struct {
	SignatureDefs [] TFSignatureDef
}

func NewTFMetaGraph(metaGraph *pb.MetaGraphDef, sigDefKey string) (TFMetaGraph, error) {
	tfMetaGraph := TFMetaGraph{
		SignatureDefs: []TFSignatureDef{},
	}
	for key, definition := range metaGraph.SignatureDef {
		if key != sigDefKey {
			continue
		}
		tfSigDef, err := NewTFSignatureDef(key, definition.Inputs, definition.Outputs)
		if err != nil {
			return TFMetaGraph{}, err
		}
		tfMetaGraph.SignatureDefs = append(tfMetaGraph.SignatureDefs, tfSigDef)
		return tfMetaGraph, nil
	}
	// len(tfMetaGraph.SignatureDefs) is 0
	return TFMetaGraph{}, fmt.Errorf("model does not contain desired SignatureDef (%s)", sigDefKey)

}

func (t *TFMetaGraph) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
