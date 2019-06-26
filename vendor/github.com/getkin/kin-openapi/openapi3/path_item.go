package openapi3

import (
	"context"
	"fmt"

	"github.com/getkin/kin-openapi/jsoninfo"
)

type PathItem struct {
	ExtensionProps
	Summary     string     `json:"summary,omitempty"`
	Description string     `json:"description,omitempty"`
	Connect     *Operation `json:"connect,omitempty"`
	Delete      *Operation `json:"delete,omitempty"`
	Get         *Operation `json:"get,omitempty"`
	Head        *Operation `json:"head,omitempty"`
	Options     *Operation `json:"options,omitempty"`
	Patch       *Operation `json:"patch,omitempty"`
	Post        *Operation `json:"post,omitempty"`
	Put         *Operation `json:"put,omitempty"`
	Trace       *Operation `json:"trace,omitempty"`
	Servers     Servers    `json:"servers,omitempty"`
	Parameters  Parameters `json:"parameters,omitempty"`
}

func (pathItem *PathItem) MarshalJSON() ([]byte, error) {
	return jsoninfo.MarshalStrictStruct(pathItem)
}

func (pathItem *PathItem) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, pathItem)
}

func (pathItem *PathItem) Operations() map[string]*Operation {
	operations := make(map[string]*Operation, 4)
	if v := pathItem.Connect; v != nil {
		operations["CONNECT"] = v
	}
	if v := pathItem.Delete; v != nil {
		operations["DELETE"] = v
	}
	if v := pathItem.Get; v != nil {
		operations["GET"] = v
	}
	if v := pathItem.Head; v != nil {
		operations["HEAD"] = v
	}
	if v := pathItem.Options; v != nil {
		operations["OPTIONS"] = v
	}
	if v := pathItem.Patch; v != nil {
		operations["PATCH"] = v
	}
	if v := pathItem.Post; v != nil {
		operations["POST"] = v
	}
	if v := pathItem.Put; v != nil {
		operations["PUT"] = v
	}
	if v := pathItem.Trace; v != nil {
		operations["TRACE"] = v
	}
	return operations
}

func (pathItem *PathItem) GetOperation(method string) *Operation {
	switch method {
	case "CONNECT":
		return pathItem.Connect
	case "DELETE":
		return pathItem.Delete
	case "GET":
		return pathItem.Get
	case "HEAD":
		return pathItem.Head
	case "OPTIONS":
		return pathItem.Options
	case "PATCH":
		return pathItem.Patch
	case "POST":
		return pathItem.Post
	case "PUT":
		return pathItem.Put
	case "TRACE":
		return pathItem.Trace
	default:
		panic(fmt.Errorf("Unsupported HTTP method '%s'", method))
	}
}

func (pathItem *PathItem) SetOperation(method string, operation *Operation) {
	switch method {
	case "CONNECT":
		pathItem.Connect = operation
	case "DELETE":
		pathItem.Delete = operation
	case "GET":
		pathItem.Get = operation
	case "HEAD":
		pathItem.Head = operation
	case "OPTIONS":
		pathItem.Options = operation
	case "PATCH":
		pathItem.Patch = operation
	case "POST":
		pathItem.Post = operation
	case "PUT":
		pathItem.Put = operation
	case "TRACE":
		pathItem.Trace = operation
	default:
		panic(fmt.Errorf("Unsupported HTTP method '%s'", method))
	}
}

func (pathItem *PathItem) Validate(c context.Context) error {
	for _, operation := range pathItem.Operations() {
		if err := operation.Validate(c); err != nil {
			return err
		}
	}
	return nil
}
