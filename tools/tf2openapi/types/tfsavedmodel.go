package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/pkg/utils"
	"log"
)

const ServingMetaGraphTag string = "serve"

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func NewTFSavedModel(model *pb.SavedModel) TFSavedModel {
	tfMetaGraphs := []TFMetaGraph{}
	for _, metaGraph := range model.MetaGraphs {
		if utils.Includes(metaGraph.MetaInfoDef.Tags, ServingMetaGraphTag) {
			tfMetaGraphs = append(tfMetaGraphs, NewTFMetaGraph(metaGraph))
		}
	}
	if len(tfMetaGraphs) == 0 {
		log.Fatalln("Model does not contain any servable MetaGraphs")
	}
	return TFSavedModel{
		MetaGraphs: tfMetaGraphs,
	}
}

func (t *TFSavedModel) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
