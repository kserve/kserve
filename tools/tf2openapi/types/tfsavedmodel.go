package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"sort"
)

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func NewTFSavedModel(model *pb.SavedModel) (TFSavedModel, error) {
	tfSavedModel := TFSavedModel{
		MetaGraphs: []TFMetaGraph{},
	}
	for _, metaGraph := range model.MetaGraphs {
		tfMetaGraph, err := NewTFMetaGraph(metaGraph)
		if err != nil {
			return TFSavedModel{}, err
		}
		tfSavedModel.MetaGraphs = append(tfSavedModel.MetaGraphs, tfMetaGraph)
	}
	return tfSavedModel, nil
}

func (t *TFSavedModel) Schema(metaGraphTags []string, sigDefKey string) (*openapi3.Schema, error) {
	for _, metaGraph := range t.MetaGraphs {
		sort.Strings(metaGraph.Tags)
		sort.Strings(metaGraphTags)
		if !cmp.Equal(metaGraphTags, metaGraph.Tags) {
			continue
		}
		schema, err := metaGraph.Schema(sigDefKey)
		if err != nil {
			return &openapi3.Schema{}, err
		}
		return schema, nil
	}
	return &openapi3.Schema{}, fmt.Errorf("model does not contain MetaGraph with tags %v", metaGraphTags)
}
