package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

const requestName = "modelInput"
const responseName = "modelOutput"
const requestRefTemplate = "#/components/requestBodies/%s"
const responseRefTemplate = "#/components/responses/%s"
const pathTemplate = "/v1/models/%s/versions/%s:predict"

func TFServingOpenAPI(model types.TFSavedModel, name string, version string, metaGraphTags []string, sigDefKey string) (*openapi3.Swagger, error) {
	requestRef := fmt.Sprintf(requestRefTemplate, requestName)
	responseRef := fmt.Sprintf(responseRefTemplate, responseName)
	path := fmt.Sprintf(pathTemplate, name, version)
	schema, err := model.Schema(metaGraphTags, sigDefKey)
	if err != nil {
		return &openapi3.Swagger{}, err
	}

	return &openapi3.Swagger{
		OpenAPI: "3.0.0",
		Components: openapi3.Components{
			Responses: map[string]*openapi3.ResponseRef{
				responseName: {
					Value: &openapi3.Response{
						Description: "Model output",
					},
				},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				requestName: {
					Value: &openapi3.RequestBody{
						Content: openapi3.NewContentWithJSONSchema(schema),
					},
				},
			},
		},
		Paths: openapi3.Paths{
			path: &openapi3.PathItem{
				Post: &openapi3.Operation{
					RequestBody: &openapi3.RequestBodyRef{
						Ref: requestRef,
					},

					Responses: openapi3.Responses{
						"200": &openapi3.ResponseRef{
							Ref: responseRef,
						},
					},
				},
			},
		},
		Info: openapi3.Info{
			Title:   "TFServing Predict Request API",
			Version: "1.0",
		},
	}, nil
}
