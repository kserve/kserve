// Package openapi3gen generates OpenAPI 3 schemas for Go types.
package openapi3gen

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/jsoninfo"
	"github.com/getkin/kin-openapi/openapi3"
)

// CycleError indicates that a type graph has one or more possible cycles.
type CycleError struct{}

func (err *CycleError) Error() string {
	return "Detected JSON cycle"
}

func NewSchemaRefForValue(value interface{}) (*openapi3.SchemaRef, map[*openapi3.SchemaRef]int, error) {
	g := NewGenerator()
	ref, err := g.GenerateSchemaRef(reflect.TypeOf(value))
	for ref := range g.SchemaRefs {
		ref.Ref = ""
	}
	return ref, g.SchemaRefs, err
}

type Generator struct {
	Types map[reflect.Type]*openapi3.SchemaRef

	// SchemaRefs contains all references and their counts.
	// If count is 1, it's not ne
	// An OpenAPI identifier has been assigned to each.
	SchemaRefs map[*openapi3.SchemaRef]int
}

func NewGenerator() *Generator {
	return &Generator{
		Types:      make(map[reflect.Type]*openapi3.SchemaRef),
		SchemaRefs: make(map[*openapi3.SchemaRef]int),
	}
}

func (g *Generator) GenerateSchemaRef(t reflect.Type) (*openapi3.SchemaRef, error) {
	return g.generateSchemaRefFor(nil, t)
}

func (g *Generator) generateSchemaRefFor(parents []*jsoninfo.TypeInfo, t reflect.Type) (*openapi3.SchemaRef, error) {
	ref := g.Types[t]
	if ref != nil {
		g.SchemaRefs[ref]++
		return ref, nil
	}
	ref, err := g.generateWithoutSaving(parents, t)
	if ref != nil {
		g.Types[t] = ref
		g.SchemaRefs[ref]++
	}
	return ref, err
}

func (g *Generator) generateWithoutSaving(parents []*jsoninfo.TypeInfo, t reflect.Type) (*openapi3.SchemaRef, error) {
	// Get TypeInfo
	typeInfo := jsoninfo.GetTypeInfo(t)
	for _, parent := range parents {
		if parent == typeInfo {
			return nil, &CycleError{}
		}
	}

	// Doesn't exist.
	// Create the schema.
	if cap(parents) == 0 {
		parents = make([]*jsoninfo.TypeInfo, 0, 4)
	}
	parents = append(parents, typeInfo)

	// Ignore pointers
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Create instance
	if strings.HasSuffix(t.Name(), "Ref") {
		_, a := t.FieldByName("Ref")
		v, b := t.FieldByName("Value")
		if a && b {
			vs, err := g.generateSchemaRefFor(parents, v.Type)
			if err != nil {
				return nil, err
			}
			refSchemaRef := RefSchemaRef
			g.SchemaRefs[refSchemaRef]++
			ref := openapi3.NewSchemaRef(t.Name(), &openapi3.Schema{
				OneOf: []*openapi3.SchemaRef{
					refSchemaRef,
					vs,
				},
			})
			g.SchemaRefs[ref]++
			return ref, nil
		}
	}

	// Allocate schema
	schema := &openapi3.Schema{}

	switch t.Kind() {
	case reflect.Func, reflect.Chan:
		return nil, nil
	case reflect.Bool:
		schema.Type = "boolean"

	case reflect.Int,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = "integer"
		schema.Format = "int64"

	case reflect.Float32, reflect.Float64:
		schema.Type = "number"

	case reflect.String:
		schema.Type = "string"

	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			if t == rawMessageType {
				return &openapi3.SchemaRef{
					Value: schema,
				}, nil
			}
			schema.Type = "string"
			schema.Format = "byte"
		} else {
			schema.Type = "array"
			items, err := g.generateSchemaRefFor(parents, t.Elem())
			if err != nil {
				return nil, err
			}
			if items != nil {
				g.SchemaRefs[items]++
				schema.Items = items
			}
		}

	case reflect.Map:
		schema.Type = "object"
		additionalProperties, err := g.generateSchemaRefFor(parents, t.Elem())
		if err != nil {
			return nil, err
		}
		if additionalProperties != nil {
			g.SchemaRefs[additionalProperties]++
			schema.AdditionalProperties = additionalProperties
		}

	case reflect.Struct:
		if t == timeType {
			schema.Type = "string"
			schema.Format = "date-time"
		} else {
			for _, fieldInfo := range typeInfo.Fields {
				// Only fields with JSON tag are considered
				if !fieldInfo.HasJSONTag {
					continue
				}
				ref, err := g.generateSchemaRefFor(parents, fieldInfo.Type)
				if err != nil {
					return nil, err
				}
				if ref != nil {
					g.SchemaRefs[ref]++
					schema.WithPropertyRef(fieldInfo.JSONName, ref)
				}
			}

			// Object only if it has properties
			if schema.Properties != nil {
				schema.Type = "object"
			}
		}
	}
	return openapi3.NewSchemaRef(t.Name(), schema), nil
}

var RefSchemaRef = openapi3.NewSchemaRef("Ref",
	openapi3.NewObjectSchema().WithProperty("$ref", openapi3.NewStringSchema().WithMinLength(1)))

var (
	timeType       = reflect.TypeOf(time.Time{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
)
