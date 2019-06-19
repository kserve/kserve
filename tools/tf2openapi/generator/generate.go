package generator

import (
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

func GenerateOpenAPI(model pb.SavedModel) string {
	m := types.NewTFSavedModel(model)
	m.Schema()

	// TODO logic for generating API
	return ""
}
