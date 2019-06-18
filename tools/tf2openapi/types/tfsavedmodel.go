package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kfserving/pkg/utils"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
)

const ServingMetaGraphTag string = "serve"

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func NewTFSavedModel(model pb.SavedModel) TFSavedModel {
	if tfMetaGraphs := extractMetaGraphs(model.MetaGraphs); len(tfMetaGraphs) == 0 {
		panic("No graph to serve from SavedModel.")
	} else {
		return TFSavedModel{
			MetaGraphs: tfMetaGraphs,
		}
	}
}

func extractMetaGraphs(metaGraphs []*pb.MetaGraphDef) []TFMetaGraph {
	tfMetaGraphs := []TFMetaGraph{}
	for _, metaGraph := range metaGraphs {
		if utils.Includes(metaGraph.MetaInfoDef.Tags, ServingMetaGraphTag) {
			tfMetaGraphs = append(tfMetaGraphs, NewTFMetaGraph(*metaGraph))
		}
	}
	return tfMetaGraphs
}

func (t *TFSavedModel) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
