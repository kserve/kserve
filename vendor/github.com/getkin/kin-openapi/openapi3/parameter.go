package openapi3

import (
	"context"
	"errors"
	"fmt"

	"github.com/getkin/kin-openapi/jsoninfo"
)

// Parameters is specified by OpenAPI/Swagger 3.0 standard.
type Parameters []*ParameterRef

func NewParameters() Parameters {
	return make(Parameters, 0, 4)
}

func (parameters Parameters) GetByInAndName(in string, name string) *Parameter {
	for _, item := range parameters {
		if v := item.Value; v != nil {
			if v.Name == name && v.In == in {
				return v
			}
		}
	}
	return nil
}

func (parameters Parameters) Validate(c context.Context) error {
	m := make(map[string]struct{})
	for _, item := range parameters {
		if err := item.Validate(c); err != nil {
			return err
		}
		if v := item.Value; v != nil {
			in := v.In
			name := v.Name
			key := in + ":" + name
			if _, exists := m[key]; exists {
				return fmt.Errorf("More than one '%s' parameter has name '%s'", in, name)
			}
			m[key] = struct{}{}
			if err := item.Validate(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// Parameter is specified by OpenAPI/Swagger 3.0 standard.
type Parameter struct {
	ExtensionProps
	Name            string                 `json:"name,omitempty"`
	In              string                 `json:"in,omitempty"`
	Description     string                 `json:"description,omitempty"`
	Style           string                 `json:"style,omitempty"`
	Explode         *bool                  `json:"explode,omitempty"`
	AllowEmptyValue bool                   `json:"allowEmptyValue,omitempty"`
	AllowReserved   bool                   `json:"allowReserved,omitempty"`
	Deprecated      bool                   `json:"deprecated,omitempty"`
	Required        bool                   `json:"required,omitempty"`
	Schema          *SchemaRef             `json:"schema,omitempty"`
	Example         interface{}            `json:"example,omitempty"`
	Examples        map[string]*ExampleRef `json:"examples,omitempty"`
	Content         Content                `json:"content,omitempty"`
}

const (
	ParameterInPath   = "path"
	ParameterInQuery  = "query"
	ParameterInHeader = "header"
	ParameterInCookie = "cookie"
)

func NewPathParameter(name string) *Parameter {
	return &Parameter{
		Name:     name,
		In:       ParameterInPath,
		Required: true,
	}
}

func NewQueryParameter(name string) *Parameter {
	return &Parameter{
		Name: name,
		In:   ParameterInQuery,
	}
}

func NewHeaderParameter(name string) *Parameter {
	return &Parameter{
		Name: name,
		In:   ParameterInHeader,
	}
}

func NewCookieParameter(name string) *Parameter {
	return &Parameter{
		Name: name,
		In:   ParameterInCookie,
	}
}

func (parameter *Parameter) WithDescription(value string) *Parameter {
	parameter.Description = value
	return parameter
}

func (parameter *Parameter) WithRequired(value bool) *Parameter {
	parameter.Required = value
	return parameter
}

func (parameter *Parameter) WithSchema(value *Schema) *Parameter {
	if value == nil {
		parameter.Schema = nil
	} else {
		parameter.Schema = &SchemaRef{
			Value: value,
		}
	}
	return parameter
}

func (parameter *Parameter) MarshalJSON() ([]byte, error) {
	return jsoninfo.MarshalStrictStruct(parameter)
}

func (parameter *Parameter) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, parameter)
}

// SerializationMethod returns a parameter's serialization method.
// When a parameter's serialization method is not defined the method returns
// the default serialization method corresponding to a parameter's location.
func (parameter *Parameter) SerializationMethod() (*SerializationMethod, error) {
	switch parameter.In {
	case ParameterInPath, ParameterInHeader:
		style := parameter.Style
		if style == "" {
			style = SerializationSimple
		}
		explode := false
		if parameter.Explode != nil {
			explode = *parameter.Explode
		}
		return &SerializationMethod{Style: style, Explode: explode}, nil
	case ParameterInQuery, ParameterInCookie:
		style := parameter.Style
		if style == "" {
			style = SerializationForm
		}
		explode := true
		if parameter.Explode != nil {
			explode = *parameter.Explode
		}
		return &SerializationMethod{Style: style, Explode: explode}, nil
	default:
		return nil, fmt.Errorf("unexpected parameter's 'in': %q", parameter.In)
	}
}

func (parameter *Parameter) Validate(c context.Context) error {
	if parameter.Name == "" {
		return errors.New("Parameter name can't be blank")
	}
	in := parameter.In
	switch in {
	case
		ParameterInPath,
		ParameterInQuery,
		ParameterInHeader,
		ParameterInCookie:
	default:
		return fmt.Errorf("Parameter can't have 'in' value '%s'", parameter.In)
	}

	// Validate a parameter's serialization method.
	sm, err := parameter.SerializationMethod()
	if err != nil {
		return err
	}
	var smSupported bool
	switch {
	case parameter.In == ParameterInPath && sm.Style == SerializationSimple && !sm.Explode,
		parameter.In == ParameterInPath && sm.Style == SerializationSimple && sm.Explode,
		parameter.In == ParameterInPath && sm.Style == SerializationLabel && !sm.Explode,
		parameter.In == ParameterInPath && sm.Style == SerializationLabel && sm.Explode,
		parameter.In == ParameterInPath && sm.Style == SerializationMatrix && !sm.Explode,
		parameter.In == ParameterInPath && sm.Style == SerializationMatrix && sm.Explode,

		parameter.In == ParameterInQuery && sm.Style == SerializationForm && sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationForm && !sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationSpaceDelimited && sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationSpaceDelimited && !sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationPipeDelimited && sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationPipeDelimited && !sm.Explode,
		parameter.In == ParameterInQuery && sm.Style == SerializationDeepObject && sm.Explode,

		parameter.In == ParameterInHeader && sm.Style == SerializationSimple && !sm.Explode,
		parameter.In == ParameterInHeader && sm.Style == SerializationSimple && sm.Explode,

		parameter.In == ParameterInCookie && sm.Style == SerializationForm && !sm.Explode,
		parameter.In == ParameterInCookie && sm.Style == SerializationForm && sm.Explode:
		smSupported = true
	}
	if !smSupported {
		return fmt.Errorf("Parameter '%v' schema is invalid: %v", parameter.Name,
			fmt.Errorf("Serialization method with style=%q and explode=%v is not supported by a %s parameter", sm.Style, sm.Explode, in))
	}

	if parameter.Schema != nil && parameter.Content != nil {
		return fmt.Errorf("Parameter '%v' schema is invalid: %v", parameter.Name,
			errors.New("Cannot contain both schema and content in a parameter"))
	}
	if schema := parameter.Schema; schema != nil {
		if err := schema.Validate(c); err != nil {
			return fmt.Errorf("Parameter '%v' schema is invalid: %v", parameter.Name, err)
		}
	}
	if content := parameter.Content; content != nil {
		if err := content.Validate(c); err != nil {
			return fmt.Errorf("Parameter content is invalid: %v", err)
		}
	}
	return nil
}
