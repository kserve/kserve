package openapi3_test

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestExampleJSON(t *testing.T) {
	t.Log("Marshal *openapi3.Example to JSON")
	data, err := json.Marshal(example())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Unmarshal *openapi3.Example from JSON")
	docA := &openapi3.Example{}
	err = json.Unmarshal(exampleJSON, &docA)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Ensure representations match")
	dataA, err := json.Marshal(docA)
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(exampleJSON))
	require.JSONEq(t, string(data), string(dataA))
}

var exampleJSON = []byte(`
{
   "summary": "An example of a cat",
   "value": {
      "name": "Fluffy",
      "petType": "Cat",
      "color": "White",
      "gender": "male",
      "breed": "Persian"
   }
}
`)

func example() *openapi3.Example {
	value := map[string]string{
		"name":    "Fluffy",
		"petType": "Cat",
		"color":   "White",
		"gender":  "male",
		"breed":   "Persian",
	}
	return &openapi3.Example{
		Summary: "An example of a cat",
		Value:   value,
	}
}
