// Package openapi2conv converts an OpenAPI v2 specification to v3.
package openapi2conv

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi3"
)

func ToV3Swagger(swagger *openapi2.Swagger) (*openapi3.Swagger, error) {
	result := &openapi3.Swagger{
		OpenAPI: "3.0",
		Info:    swagger.Info,
		Components: openapi3.Components{
			Tags: swagger.Tags,
		},
	}
	host := swagger.Host
	if len(host) > 0 {
		schemes := swagger.Schemes
		if len(schemes) == 0 {
			schemes = []string{
				"https://",
			}
		}
		basePath := swagger.BasePath
		for _, scheme := range schemes {
			u := url.URL{
				Scheme: scheme,
				Host:   host,
				Path:   basePath,
			}
			result.AddServer(&openapi3.Server{
				URL: u.String(),
			})
		}
	}
	if paths := swagger.Paths; paths != nil {
		resultPaths := make(map[string]*openapi3.PathItem, len(paths))
		for path, pathItem := range paths {
			r, err := ToV3PathItem(swagger, pathItem)
			if err != nil {
				return nil, err
			}
			resultPaths[path] = r
		}
		result.Paths = resultPaths
	}
	if parameters := swagger.Parameters; parameters != nil {
		result.Components.Parameters = make(map[string]*openapi3.ParameterRef)
		result.Components.RequestBodies = make(map[string]*openapi3.RequestBodyRef)
		for k, parameter := range parameters {
			resultParameter, resultRequestBody, err := ToV3Parameter(parameter)
			if err != nil {
				return nil, err
			}
			if resultParameter != nil {
				result.Components.Parameters[k] = resultParameter
			}
			if resultRequestBody != nil {
				result.Components.RequestBodies[k] = resultRequestBody
			}
		}
	}
	if responses := swagger.Responses; responses != nil {
		result.Components.Responses = make(map[string]*openapi3.ResponseRef, len(responses))
		for k, response := range responses {
			r, err := ToV3Response(response)
			if err != nil {
				return nil, err
			}
			result.Components.Responses[k] = r
		}
	}
	result.Components.Schemas = swagger.Definitions
	if m := swagger.SecurityDefinitions; m != nil {
		resultSecuritySchemes := make(map[string]*openapi3.SecuritySchemeRef)
		for k, v := range m {
			r, err := ToV3SecurityScheme(v)
			if err != nil {
				return nil, err
			}
			resultSecuritySchemes[k] = r
		}
		result.Components.SecuritySchemes = resultSecuritySchemes
	}
	result.Security = ToV3SecurityRequirements(swagger.Security)
	return result, nil
}

func ToV3PathItem(swagger *openapi2.Swagger, pathItem *openapi2.PathItem) (*openapi3.PathItem, error) {
	result := &openapi3.PathItem{}
	for method, operation := range pathItem.Operations() {
		resultOperation, err := ToV3Operation(swagger, pathItem, operation)
		if err != nil {
			return nil, err
		}
		result.SetOperation(method, resultOperation)
	}
	for _, parameter := range pathItem.Parameters {
		v3Parameter, v3RequestBody, err := ToV3Parameter(parameter)
		if err != nil {
			return nil, err
		}
		if v3RequestBody != nil {
			return nil, errors.New("PathItem shouldn't have a body parameter")
		}
		result.Parameters = append(result.Parameters, v3Parameter)
	}
	return result, nil
}

func ToV3Operation(swagger *openapi2.Swagger, pathItem *openapi2.PathItem, operation *openapi2.Operation) (*openapi3.Operation, error) {
	if operation == nil {
		return nil, nil
	}
	result := &openapi3.Operation{
		OperationID: operation.OperationID,
		Summary:     operation.Summary,
		Description: operation.Description,
		Tags:        operation.Tags,
	}
	if v := operation.Security; v != nil {
		resultSecurity := ToV3SecurityRequirements(*v)
		result.Security = &resultSecurity
	}
	for _, parameter := range operation.Parameters {
		v3Parameter, v3RequestBody, err := ToV3Parameter(parameter)
		if err != nil {
			return nil, err
		}
		if v3RequestBody != nil {
			result.RequestBody = v3RequestBody
		} else if v3Parameter != nil {
			result.Parameters = append(result.Parameters, v3Parameter)
		}
	}
	if responses := operation.Responses; responses != nil {
		resultResponses := make(openapi3.Responses, len(responses))
		for k, response := range responses {
			result, err := ToV3Response(response)
			if err != nil {
				return nil, err
			}
			resultResponses[k] = result
		}
		result.Responses = resultResponses
	}
	return result, nil
}

