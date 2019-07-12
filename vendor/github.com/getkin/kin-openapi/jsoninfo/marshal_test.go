package jsoninfo_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/jsoninfo"
	"github.com/getkin/kin-openapi/openapi3"
)

type Simple struct {
	openapi3.ExtensionProps
	Bool    bool      `json:"bool"`
	Int     int       `json:"int"`
	Int64   int64     `json:"int64"`
	Float64 float64   `json:"float64"`
	Time    time.Time `json:"time"`
	String  string    `json:"string"`
	Bytes   []byte    `json:"bytes"`
}

type SimpleOmitEmpty struct {
	openapi3.ExtensionProps
	Bool    bool      `json:"bool,omitempty"`
	Int     int       `json:"int,omitempty"`
	Int64   int64     `json:"int64,omitempty"`
	Float64 float64   `json:"float64,omitempty"`
	Time    time.Time `json:"time,omitempty"`
	String  string    `json:"string,omitempty"`
	Bytes   []byte    `json:"bytes,omitempty"`
}

type SimplePtrOmitEmpty struct {
	openapi3.ExtensionProps
	Bool    *bool      `json:"bool,omitempty"`
	Int     *int       `json:"int,omitempty"`
	Int64   *int64     `json:"int64,omitempty"`
	Float64 *float64   `json:"float64,omitempty"`
	Time    *time.Time `json:"time,omitempty"`
	String  *string    `json:"string,omitempty"`
	Bytes   *[]byte    `json:"bytes,omitempty"`
}

type OriginalNameType struct {
	openapi3.ExtensionProps
	Field string `json:",omitempty"`
}

type RootType struct {
	openapi3.ExtensionProps
	EmbeddedType0
	EmbeddedType1
}

type EmbeddedType0 struct {
	openapi3.ExtensionProps
	Field0 string `json:"embedded0,omitempty"`
}

type EmbeddedType1 struct {
	openapi3.ExtensionProps
	Field1 string `json:"embedded1,omitempty"`
}

// Example describes expected outcome of:
//   1.Marshal JSON
//   2.Unmarshal value
//   3.Marshal value
type Example struct {
	NoMarshal   bool
	NoUnmarshal bool
	Value       jsoninfo.StrictStruct
	JSON        interface{}
}

var Examples = []Example{
	// Primitives
	{
		Value: &SimpleOmitEmpty{},
		JSON: Object{
			"time": time.Unix(0, 0),
		},
	},
	{
		Value: &SimpleOmitEmpty{},
		JSON: Object{
			"bool":    true,
			"int":     42,
			"int64":   42,
			"float64": 3.14,
			"string":  "abc",
			"bytes":   []byte{1, 2, 3},
			"time":    time.Unix(1, 0),
		},
	},

	// Pointers
	{
		Value: &SimplePtrOmitEmpty{},
		JSON:  Object{},
	},
	{
		Value: &SimplePtrOmitEmpty{},
		JSON: Object{
			"bool":    true,
			"int":     42,
			"int64":   42,
			"float64": 3.14,
			"string":  "abc",
			"bytes":   []byte{1, 2, 3},
			"time":    time.Unix(1, 0),
		},
	},

	// JSON tag "fieldName"
	{
		Value: &Simple{},
		JSON: Object{
			"bool":    false,
			"int":     0,
			"int64":   0,
			"float64": 0,
			"string":  "",
			"bytes":   []byte{},
			"time":    time.Unix(0, 0),
		},
	},

	// JSON tag ",omitempty"
	{
		Value: &OriginalNameType{},
		JSON: Object{
			"Field": "abc",
		},
	},

	// Embedding
	{
		Value: &RootType{},
		JSON:  Object{},
	},
	{
		Value: &RootType{},
		JSON: Object{
			"embedded0": "0",
			"embedded1": "1",
			"x-other":   "abc",
		},
	},
}

type Object map[string]interface{}

func TestExtensions(t *testing.T) {
	for _, example := range Examples {
		// Define JSON that will be unmarshalled
		expectedData, err := json.Marshal(example.JSON)
		if err != nil {
			panic(err)
		}
		expected := string(expectedData)

		// Define value that will marshalled
		x := example.Value

		// Unmarshal
		if !example.NoUnmarshal {
			t.Logf("Unmarshalling %T", x)
			if err := jsoninfo.UnmarshalStrictStruct(expectedData, x); err != nil {
				t.Fatalf("Error unmarshalling %T: %v", x, err)
			}
			t.Logf("Marshalling %T", x)
		}

		// Marshal
		if !example.NoMarshal {
			data, err := jsoninfo.MarshalStrictStruct(x)
			if err != nil {
				t.Fatalf("Error marshalling: %v", err)
			}
			actually := string(data)

			if actually != expected {
				t.Fatalf("Error!\nExpected: %s\nActually: %s", expected, actually)
			}
		}
	}
}
