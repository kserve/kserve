package openapi3_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestEncodingJSON(t *testing.T) {
	t.Log("Marshal *openapi3.Encoding to JSON")
	data, err := json.Marshal(encoding())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Unmarshal *openapi3.Encoding from JSON")
	docA := &openapi3.Encoding{}
	err = json.Unmarshal(encodingJSON, &docA)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log("Validate *openapi3.Encoding")
	err = docA.Validate(context.TODO())
	require.NoError(t, err)

	t.Log("Ensure representations match")
	dataA, err := json.Marshal(docA)
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(encodingJSON))
	require.JSONEq(t, string(data), string(dataA))
}

var encodingJSON = []byte(`
{
  "contentType": "application/json",
  "headers": {
    "someHeader": {}
  },
  "style": "form",
  "explode": true,
  "allowReserved": true
}
`)

func encoding() *openapi3.Encoding {
	explode := true
	return &openapi3.Encoding{
		ContentType: "application/json",
		Headers: map[string]*openapi3.HeaderRef{
			"someHeader": {
				Value: &openapi3.Header{},
			},
		},
		Style:         "form",
		Explode:       &explode,
		AllowReserved: true,
	}
}

func TestEncodingSerializationMethod(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	testCases := []struct {
		name string
		enc  *openapi3.Encoding
		want *openapi3.SerializationMethod
	}{
		{
			name: "default",
			want: &openapi3.SerializationMethod{Style: openapi3.SerializationForm, Explode: true},
		},
		{
			name: "encoding with style",
			enc:  &openapi3.Encoding{Style: openapi3.SerializationSpaceDelimited},
			want: &openapi3.SerializationMethod{Style: openapi3.SerializationSpaceDelimited, Explode: true},
		},
		{
			name: "encoding with explode",
			enc:  &openapi3.Encoding{Explode: boolPtr(true)},
			want: &openapi3.SerializationMethod{Style: openapi3.SerializationForm, Explode: true},
		},
		{
			name: "encoding with no explode",
			enc:  &openapi3.Encoding{Explode: boolPtr(false)},
			want: &openapi3.SerializationMethod{Style: openapi3.SerializationForm, Explode: false},
		},
		{
			name: "encoding with style and explode ",
			enc:  &openapi3.Encoding{Style: openapi3.SerializationSpaceDelimited, Explode: boolPtr(false)},
			want: &openapi3.SerializationMethod{Style: openapi3.SerializationSpaceDelimited, Explode: false},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.enc.SerializationMethod()
			require.True(t, reflect.DeepEqual(got, tc.want), "got %#v, want %#v", got, tc.want)
		})
	}
}