func ToV3Parameter(parameter *openapi2.Parameter) (*openapi3.ParameterRef, *openapi3.RequestBodyRef, error) {
	if parameter == nil {
		return nil, nil, nil
	}
	in := parameter.In
	if in == "body" {
		result := &openapi3.RequestBody{
			Description: parameter.Description,
			Required:    parameter.Required,
		}
		if schemaRef := parameter.Schema; schemaRef != nil {
			// Assume it's JSON
			result.WithJSONSchemaRef(schemaRef)
		}
		return nil, &openapi3.RequestBodyRef{
			Value: result,
		}, nil
	}
	result := &openapi3.Parameter{
		In:          in,
		Name:        parameter.Name,
		Description: parameter.Description,
		Required:    parameter.Required,
		Schema:      parameter.Schema,
	}
	if parameter.Type != "" {
		result.Schema = &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:         parameter.Type,
				Format:       parameter.Format,
				Enum:         parameter.Enum,
				Min:          parameter.Minimum,
				Max:          parameter.Maximum,
				ExclusiveMin: parameter.ExclusiveMin,
				ExclusiveMax: parameter.ExclusiveMax,
				MinLength:    parameter.MinLength,
				MaxLength:    parameter.MaxLength,
			},
		}
	}
	return &openapi3.ParameterRef{
		Value: result,
	}, nil, nil
}

func ToV3Response(response *openapi2.Response) (*openapi3.ResponseRef, error) {
	if ref := response.Ref; len(ref) > 0 {
		return &openapi3.ResponseRef{
			Ref: ref,
		}, nil
	}
	result := &openapi3.Response{
		Description: response.Description,
	}
	if schemaRef := response.Schema; schemaRef != nil {
		result.WithJSONSchemaRef(schemaRef)
	}
	return &openapi3.ResponseRef{
		Value: result,
	}, nil
}

func ToV3SecurityRequirements(requirements openapi2.SecurityRequirements) openapi3.SecurityRequirements {
	if requirements == nil {
		return nil
	}
	result := make(openapi3.SecurityRequirements, len(requirements))
	for i, item := range requirements {
		result[i] = item
	}
	return result
}

func ToV3SecurityScheme(securityScheme *openapi2.SecurityScheme) (*openapi3.SecuritySchemeRef, error) {
	if securityScheme == nil {
		return nil, nil
	}
	result := &openapi3.SecurityScheme{
		Description: securityScheme.Description,
	}
	switch securityScheme.Type {
	case "basic":
		result.Type = "http"
		result.Scheme = "basic"
	case "apiKey":
		result.Type = "apiKey"
		result.In = securityScheme.In
		result.Name = securityScheme.Name
	case "oauth2":
		result.Type = "oauth2"
		flows := &openapi3.OAuthFlows{}
		result.Flows = flows
		scopesMap := make(map[string]string)
		for _, scope := range securityScheme.Scopes {
			scopesMap[scope] = ""
		}
		flow := &openapi3.OAuthFlow{
			AuthorizationURL: securityScheme.AuthorizationURL,
			TokenURL:         securityScheme.TokenURL,
			Scopes:           scopesMap,
		}
		switch securityScheme.Flow {
		case "implicit":
			flows.Implicit = flow
		case "accesscode":
			flows.AuthorizationCode = flow
		case "password":
			flows.Password = flow
		default:
			return nil, fmt.Errorf("Unsupported flow '%s'", securityScheme.Flow)
		}
	}
	return &openapi3.SecuritySchemeRef{
		Ref:   securityScheme.Ref,
		Value: result,
	}, nil
}

