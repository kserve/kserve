package types

/**
TFMetaGraph contains meta information about the TensorFlow graph, i.e. the signature definitions of the graph.
It is the internal model representation for the MetaGraph defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
)

const PredictReqSigDefMethod string = "tensorflow/serving/predict"

type TFMetaGraph struct {
	SignatureDefs [] TFSignatureDef
}

func NewTFMetaGraph(metaGraph pb.MetaGraphDef) TFMetaGraph {
	return TFMetaGraph{}
}

func extractSigDefs(sigDefs map[string]*pb.SignatureDef) []TFSignatureDef {
	return []TFSignatureDef{}
}

func (t *TFMetaGraph) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
