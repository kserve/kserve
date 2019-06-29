package openapi3

import (
	"context"

	"github.com/getkin/kin-openapi/jsoninfo"
)

type Swagger struct {
	ExtensionProps
	OpenAPI      string               `json:"openapi"` // Required
	Info         Info                 `json:"info"`    // Required
	Servers      Servers              `json:"servers,omitempty"`
	Paths        Paths                `json:"paths,omitempty"`
	Components   Components           `json:"components,omitempty"`
	Security     SecurityRequirements `json:"security,omitempty"`
	ExternalDocs *ExternalDocs        `json:"externalDocs,omitempty"`
}

func (swagger *Swagger) MarshalJSON() ([]byte, error) {
	return jsoninfo.MarshalStrictStruct(swagger)
}

func (swagger *Swagger) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, swagger)
}

func (swagger *Swagger) AddOperation(path string, method string, operation *Operation) {
	paths := swagger.Paths
	if paths == nil {
		paths = make(Paths)
		swagger.Paths = paths
	}
	pathItem := paths[path]
	if pathItem == nil {
		pathItem = &PathItem{}
		paths[path] = pathItem
	}
	pathItem.SetOperation(method, operation)
}

func (swagger *Swagger) AddServer(server *Server) {
	swagger.Servers = append(swagger.Servers, server)
}

func (swagger *Swagger) Validate(c context.Context) error {
	if err := swagger.Components.Validate(c); err != nil {
		return err
	}
	if v := swagger.Security; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	if paths := swagger.Paths; paths != nil {
		if err := paths.Validate(c); err != nil {
			return err
		}
	}
	if v := swagger.Servers; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	if v := swagger.Paths; v != nil {
		if err := v.Validate(c); err != nil {
			return err
		}
	}
	return nil
}