func FromV3Swagger(swagger *openapi3.Swagger) (*openapi2.Swagger, error) {
	result := &openapi2.Swagger{
		Info: swagger.Info,
		Tags: swagger.Components.Tags,
	}
	isHTTPS := false
	isHTTP := false
	servers := swagger.Servers
	for i, server := range servers {
		parsedURL, err := url.Parse(server.URL)
		if err == nil {
			// See which schemes seem to be supported
			if parsedURL.Scheme == "https" {
				isHTTPS = true
			} else if parsedURL.Scheme == "http" {
				isHTTP = true
			}
			// The first server is assumed to provide the base path
			if i == 0 {
				result.Host = parsedURL.Host
				result.BasePath = parsedURL.Path
			}
		}
	}
	if isHTTPS {
		result.Schemes = append(result.Schemes, "https")
	}
	if isHTTP {
		result.Schemes = append(result.Schemes, "http")
	}
	for path, pathItem := range swagger.Paths {
		if pathItem == nil {
			continue
		}
		for method, operation := range pathItem.Operations() {
			if operation == nil {
				continue
			}
			resultOperation, err := FromV3Operation(swagger, operation)
			if err != nil {
				return nil, err
			}
			result.AddOperation(path, method, resultOperation)
		}
	}
	if m := swagger.Components.SecuritySchemes; m != nil {
		resultSecuritySchemes := make(map[string]*openapi2.SecurityScheme)
		for id, securityScheme := range m {
			v, err := FromV3SecurityScheme(swagger, securityScheme)
			if err != nil {
				return nil, err
			}
			resultSecuritySchemes[id] = v
		}
		result.SecurityDefinitions = resultSecuritySchemes
	}
	result.Security = FromV3SecurityRequirements(swagger.Security)
	return result, nil
}

func FromV3SecurityRequirements(requirements openapi3.SecurityRequirements) openapi2.SecurityRequirements {
	if requirements == nil {
		return nil
	}
	result := make([]map[string][]string, len(requirements))
	for i, item := range requirements {
		result[i] = item
	}
	return result
}

func FromV3PathItem(swagger *openapi3.Swagger, pathItem *openapi3.PathItem) (*openapi2.PathItem, error) {
	result := &openapi2.PathItem{}
	for method, operation := range pathItem.Operations() {
		r, err := FromV3Operation(swagger, operation)
		if err != nil {
			return nil, err
		}
		result.SetOperation(method, r)
	}
	for _, parameter := range pathItem.Parameters {
		p, err := FromV3Parameter(parameter)
		if err != nil {
			return nil, err
		}
		result.Parameters = append(result.Parameters, p)
	}
	return result, nil
}

func findNameForRequestBody(operation *openapi3.Operation) string {
nameSearch:
	for _, name := range attemptedBodyParameterNames {
		for _, parameterRef := range operation.Parameters {
			parameter := parameterRef.Value
			if parameter != nil && parameter.Name == name {
				continue nameSearch
			}
		}
		return name
	}
	return ""
}

func FromV3Operation(swagger *openapi3.Swagger, operation *openapi3.Operation) (*openapi2.Operation, error) {
	if operation == nil {
		return nil, nil
	}
	result := &openapi2.Operation{
		OperationID: operation.OperationID,
		Summary:     operation.Summary,
		Description: operation.Description,
		Tags:        operation.Tags,
	}
	if v := operation.Security; v != nil {
		resultSecurity := FromV3SecurityRequirements(*v)
		result.Security = &resultSecurity
	}
	for _, parameter := range operation.Parameters {
		r, err := FromV3Parameter(parameter)
		if err != nil {
			return nil, err
		}
		result.Parameters = append(result.Parameters, r)
	}
	if v := operation.RequestBody; v != nil {
		r, err := FromV3RequestBody(swagger, operation, v)
		if err != nil {
			return nil, err
		}
		result.Parameters = append(result.Parameters, r)
	}
	if responses := operation.Responses; responses != nil {
		resultResponses := make(map[string]*openapi2.Response, len(responses))
		result.Responses = resultResponses
		for k, response := range responses {
			r, err := FromV3Response(response)
			if err != nil {
				return nil, err
			}
			resultResponses[k] = r
		}
	}
	return result, nil
}

