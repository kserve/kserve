package generator

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
	"log"
)

const defaultSigDefKey = "serving_default"

func GenerateOpenAPI(model *pb.SavedModel, name string, version string, sigDefKey string) (string, error) {
	if sigDefKey == "" {
		sigDefKey = defaultSigDefKey
	}
	m, err := types.NewTFSavedModel(model, sigDefKey)
	if err != nil {
		return "", err
	}
	schema := m.Schema()
	response := &openapi3.Response{
		Description: "Model output",
	}
	requestBody := &openapi3.RequestBody{
		Content: openapi3.NewContentWithJSONSchema(schema),
	}
	log.Print(requestBody)
	swagger := &openapi3.Swagger{
		OpenAPI: "3.0.0",
		Components: openapi3.Components{
			Responses: map[string]*openapi3.ResponseRef{
				"modelOutput": {
					Value: response,
				},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				"modelInput": {
					Value: requestBody,
				},
			},
		},
		Paths: openapi3.Paths{
			fmt.Sprintf("/v1/models/%s/versions/%s:predict", name, version): &openapi3.PathItem{
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
	if json, err := (*swagger).MarshalJSON(); err != nil {
		panic("Unable to unmarshal json string")
	} else {
		log.Println(string(json))
		loader := openapi3.NewSwaggerLoader()
		docA, err := loader.LoadSwaggerFromData(json)
		if err != nil {
			log.Fatalln(err)

		} else {
			e := docA.Validate(loader.Context)
			log.Println(e)
			log.Println("HELLO VALIDATED!")
		}
		return string(json), nil
	}
}
