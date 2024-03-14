package generator

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kserve/kserve/tools/tf2openapi/types"
)

const (
	requestName         = "modelInput"
	responseName        = "modelOutput"
	requestRefTemplate  = "#/components/requestBodies/%s"
	responseRefTemplate = "#/components/responses/%s"
	pathTemplate        = "/v1/models/%s/versions/%s:predict"
)

func (g *Generator) tfServingOpenAPI(model types.TFSavedModel) (*openapi3.T, error) {
	requestSchema, responseSchema, err := model.Schema(g.metaGraphTags, g.sigDefKey)
	if err != nil {
		return &openapi3.T{}, err
	}
	return &openapi3.T{
		OpenAPI: "3.0.0",
		Components: &openapi3.Components{
			Responses: map[string]*openapi3.ResponseRef{
				responseName: {
					Value: &openapi3.Response{
						Description: proto.String("Model output"),
						Content:     openapi3.NewContentWithJSONSchema(responseSchema),
					},
				},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				requestName: {
					Value: &openapi3.RequestBody{
						Content: openapi3.NewContentWithJSONSchema(requestSchema),
					},
				},
			},
		},
		Paths: openapi3.Paths{
			fmt.Sprintf(pathTemplate, g.name, g.version): &openapi3.PathItem{
				Post: &openapi3.Operation{
					RequestBody: &openapi3.RequestBodyRef{
						Ref: fmt.Sprintf(requestRefTemplate, requestName),
					},

					Responses: openapi3.Responses{
						"200": &openapi3.ResponseRef{
							Ref: fmt.Sprintf(responseRefTemplate, responseName),
						},
					},
				},
			},
		},
		Info: &openapi3.Info{
			Title:   "TFServing Predict Request API",
			Version: "1.0",
		},
	}, nil
}