func FromV3RequestBody(swagger *openapi3.Swagger, operation *openapi3.Operation, requestBodyRef *openapi3.RequestBodyRef) (*openapi2.Parameter, error) {
	if ref := requestBodyRef.Ref; len(ref) > 0 {
		return &openapi2.Parameter{
			Ref: ref,
		}, nil
	}
	requestBody := requestBodyRef.Value

	// Find parameter name that we can use for the body
	name := findNameForRequestBody(operation)

	// If found an available name
	if name == "" {
		return nil, errors.New("Could not find a name for request body")
	}
	result := &openapi2.Parameter{
		In:          "body",
		Name:        name,
		Description: requestBody.Description,
		Required:    requestBody.Required,
	}

	// Add JSON schema
	mediaType := requestBody.GetMediaType("application/json")
	if mediaType != nil {
		result.Schema = mediaType.Schema
	}
	return result, nil
}

func FromV3Parameter(ref *openapi3.ParameterRef) (*openapi2.Parameter, error) {
	if v := ref.Ref; len(v) > 0 {
		return &openapi2.Parameter{
			Ref: v,
		}, nil
	}
	parameter := ref.Value
	if parameter == nil {
		return nil, nil
	}
	result := &openapi2.Parameter{
		Description: parameter.Description,
		In:          parameter.In,
		Name:        parameter.Name,
		Required:    parameter.Required,
		Schema:      parameter.Schema,
	}
	if schemaRef := parameter.Schema; schemaRef != nil {
		schema := schemaRef.Value
		result.Type = schema.Type
		result.Format = schema.Format
		result.Enum = schema.Enum
		result.Minimum = schema.Min
		result.Maximum = schema.Min
		result.ExclusiveMin = schema.ExclusiveMin
		result.ExclusiveMax = schema.ExclusiveMax
		result.MinLength = schema.MinLength
		result.MaxLength = schema.MaxLength
		result.Pattern = schema.Pattern
	}
	return result, nil
}

func FromV3Response(ref *openapi3.ResponseRef) (*openapi2.Response, error) {
	response := ref.Value
	if response == nil {
		return nil, nil
	}
	result := &openapi2.Response{
		Description: response.Description,
	}
	if content := response.Content; content != nil {
		if ct := content["application/json"]; ct != nil {
			result.Schema = ct.Schema
		}
	}
	return result, nil
}

func FromV3SecurityScheme(swagger *openapi3.Swagger, ref *openapi3.SecuritySchemeRef) (*openapi2.SecurityScheme, error) {
	securityScheme := ref.Value
	if securityScheme == nil {
		return nil, nil
	}
	result := &openapi2.SecurityScheme{
		Ref:         ref.Ref,
		Description: securityScheme.Description,
	}
	switch securityScheme.Type {
	case "http":
		switch securityScheme.Scheme {
		case "basic":
			result.Type = "basic"
		default:
			result.Type = "apiKey"
			result.In = "header"
			result.Name = "Authorization"
		}
	case "apiKey":
		result.Type = "apiKey"
		result.In = securityScheme.In
		result.Name = securityScheme.Name
	case "oauth2":
		result.Type = "oauth2"
		flows := securityScheme.Flows
		if flows != nil {
			var flow *openapi3.OAuthFlow
			// TODO: Is this the right priority? What if multiple defined?
			if flow = flows.Implicit; flow != nil {
				result.Flow = "implicit"
			} else if flow = flows.AuthorizationCode; flow != nil {
				result.Flow = "accesscode"
			} else if flow = flows.Password; flow != nil {
				result.Flow = "password"
			} else {
				return nil, nil
			}
			for scope := range flow.Scopes {
				result.Scopes = append(result.Scopes, scope)
			}
		}
	default:
		return nil, fmt.Errorf("Unsupported security scheme type '%s'", securityScheme.Type)
	}
	return result, nil
}

var attemptedBodyParameterNames = []string{
	"body",
	"requestBody",
}
