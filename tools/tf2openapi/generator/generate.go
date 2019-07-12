package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

// Default builder constants
const (
	defaultSigDefKey = "serving_default"
	defaultTag       = "serve"
)

// Known error messages
const (
	SpecGenerationError     = "missing info to generate OpenAPI specification\n error: %s"
	UnmarshallableSpecError = "generated OpenAPI specification is corrupted\n error: %s \n specification: %s"
	UnloadableSpecError     = "generated OpenAPI specification (below) is corrupted\n error: %s \n specification: %s"
	InvalidSpecError        = "generated OpenAPI specification (below) is constructed incorrectly\n error: %s \n specification: %s"
)

type Generator struct {
	model         *pb.SavedModel
	name          string
	version       string
	metaGraphTags []string
	sigDefKey     string
}

type GeneratorBuilder struct {
	Generator
}

func (gb *GeneratorBuilder) Build() Generator {
	if gb.Generator.sigDefKey == "" {
		gb.SetSigDefKey(defaultSigDefKey)
	}
	if len(gb.Generator.metaGraphTags) == 0 {
		gb.SetMetaGraphTags([]string{defaultTag})
	}
	return gb.Generator
}

func (g *GeneratorBuilder) SetName(name string) {
	g.Generator.name = name
}

func (g *GeneratorBuilder) SetVersion(version string) {
	g.Generator.version = version
}

func (g *GeneratorBuilder) SetMetaGraphTags(metaGraphTags []string) {
	g.Generator.metaGraphTags = metaGraphTags
}

func (g *GeneratorBuilder) SetSigDefKey(sigDefKey string) {
	g.Generator.sigDefKey = sigDefKey
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
		return "", fmt.Errorf(SpecGenerationError, genErr.Error())
	}
	json, marshallingErr := spec.MarshalJSON()
	if marshallingErr != nil {
		return "", fmt.Errorf(UnmarshallableSpecError, marshallingErr.Error(), json)
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
		return fmt.Errorf(UnloadableSpecError, err.Error(), json)
	}
	err = swagger.Validate(loader.Context)
	if err != nil {
		return fmt.Errorf(InvalidSpecError, err.Error(), json)
	}
	return nil
}
