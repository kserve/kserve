package openapi3

import (
	"context"
	"fmt"
	"regexp"

	"github.com/getkin/kin-openapi/jsoninfo"
)

// Components is specified by OpenAPI/Swagger standard version 3.0.
type Components struct {
	ExtensionProps
	Schemas         map[string]*SchemaRef         `json:"schemas,omitempty"`
	Parameters      map[string]*ParameterRef      `json:"parameters,omitempty"`
	Headers         map[string]*HeaderRef         `json:"headers,omitempty"`
	RequestBodies   map[string]*RequestBodyRef    `json:"requestBodies,omitempty"`
	Responses       map[string]*ResponseRef       `json:"responses,omitempty"`
	SecuritySchemes map[string]*SecuritySchemeRef `json:"securitySchemes,omitempty"`
	Examples        map[string]*ExampleRef        `json:"examples,omitempty"`
	Tags            Tags                          `json:"tags,omitempty"`
	Links           map[string]*LinkRef           `json:"links,omitempty"`
	Callbacks       map[string]*CallbackRef       `json:"callbacks,omitempty"`
}

func NewComponents() Components {
	return Components{}
}

func (components *Components) MarshalJSON() ([]byte, error) {
	return jsoninfo.MarshalStrictStruct(components)
}

func (components *Components) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, components)
}

func (components *Components) Validate(c context.Context) (err error) {
	for k, v := range components.Schemas {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	for k, v := range components.Parameters {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	for k, v := range components.RequestBodies {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	for k, v := range components.Responses {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	for k, v := range components.Headers {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	for k, v := range components.SecuritySchemes {
		if err = ValidateIdentifier(k); err != nil {
			return
		}
		if err = v.Validate(c); err != nil {
			return
		}
	}

	return
}

const identifierPattern = `^[a-zA-Z0-9.\-_]+$`

var identifierRegExp = regexp.MustCompile(identifierPattern)

func ValidateIdentifier(value string) error {
	if identifierRegExp.MatchString(value) {
		return nil
	}
	return fmt.Errorf("Identifier '%s' is not supported by OpenAPI version 3 standard (regexp: '%s')", value, identifierPattern)
}
