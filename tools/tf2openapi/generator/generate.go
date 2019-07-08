package generator

import (
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

const defaultSigDefKey = "serving_default"

func GenerateOpenAPI(model *pb.SavedModel, sigDefKey string) (string, error) {
	if sigDefKey == "" {
		sigDefKey = defaultSigDefKey
	}
	// TODO logic for generating API
	return "", nil
}
