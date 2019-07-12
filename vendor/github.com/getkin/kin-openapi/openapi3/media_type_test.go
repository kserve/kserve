package openapi3_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestMediaTypeJSON(t *testing.T) {
	t.Log("Marshal *openapi3.MediaType to JSON")
	data, err := json.Marshal(mediaType())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Unmarshal *openapi3.MediaType from JSON")
	docA := &openapi3.MediaType{}
	err = json.Unmarshal(mediaTypeJSON, &docA)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Validate *openapi3.MediaType")
	err = docA.Validate(context.TODO())
	require.NoError(t, err)

	t.Log("Ensure representations match")
	dataA, err := json.Marshal(docA)
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(mediaTypeJSON))
	require.JSONEq(t, string(data), string(dataA))
}

var mediaTypeJSON = []byte(`
{
   "schema": {
      "description": "Some schema"
   },
   "encoding": {
      "someEncoding": {
         "contentType": "application/xml; charset=utf-8"
      }
   },
   "examples": {
      "someExample": {
         "value": {
            "name": "Some example"
         }
      }
   }
}
`)

func mediaType() *openapi3.MediaType {
	example := map[string]string{"name": "Some example"}
	return &openapi3.MediaType{
		Schema: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Description: "Some schema",
			},
		},
		Encoding: map[string]*openapi3.Encoding{
			"someEncoding": {
				ContentType: "application/xml; charset=utf-8",
			},
		},
		Examples: map[string]*openapi3.ExampleRef{
			"someExample": {
				Value: openapi3.NewExample(example),
			},
		},
	}
}
