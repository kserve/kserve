package generator

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
	"github.com/kserve/kserve/tools/tf2openapi/types"
)

const (
	DefaultSigDefKey = "serving_default"
	DefaultTag       = "serve"
)

// Known error messages
const (
	SpecGenerationError     = "missing info to generate OpenAPI specification\n error: %s"
	UnmarshallableSpecError = "generated OpenAPI specification is corrupted\n error: %s \n specification: %s"
	UnloadableSpecError     = "generated OpenAPI specification (below) is corrupted\n error: %s \n specification: %s"
	InvalidSpecError        = "generated OpenAPI specification (below) is constructed incorrectly\n error: %s \n specification: %s"
)

type Generator struct {
	name          string
	version       string
	metaGraphTags []string
	sigDefKey     string
}

type Builder struct {
	Generator
}

func (b *Builder) Build() Generator {
	if b.Generator.sigDefKey == "" {
		b.SetSigDefKey(DefaultSigDefKey)
	}
	if len(b.Generator.metaGraphTags) == 0 {
		b.SetMetaGraphTags([]string{DefaultTag})
	}
	return b.Generator
}

func (b *Builder) SetName(name string) {
	b.Generator.name = name
}

func (b *Builder) SetVersion(version string) {
	b.Generator.version = version
}

func (b *Builder) SetMetaGraphTags(metaGraphTags []string) {
	b.Generator.metaGraphTags = metaGraphTags
}

func (b *Builder) SetSigDefKey(sigDefKey string) {
	b.Generator.sigDefKey = sigDefKey
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
		panic(fmt.Errorf(UnmarshallableSpecError, marshallingErr.Error(), json))
	}
	if validationErr := validateOpenAPI(json); validationErr != nil {
		panic(validationErr)
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
