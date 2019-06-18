package generator

import (
	"github.com/kfserving/tools/tf2openapi/types"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
)

func GenerateOpenAPI(model pb.SavedModel) string {
	schema := types.NewTFSavedModel(model).Schema()
	// TODO logic for generating API
	return ""
}
