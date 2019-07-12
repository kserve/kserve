package openapi3filter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/stretchr/testify/require"
)

type ExampleRequest struct {
	Method      string
	URL         string
	ContentType string
	Body        interface{}
}

type ExampleResponse struct {
	Status      int
	ContentType string
	Body        interface{}
}

func TestFilter(t *testing.T) {
	// Declare a schema for an object with name and id properties
	complexArgSchema := openapi3.NewObjectSchema().
		WithProperty("name", openapi3.NewStringSchema()).
		WithProperty("id", openapi3.NewStringSchema().WithMaxLength(2))
	complexArgSchema.Required = []string{"name", "id"}

	// Declare router
	swagger := &openapi3.Swagger{
		Servers: openapi3.Servers{
			{
				URL: "http://example.com/api/",
			},
		},
		Paths: openapi3.Paths{
			"/prefix/{pathArg}/suffix": &openapi3.PathItem{
				Post: &openapi3.Operation{
					Parameters: openapi3.Parameters{
						{
							Value: &openapi3.Parameter{
								In:     "path",
								Name:   "pathArg",
								Schema: openapi3.NewStringSchema().WithMaxLength(2).NewRef(),
							},
						},
						{
							Value: &openapi3.Parameter{
								In:     "query",
								Name:   "queryArg",
								Schema: openapi3.NewStringSchema().WithMaxLength(2).NewRef(),
							},
						},
						{
							Value: &openapi3.Parameter{
								In:   "query",
								Name: "contentArg",
								Content: openapi3.NewContentWithJSONSchema(complexArgSchema),
							},
						},
						{
							Value: &openapi3.Parameter{
								In:   "query",
								Name: "contentArg2",
								Content: openapi3.Content{
									"application/something_funny": openapi3.NewMediaType().WithSchema(complexArgSchema),
								},
							},
						},
					},
				},
			},
		},
	}

	router := openapi3filter.NewRouter().WithSwagger(swagger)
	expectWithDecoder := func(req ExampleRequest, resp ExampleResponse, decoder openapi3filter.ContentParameterDecoder) error {
		t.Logf("Request: %s %s", req.Method, req.URL)
		httpReq, _ := http.NewRequest(req.Method, req.URL, marshalReader(req.Body))
		httpReq.Header.Set("Content-Type", req.ContentType)

		// Find route
		route, pathParams, err := router.FindRoute(httpReq.Method, httpReq.URL)
		require.NoError(t, err)

		// Validate request
		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    httpReq,
			PathParams: pathParams,
			Route:      route,
			ParamDecoder: decoder,
		}
		if err := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput); err != nil {
			return err
		}
		t.Logf("Response: %d", resp.Status)
		responseValidationInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: requestValidationInput,
			Status:                 resp.Status,
			Header: http.Header{
				"Content-Type": []string{
					resp.ContentType,
				},
			},
		}
		if resp.Body != nil {
			data, err := json.Marshal(resp.Body)
			require.NoError(t, err)
			responseValidationInput.SetBodyBytes(data)
		}
		err = openapi3filter.ValidateResponse(context.TODO(), responseValidationInput)
		require.NoError(t, err)
		return err
	}
	expect := func(req ExampleRequest, resp ExampleResponse) error {
		return expectWithDecoder(req, resp, nil)
	}

	var err error
	var req ExampleRequest
	var resp ExampleResponse

	// Test paths
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix",
	}
	resp = ExampleResponse{
		Status: 200,
	}
	err = expect(req, resp)
	require.NoError(t, err)

	// Test query parameter openapi3filter
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/EXCEEDS_MAX_LENGTH/suffix",
	}
	err = expect(req, resp)
	require.IsType(t, &openapi3filter.RequestError{}, err)

	// Test query parameter openapi3filter
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?queryArg=a",
	}
	err = expect(req, resp)
	require.NoError(t, err)

	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?queryArg=EXCEEDS_MAX_LENGTH",
	}
	err = expect(req, resp)
	require.IsType(t, &openapi3filter.RequestError{}, err)

	// Test query parameter openapi3filter
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?queryArg=a",
	}
	err = expect(req, resp)
	require.NoError(t, err)

	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?queryArg=EXCEEDS_MAX_LENGTH",
	}
	err = expect(req, resp)
	require.IsType(t, &openapi3filter.RequestError{}, err)

	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix",
	}
	resp = ExampleResponse{
		Status: 200,
	}
	err = expect(req, resp)
	// require.IsType(t, &openapi3filter.ResponseError{}, err)
	require.NoError(t, err)

	// Check that content validation works. This should pass, as ID is short
	// enough.
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?contentArg={\"name\":\"bob\", \"id\":\"a\"}",
	}
	err = expect(req, resp)
	require.NoError(t, err)

	// Now it should fail due the ID being too long
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?contentArg={\"name\":\"bob\", \"id\":\"EXCEEDS_MAX_LENGTH\"}",
	}
	err = expect(req, resp)
	require.IsType(t, &openapi3filter.RequestError{}, err)

	// Now, repeat the above two test cases using a custom parameter decoder.
	customDecoder := func(param *openapi3.Parameter, values []string) (interface{}, *openapi3.Schema, error) {
		var value interface{}
		err := json.Unmarshal([]byte(values[0]), &value)
		schema := param.Content.Get("application/something_funny").Schema.Value
		return value, schema, err
	}

	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?contentArg2={\"name\":\"bob\", \"id\":\"a\"}",
	}
	err = expectWithDecoder(req, resp, customDecoder)
	require.NoError(t, err)

	// Now it should fail due the ID being too long
	req = ExampleRequest{
		Method: "POST",
		URL:    "http://example.com/api/prefix/v/suffix?contentArg2={\"name\":\"bob\", \"id\":\"EXCEEDS_MAX_LENGTH\"}",
	}
	err = expectWithDecoder(req, resp, customDecoder)
	require.IsType(t, &openapi3filter.RequestError{}, err)
}

