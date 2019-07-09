package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

const defaultSigDefKey = "serving_default"
const defaultTag = "serve"

type Generator struct {
	model         *pb.SavedModel
	name          string
	version       string
	metaGraphTags []string
	sigDefKey     string
}

func (g *Generator) WithName(name string) {
	g.name = name
}

func (g *Generator) WithVersion(version string) {
	g.version = version
}

func (g *Generator) WithMetaGraphTags(metaGraphTags []string) {
	g.metaGraphTags = metaGraphTags
}

func (g *Generator) WithSigDefKey(sigDefKey string) {
	g.sigDefKey = sigDefKey
}

func NewGenerator() Generator {
	return Generator{
		name:          "model",
		version:       "1",
		metaGraphTags: []string{defaultTag},
		sigDefKey:     defaultSigDefKey,
	}
}

func (g *Generator) GenerateOpenAPI(model *pb.SavedModel) (string, error) {
	tfModel, constructionErr := types.NewTFSavedModel(model)
	if constructionErr != nil {
		return "", constructionErr
	}
	spec, genErr := g.tfServingOpenAPI(tfModel)
	if genErr != nil {
		return "", fmt.Errorf("missing info to generate OpenAPI specification\n error: %s", genErr.Error())
	}
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
