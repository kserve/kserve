package openapi3

import (
	"context"
)

type Header struct {
	ExtensionProps

	// Optional description. Should use CommonMark syntax.
	Description string `json:"description,omitempty"`

	// Optional schema
	Schema *SchemaRef `json:"schema,omitempty"`
}

func (value *Header) Validate(c context.Context) error {
	if v := value.Schema; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	return nil
}
