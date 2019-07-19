package openapi3filter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// ErrInvalidRequired is an error that happens when a required value of a parameter or request's body is not defined.
var ErrInvalidRequired = fmt.Errorf("must have a value")

func ValidateRequest(c context.Context, input *RequestValidationInput) error {
	options := input.Options
	if options == nil {
		options = DefaultOptions
	}
	route := input.Route
	if route == nil {
		return errors.New("invalid route")
	}
	operation := route.Operation
	if operation == nil {
		return errRouteMissingOperation
	}
	operationParameters := operation.Parameters
	pathItemParameters := route.PathItem.Parameters

	// For each parameter of the PathItem
	for _, parameterRef := range pathItemParameters {
		parameter := parameterRef.Value
		if operationParameters != nil {
			if override := operationParameters.GetByInAndName(parameter.In, parameter.Name); override != nil {
				continue
			}
			if err := ValidateParameter(c, input, parameter); err != nil {
				return err
			}
		}
	}

	// For each parameter of the Operation
	for _, parameter := range operationParameters {
		if err := ValidateParameter(c, input, parameter.Value); err != nil {
			return err
		}
	}

	// RequestBody
	requestBody := operation.RequestBody
	if requestBody != nil && !options.ExcludeRequestBody {
		if err := ValidateRequestBody(c, input, requestBody.Value); err != nil {
			return err
		}
	}

	// Security
	security := operation.Security
	if security != nil {
		if err := ValidateSecurityRequirements(c, input, *security); err != nil {
			return err
		}
	}
	return nil
}

// ValidateParameter validates a parameter's value by JSON schema.
// The function returns RequestError with a ParseError cause when unable to parse a value.
// The function returns RequestError with ErrInvalidRequired cause when a value of a required parameter is not defined.
// The function returns RequestError with a openapi3.SchemaError cause when a value is invalid by JSON schema.
func ValidateParameter(c context.Context, input *RequestValidationInput, parameter *openapi3.Parameter) error {
	if parameter.Schema == nil && parameter.Content == nil {
		// We have no schema for the parameter. Assume that everything passes
		// a schema-less check, but this could also be an error. The Swagger
		// validation allows this to happen.
		return nil
	}

	var value interface{}
	var err error
	var schema *openapi3.Schema

	// Validation will ensure that we either have content or schema.
	if parameter.Content != nil {
		value, schema, err = decodeContentParameter(parameter, input)
		if err != nil {
			return &RequestError{Input: input, Parameter: parameter, Err: err}
		}
	} else {
		value, err = decodeStyledParameter(parameter, input)
		if err != nil {
			return &RequestError{Input: input, Parameter: parameter, Err: err}
		}
		schema = parameter.Schema.Value
	}
	// Validate a parameter's value.
	if value == nil {
		if parameter.Required {
			return &RequestError{Input: input, Parameter: parameter, Reason: "must have a value", Err: ErrInvalidRequired}
		}
		return nil
	}
	if schema == nil {
		// A parameter's schema is not defined so skip validation of a parameter's value.
		return nil
	}
	if err = schema.VisitJSON(value); err != nil {
		return &RequestError{Input: input, Parameter: parameter, Err: err}
	}
	return nil
}

