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
	SignatureDef TFSignatureDef
}

func NewTFMetaGraph(metaGraph *pb.MetaGraphDef, sigDefKey string) (TFMetaGraph, error) {
	sigDef, ok := metaGraph.SignatureDef[sigDefKey]
	if !ok {
		return TFMetaGraph{}, fmt.Errorf("model does not contain desired SignatureDef (%s)", sigDefKey)
	}
	tfSigDef, err := NewTFSignatureDef(sigDefKey, sigDef.Inputs, sigDef.Outputs)
	if err != nil {
		return TFMetaGraph{}, err
	}

	return TFMetaGraph{
		SignatureDef: tfSigDef,
	}, nil
}

func (t *TFMetaGraph) Schema() *openapi3.Schema {
	return t.SignatureDef.Schema()
}
