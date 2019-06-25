package generator

import (
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
	"log"
)

const defaultSigDefKey = "serving_default"

func GenerateOpenAPI(model *pb.SavedModel, sigDefKey string) (string, error) {
	if sigDefKey == "" {
		sigDefKey = defaultSigDefKey
	}
	m, err := types.NewTFSavedModel(model, sigDefKey)
	if err != nil {
		return "", err
	}
	schema := m.Schema()
	response := &openapi3.Response{
		Description: "Some response",
	}
	//complexArgSchema := openapi3.NewObjectSchema().
	//	WithProperty("name", openapi3.NewStringSchema()).
	//	WithProperty("id", openapi3.NewStringSchema().WithMaxLength(2))
	//schema = complexArgSchema
	requestBody := &openapi3.RequestBody{
		Content: openapi3.NewContentWithJSONSchema(schema),
	}
	log.Print(requestBody)
	swagger := &openapi3.Swagger{
		OpenAPI: "3.0.0",
		Components: openapi3.Components{
			Responses: map[string]*openapi3.ResponseRef{
				"someResponse": {
					Value: response,
				},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				"someRequestBody": {
					Value: requestBody,
				},
			},
		},
		Paths: openapi3.Paths{
			// TODO fix parameters and response, etc.
			"/models": &openapi3.PathItem{

				Post: &openapi3.Operation{

					RequestBody: &openapi3.RequestBodyRef{
						Ref: "#/components/requestBodies/someRequestBody",
					},

					Responses: openapi3.Responses{
						"200": &openapi3.ResponseRef{
							Ref: "#/components/responses/someResponse",
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
