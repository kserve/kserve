package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"errors"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeflow/kfserving/pkg/utils"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

const ServingMetaGraphTag string = "serve"

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func NewTFSavedModel(model *pb.SavedModel) (TFSavedModel, error) {
	tfSavedModel := TFSavedModel{
		MetaGraphs: []TFMetaGraph{},
	}
	for _, metaGraph := range model.MetaGraphs {
		if !utils.Includes(metaGraph.MetaInfoDef.Tags, ServingMetaGraphTag) {
			continue
		}
		tfMetaGraph, err := NewTFMetaGraph(metaGraph)
		if err != nil {
			return TFSavedModel{}, err
		}
		tfSavedModel.MetaGraphs = append(tfSavedModel.MetaGraphs, tfMetaGraph)

	}
	if len(tfSavedModel.MetaGraphs) == 0 {
		return TFSavedModel{}, errors.New("model does not contain any servable MetaGraphs")
	}
	return tfSavedModel, nil
}

func (t *TFSavedModel) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
