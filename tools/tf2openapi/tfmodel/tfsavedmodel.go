package tfmodel

/**
TFSavedModel is the high level serialization format for TensorFlow saved models.
It is the internal model representation for the SavedModel defined in the TensorFlow repository
[tensorflow/core/protobuf/saved_model.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
)

type TFSavedModel struct {
	MetaGraphs [] TFMetaGraph
}

func (model *TFSavedModel) Schema() *openapi3.Schema {
	savedModelSchema := openapi3.NewOneOfSchema()
	for _, x := range model.MetaGraphs {
		savedModelSchema.OneOf = append(savedModelSchema.OneOf, x.Schema().NewRef())
	}
	return savedModelSchema
}

func OpenAPI(schema *openapi3.Schema) string{
	swagger := &openapi3.Swagger{
		OpenAPI: "3.0",
		//Servers: openapi3.Servers{
		//	{
		//		URL: "http://example.com/api/",
		//	},
		//},
		Paths: openapi3.Paths{
			// TODO fix parameters
			"models/${MODEL_NAME}[/versions/${MODEL_VERSION}]:predict": &openapi3.PathItem{
				Post: &openapi3.Operation{
					Parameters: openapi3.Parameters{
						{
							Value: &openapi3.Parameter{
								In:   "query",
								Name: "contentArg2",
								Content: openapi3.Content{
									"application/JSON":
									openapi3.NewMediaType().WithSchema(schema),
								},
							},
						},
					},
				},
			},
		},
		Info: openapi3.Info{
			Title: "TFServing Predict Request API",
			Version: "1.0",
		},
	}
	jsonSerialized, _ := (*swagger).MarshalJSON()

	return string(jsonSerialized)
}
