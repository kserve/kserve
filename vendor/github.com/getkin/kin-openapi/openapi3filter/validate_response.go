// Package openapi3filter validates that requests and inputs request an OpenAPI 3 specification file.
package openapi3filter

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

func ValidateResponse(c context.Context, input *ResponseValidationInput) error {
	req := input.RequestValidationInput.Request
	switch req.Method {
	case "HEAD":
		return nil
	}
	status := input.Status
	if status < 100 {
		return &ResponseError{
			Input:  input,
			Reason: "illegal status code",
			Err:    fmt.Errorf("Status %d", status),
		}
	}

	// These status codes will never be validated.
	// TODO: The list is probably missing some.
	switch status {
	case http.StatusNotModified,
		http.StatusPermanentRedirect,
		http.StatusTemporaryRedirect,
		http.StatusMovedPermanently:
		return nil
	}
	route := input.RequestValidationInput.Route
	options := input.Options
	if options == nil {
		options = DefaultOptions
	}

	// Find input for the current status
	responses := route.Operation.Responses
	if len(responses) == 0 {
		return nil
	}
	responseRef := responses.Get(status) // Response
	if responseRef == nil {
		responseRef = responses.Default() // Default input
	}
	if responseRef == nil {
		// By default, status that is not documented is allowed.
		if !options.IncludeResponseStatus {
			return nil
		}

		return &ResponseError{Input: input, Reason: "status is not supported"}
	}
	response := responseRef.Value
	if response == nil {
		return &ResponseError{Input: input, Reason: "response has not been resolved"}
	}

	if options.ExcludeResponseBody {
		// A user turned off validation of a response's body.
		return nil
	}

	content := response.Content
	if len(content) == 0 || options.ExcludeResponseBody {
		// An operation does not contains a validation schema for responses with this status code.
		return nil
	}

	inputMIME := input.Header.Get("Content-Type")
	contentType := content.Get(inputMIME)
	if contentType == nil {
		return &ResponseError{
			Input:  input,
			Reason: fmt.Sprintf("input header 'Content-Type' has unexpected value: %q", inputMIME),
		}
	}

	if contentType.Schema == nil {
		// An operation does not contains a validation schema for responses with this status code.
		return nil
	}

	// Read response's body.
	body := input.Body

	// Response would contain partial or empty input body
	// after we begin reading.
	// Ensure that this doesn't happen.
	input.Body = nil

	// Ensure we close the reader
	defer body.Close()

	// Read all
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "failed to read response body",
			Err:    err,
		}
	}

	// Put the data back into the response.
	input.SetBodyBytes(data)

	encFn := func(name string) *openapi3.Encoding { return contentType.Encoding[name] }
	value, err := decodeBody(bytes.NewBuffer(data), input.Header, contentType.Schema, encFn)
	if err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "failed to decode response body",
			Err:    err,
		}
	}

	// Validate data with the schema.
	if err := contentType.Schema.Value.VisitJSON(value); err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "response body doesn't match the schema",
			Err:    err,
		}
	}
	return nil
}
