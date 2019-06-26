package openapi3

import (
	"context"
	"strconv"

	"github.com/getkin/kin-openapi/jsoninfo"
)

// Operation represents "operation" specified by" OpenAPI/Swagger 3.0 standard.
type Operation struct {
	ExtensionProps

	// Optional tags for documentation.
	Tags []string `json:"tags,omitempty"`

	// Optional short summary.
	Summary string `json:"summary,omitempty"`

	// Optional description. Should use CommonMark syntax.
	Description string `json:"description,omitempty"`

	// Optional operation ID.
	OperationID string `json:"operationId,omitempty"`

	// Optional parameters.
	Parameters Parameters `json:"parameters,omitempty"`

	// Optional body parameter.
	RequestBody *RequestBodyRef `json:"requestBody,omitempty"`

	// Optional responses.
	Responses Responses `json:"responses,omitempty"`

	// Optional callbacks
	Callbacks map[string]*CallbackRef `json:"callbacks,omitempty"`

	Deprecated bool `json:"deprecated,omitempty"`

	// Optional security requirements that overrides top-level security.
	Security *SecurityRequirements `json:"security,omitempty"`

	// Optional servers that overrides top-level servers.
	Servers *Servers `json:"servers,omitempty"`
}

func NewOperation() *Operation {
	return &Operation{}
}

func (operation *Operation) MarshalJSON() ([]byte, error) {
	return jsoninfo.MarshalStrictStruct(operation)
}

func (operation *Operation) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, operation)
}

func (operation *Operation) AddParameter(p *Parameter) {
	operation.Parameters = append(operation.Parameters, &ParameterRef{
		Value: p,
	})
}

func (operation *Operation) AddResponse(status int, response *Response) {
	responses := operation.Responses
	if responses == nil {
		responses = NewResponses()
		operation.Responses = responses
	}
	if status == 0 {
		responses["default"] = &ResponseRef{
			Value: response,
		}
	} else {
		responses[strconv.FormatInt(int64(status), 10)] = &ResponseRef{
			Value: response,
		}
	}
}

func (operation *Operation) Validate(c context.Context) error {
	if v := operation.Parameters; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	if v := operation.RequestBody; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	if v := operation.Responses; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	return nil
}
