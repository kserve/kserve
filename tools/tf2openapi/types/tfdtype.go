package types

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type TFDType int

const (
	// all the possible constants that can be JSON-ified according to
	// https://www.tensorflow.org/tfx/serving/api_rest#json_mapping
	// along with a representation for B64 strings
	DtBool TFDType = iota
	DtB64String
	DtString
	DtInt8
	DtUInt8
	DtInt16
	DtInt32
	DtUInt32
	DtInt64
	DtUInt64
	DtFloat
	DtDouble
)

// Known error messages
const (
	UnsupportedDataTypeError = "tensor (%s) contains unsupported data type (%s) for generating payloads"
)

func NewTFDType(name string, dType string) (TFDType, error) {
	tfDType, ok := map[string]TFDType{
		"DT_BOOL":   DtBool,
		"DT_INT8":   DtInt8,
		"DT_UINT8":  DtUInt8,
		"DT_INT16":  DtInt16,
		"DT_INT32":  DtInt32,
		"DT_UINT32": DtUInt32,
		"DT_INT64":  DtInt64,
		"DT_UINT64": DtUInt64,
		"DT_FLOAT":  DtFloat,
		"DT_DOUBLE": DtDouble,
		"DT_STRING": stringType(name),
	}[dType]
	if !ok {
		return TFDType(0), fmt.Errorf(UnsupportedDataTypeError, name, dType)
	}
	return tfDType, nil
}

func stringType(name string) TFDType {
	if strings.HasSuffix(name, B64KeySuffix) {
		return DtB64String
	}
	return DtString
}

func (t *TFDType) Schema() *openapi3.Schema {
	schema, ok := map[TFDType]*openapi3.Schema{
		DtBool:      openapi3.NewBoolSchema(),
		DtString:    openapi3.NewStringSchema(),
		DtB64String: openapi3.NewObjectSchema().WithProperty("b64", openapi3.NewStringSchema()),
		// JSON should be a decimal number for Ints and UInts
		DtInt8:   openapi3.NewFloat64Schema(),
		DtUInt8:  openapi3.NewFloat64Schema(),
		DtInt16:  openapi3.NewFloat64Schema(),
		DtInt32:  openapi3.NewFloat64Schema(),
		DtUInt32: openapi3.NewFloat64Schema(),
		DtInt64:  openapi3.NewFloat64Schema(),
		DtUInt64: openapi3.NewFloat64Schema(),
		// OpenAPI does NOT support NaN, Inf, -Inf
		// unlike TFServing which permits using these values as numbers for Floats and Doubles
		// (https://www.tensorflow.org/tfx/serving/api_rest#json_conformance)
		DtFloat:  openapi3.NewFloat64Schema(),
		DtDouble: openapi3.NewFloat64Schema(),
	}[*t]
	if !ok {
		panic(fmt.Sprintf("valid dtype (%v) not mapped to schema", t))
	}
	return schema
}