// ValidateRequestBody validates data of a request's body.
//
// The function returns RequestError with ErrInvalidRequired cause when a value is required but not defined.
// The function returns RequestError with a openapi3.SchemaError cause when a value is invalid by JSON schema.
func ValidateRequestBody(c context.Context, input *RequestValidationInput, requestBody *openapi3.RequestBody) error {
	var (
		req  = input.Request
		data []byte
	)

	if req.Body != http.NoBody {
		defer req.Body.Close()
		var err error
		if data, err = ioutil.ReadAll(req.Body); err != nil {
			return &RequestError{
				Input:       input,
				RequestBody: requestBody,
				Reason:      "reading failed",
				Err:         err,
			}
		}
		// Put the data back into the input
		req.Body = ioutil.NopCloser(bytes.NewReader(data))
	}

	if len(data) == 0 {
		if requestBody.Required {
			return &RequestError{Input: input, RequestBody: requestBody, Err: ErrInvalidRequired}
		}
		return nil
	}

	content := requestBody.Content
	if len(content) == 0 {
		// A request's body does not have declared content, so skip validation.
		return nil
	}

	inputMIME := req.Header.Get("Content-Type")
	contentType := requestBody.Content.Get(inputMIME)
	if contentType == nil {
		return &RequestError{
			Input:       input,
			RequestBody: requestBody,
			Reason:      fmt.Sprintf("header 'Content-Type' has unexpected value: %q", inputMIME),
		}
	}

	if contentType.Schema == nil {
		// A JSON schema that describes the received data is not declared, so skip validation.
		return nil
	}

	encFn := func(name string) *openapi3.Encoding { return contentType.Encoding[name] }
	value, err := decodeBody(bytes.NewReader(data), req.Header, contentType.Schema, encFn)
	if err != nil {
		return &RequestError{
			Input:       input,
			RequestBody: requestBody,
			Reason:      "failed to decode request body",
			Err:         err,
		}
	}

	// Validate JSON with the schema
	if err := contentType.Schema.Value.VisitJSON(value); err != nil {
		return &RequestError{
			Input:       input,
			RequestBody: requestBody,
			Reason:      "doesn't match the schema",
			Err:         err,
		}
	}
	return nil
}

// ValidateSecurityRequirements validates a multiple OpenAPI 3 security requirements.
// Returns nil if one of them inputed.
// Otherwise returns an error describing the security failures.
func ValidateSecurityRequirements(c context.Context, input *RequestValidationInput, srs openapi3.SecurityRequirements) error {
	// Alternative requirements
	if len(srs) == 0 {
		return nil
	}

	doneChan := make(chan bool, len(srs))
	errs := make([]error, len(srs))

	// For each alternative
	for i, securityRequirement := range srs {
		// Capture index from iteration variable
		currentIndex := i
		currentSecurityRequirement := securityRequirement
		go func() {
			defer func() {
				v := recover()
				if v != nil {
					if err, ok := v.(error); ok {
						errs[currentIndex] = err
					} else {
						errs[currentIndex] = errors.New("Panicked")
					}
					doneChan <- false
				}
			}()
			if err := validateSecurityRequirement(c, input, currentSecurityRequirement); err == nil {
				doneChan <- true
			} else {
				errs[currentIndex] = err
				doneChan <- false
			}
		}()
	}

	// Wait for all
	for i := 0; i < len(srs); i++ {
		ok := <-doneChan
		if ok {
			close(doneChan)
			return nil
		}
	}
	return &SecurityRequirementsError{
		SecurityRequirements: srs,
		Errors:               errs,
	}
}

// validateSecurityRequirement validates a single OpenAPI 3 security requirement
func validateSecurityRequirement(c context.Context, input *RequestValidationInput, securityRequirement openapi3.SecurityRequirement) error {
	swagger := input.Route.Swagger
	if swagger == nil {
		return errRouteMissingSwagger
	}
	securitySchemes := swagger.Components.SecuritySchemes

	// Ensure deterministic order
	names := make([]string, 0, len(securityRequirement))
	for name := range securityRequirement {
		names = append(names, name)
	}
	sort.Strings(names)

	// Get authentication function
	options := input.Options
	if options == nil {
		options = DefaultOptions
	}
	f := options.AuthenticationFunc
	if f == nil {
		return ErrAuthenticationServiceMissing
	}

	if len(names) > 0 {
		name := names[0]
		var securityScheme *openapi3.SecurityScheme
		if securitySchemes != nil {
			if ref := securitySchemes[name]; ref != nil {
				securityScheme = ref.Value
			}
		}
		if securityScheme == nil {
			return &RequestError{
				Input: input,
				Err:   fmt.Errorf("Security scheme '%s' is not declared", name),
			}
		}
		scopes := securityRequirement[name]
		return f(c, &AuthenticationInput{
			RequestValidationInput: input,
			SecuritySchemeName:     name,
			SecurityScheme:         securityScheme,
			Scopes:                 scopes,
		})
	}
	return nil
}