func marshalReader(value interface{}) io.ReadCloser {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return ioutil.NopCloser(bytes.NewReader(data))
}

func TestValidateRequestBody(t *testing.T) {
	requiredReqBody := openapi3.NewRequestBody().
		WithContent(openapi3.NewContentWithJSONSchema(openapi3.NewStringSchema())).
		WithRequired(true)

	plainTextContent := openapi3.NewContent()
	plainTextContent["text/plain"] = openapi3.NewMediaType().WithSchema(openapi3.NewStringSchema())

	testCases := []struct {
		name    string
		body    *openapi3.RequestBody
		mime    string
		data    io.Reader
		wantErr error
	}{
		{
			name: "non required empty",
			body: openapi3.NewRequestBody().
				WithContent(openapi3.NewContentWithJSONSchema(openapi3.NewStringSchema())),
		},
		{
			name: "non required not empty",
			body: openapi3.NewRequestBody().
				WithContent(openapi3.NewContentWithJSONSchema(openapi3.NewStringSchema())),
			mime: "application/json",
			data: toJSON("foo"),
		},
		{
			name:    "required empty",
			body:    requiredReqBody,
			wantErr: &openapi3filter.RequestError{RequestBody: requiredReqBody, Err: openapi3filter.ErrInvalidRequired},
		},
		{
			name: "required not empty",
			body: requiredReqBody,
			mime: "application/json",
			data: toJSON("foo"),
		},
		{
			name: "not JSON data",
			body: openapi3.NewRequestBody().WithContent(plainTextContent).WithRequired(true),
			mime: "text/plain",
			data: strings.NewReader("foo"),
		},
		{
			name: "not declared content",
			body: openapi3.NewRequestBody().WithRequired(true),
			mime: "application/json",
			data: toJSON("foo"),
		},
		{
			name: "not declared schema",
			body: openapi3.NewRequestBody().WithJSONSchemaRef(nil),
			mime: "application/json",
			data: toJSON("foo"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", tc.data)
			if tc.mime != "" {
				req.Header.Set(http.CanonicalHeaderKey("Content-Type"), tc.mime)
			}
			inp := &openapi3filter.RequestValidationInput{Request: req}
			err := openapi3filter.ValidateRequestBody(context.Background(), inp, tc.body)

			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}

			require.True(t, matchReqBodyError(tc.wantErr, err), "got error:\n%s\nwant error\n%s", err, tc.wantErr)
		})
	}
}

func matchReqBodyError(want, got error) bool {
	if want == got {
		return true
	}
	wErr, ok := want.(*openapi3filter.RequestError)
	if !ok {
		return false
	}
	gErr, ok := got.(*openapi3filter.RequestError)
	if !ok {
		return false
	}
	if !reflect.DeepEqual(wErr.RequestBody, gErr.RequestBody) {
		return false
	}
	if wErr.Err != nil {
		return matchReqBodyError(wErr.Err, gErr.Err)
	}
	return false
}

func toJSON(v interface{}) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}
