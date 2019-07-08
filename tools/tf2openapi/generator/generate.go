package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

const defaultSigDefKey = "serving_default"
const defaultTag = "serve"

type Factory struct {
	model         *pb.SavedModel
	name          string
	version       string
	metaGraphTags []string
	sigDefKey     string
}

func (f *Factory) WithName(name string) *Factory {
	f.name = name
	return f
}

func (f *Factory) WithVersion(version string) *Factory {
	f.version = version
	return f
}

func (f *Factory) WithMetaGraphTags(metaGraphTags []string) *Factory {
	f.metaGraphTags = metaGraphTags
	return f
}

func (f *Factory) WithSigDefKey(sigDefKey string) *Factory {
	f.sigDefKey = sigDefKey
	return f
}

func NewGenerator(model *pb.SavedModel) Factory {
	return Factory{
		model:         model,
		name:          "model",
		version:       "1",
		metaGraphTags: []string{defaultTag},
		sigDefKey:     defaultSigDefKey,
	}
}

func (f *Factory) GenerateOpenAPI() (string, error) {
	tfModel, constructionErr := types.NewTFSavedModel(f.model)
	if constructionErr != nil {
		return "", constructionErr
	}
	spec := TFServingOpenAPI(tfModel, f.name, f.version)
	json, marshallingErr := (*spec).MarshalJSON()
	if marshallingErr != nil {
		return "", fmt.Errorf("generated OpenAPI specification is corrupted\n error: %s \n specification: %s", marshallingErr.Error(), json)
	}
	if validationErr := validateOpenAPI(json); validationErr != nil {
		return "", validationErr
	}
	return string(json), nil
}

func validateOpenAPI(json []byte) error {
	loader := openapi3.NewSwaggerLoader()
	swagger, err := loader.LoadSwaggerFromData(json)
	if err != nil {
		return fmt.Errorf("generated OpenAPI specification (below) is corrupted\n error: %s \n specification: %s", err.Error(), json)
	}
	err = swagger.Validate(loader.Context)
	if err != nil {
		return fmt.Errorf("generated OpenAPI specification (below) is constructed incorrectly\n error: %s \n specification: %s", err.Error(), json)
	}
	return nil
}
