package openapi3gen_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/stretchr/testify/require"
)

type CyclicType0 struct {
	CyclicField *CyclicType1 `json:"a"`
}
type CyclicType1 struct {
	CyclicField *CyclicType0 `json:"b"`
}

func TestCyclic(t *testing.T) {
	schema, refsMap, err := openapi3gen.NewSchemaRefForValue(&CyclicType0{})
	require.IsType(t, &openapi3gen.CycleError{}, err)
	require.Nil(t, schema)
	require.Empty(t, refsMap)
}

func TestSimple(t *testing.T) {
	type ExampleChild string
	type Example struct {
		Bool    bool                     `json:"bool"`
		Int     int                      `json:"int"`
		Int64   int64                    `json:"int64"`
		Float64 float64                  `json:"float64"`
		String  string                   `json:"string"`
		Bytes   []byte                   `json:"bytes"`
		JSON    json.RawMessage          `json:"json"`
		Time    time.Time                `json:"time"`
		Slice   []*ExampleChild          `json:"slice"`
		Map     map[string]*ExampleChild `json:"map"`
		Struct  struct {
			X string `json:"x"`
		} `json:"struct"`
		EmptyStruct struct {
			X string
		} `json:"structWithoutFields"`
		Ptr *ExampleChild `json:"ptr"`
	}

	schema, refsMap, err := openapi3gen.NewSchemaRefForValue(&Example{})
	require.NoError(t, err)
	require.Len(t, refsMap, 14)
	data, err := json.Marshal(schema)
	require.NoError(t, err)
	require.JSONEq(t, expectedSimple, string(data))
}

const expectedSimple = `
{
  "type": "object",
  "properties": {
    "bool": {
      "type": "boolean"
    },
    "int": {
      "type": "integer",
      "format": "int64"
    },
    "int64": {
      "type": "integer",
      "format": "int64"
    },
    "float64": {
      "type": "number"
    },
    "time": {
      "type": "string",
      "format": "date-time"
    },
    "string": {
      "type": "string"
    },
    "bytes": {
      "type": "string",
      "format": "byte"
    },
    "json": {},
    "slice": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "map": {
      "type": "object",
      "additionalProperties": {
        "type": "string"
      }
    },
    "struct": {
      "type": "object",
      "properties": {
        "x": {
          "type": "string"
        }
      }
    },
    "structWithoutFields": {},
    "ptr": {
      "type": "string"
    }
  }
}
`
