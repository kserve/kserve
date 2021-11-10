package types

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"fmt"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
)

type TFSavedModel struct {
	MetaGraphs []TFMetaGraph
}

// Known error messages
const (
	MetaGraphNotFoundError = "model does not contain MetaGraph with tags %v"
)

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

func (t *TFSavedModel) Schema(metaGraphTags []string, sigDefKey string) (*openapi3.Schema, *openapi3.Schema, error) {
	for _, metaGraph := range t.MetaGraphs {
		if setEquals(metaGraphTags, metaGraph.Tags) {
			return metaGraph.Schema(sigDefKey)
		}
	}
	return &openapi3.Schema{}, &openapi3.Schema{}, fmt.Errorf(MetaGraphNotFoundError, metaGraphTags)
}

func setEquals(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)
	if cmp.Equal(a, b) {
		return true
	}
	return false
}
