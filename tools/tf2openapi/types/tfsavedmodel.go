package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

const ServingMetaGraphTag string = "serve"

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func NewTFSavedModel(model pb.SavedModel) TFSavedModel {
	return TFSavedModel{}
}

func extractMetaGraphs(metaGraphs []*pb.MetaGraphDef) []TFMetaGraph {
	return []TFMetaGraph{}
}

func (t *TFSavedModel) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
