package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

const defaultSigDefKey = "serving_default"

func GenerateOpenAPI(model *pb.SavedModel, name string, version string, sigDefKey string) (string, error) {
	// Create OpenAPI spec
	if sigDefKey == "" {
		sigDefKey = defaultSigDefKey
	}
	tfModel, constructionErr := types.NewTFSavedModel(model, sigDefKey)
	if constructionErr != nil {
		return "", constructionErr
	}
	spec := wrapOpenAPI(tfModel, name, version)
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

func wrapOpenAPI(model types.TFSavedModel, name string, version string) *openapi3.Swagger {
	path := fmt.Sprintf("/v1/models/%s/versions/%s:predict", name, version)
	return &openapi3.Swagger{
		OpenAPI: "3.0.0",
		Components: openapi3.Components{
			Responses: map[string]*openapi3.ResponseRef{
				"modelOutput": {
					Value: &openapi3.Response{
						Description: "Model output",
					},
				},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				"modelInput": {
					Value: &openapi3.RequestBody{
						Content: openapi3.NewContentWithJSONSchema(model.Schema()),
					},
				},
			},
		},
		Paths: openapi3.Paths{
			path: &openapi3.PathItem{
				Post: &openapi3.Operation{
					RequestBody: &openapi3.RequestBodyRef{
						Ref: "#/components/requestBodies/modelInput",
					},

					Responses: openapi3.Responses{
						"200": &openapi3.ResponseRef{
							Ref: "#/components/responses/modelOutput",
						},
					},
				},
			},
		},
		Info: openapi3.Info{
			Title:   "TFServing Predict Request API",
			Version: "1.0",
		},
	}
}
