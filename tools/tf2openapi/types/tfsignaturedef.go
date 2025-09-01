package types

/**
TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"errors"
	"fmt"
	ptr "k8s.io/utils/ptr"

	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
)

type TFSignatureDef struct {
	Key     string
	Method  TFMethod
	Inputs  []TFTensor
	Outputs []TFTensor
}

type TFMethod int

const (
	Predict TFMethod = iota
	Classify
	Regress
)

// Known error messages
const (
	UnsupportedSignatureMethodError    = "signature (%s) contains unsupported method (%s)"
	UnsupportedAPISchemaError          = "schemas for classify/regress APIs currently not supported"
	InconsistentInputOutputFormatError = "expecting all output tensors to have -1 in 0th dimension, like the input tensors"
)

func NewTFSignatureDef(key string, method string, inputs map[string]*pb.TensorInfo, outputs map[string]*pb.TensorInfo) (TFSignatureDef, error) {
	inputTensors, inputErr := extractTensors(inputs)
	if inputErr != nil {
		return TFSignatureDef{}, inputErr
	}
	outputTensors, outputErr := extractTensors(outputs)
	if outputErr != nil {
		return TFSignatureDef{}, outputErr
	}
	tfMethod, methodErr := NewTFMethod(key, method)
	if methodErr != nil {
		return TFSignatureDef{}, methodErr
	}
	return TFSignatureDef{
		Key:     key,
		Method:  tfMethod,
		Inputs:  inputTensors,
		Outputs: outputTensors,
	}, nil
}

func extractTensors(tensors map[string]*pb.TensorInfo) ([]TFTensor, error) {
	tfTensors := []TFTensor{}
	for key, tensor := range tensors {
		tfTensor, err := NewTFTensor(key, tensor)
		if err != nil {
			return nil, err
		}
		tfTensors = append(tfTensors, tfTensor)
	}
	return tfTensors, nil
}

func NewTFMethod(key string, method string) (TFMethod, error) {
	tfMethod, ok := map[string]TFMethod{
		"tensorflow/serving/predict":  Predict,
		"tensorflow/serving/classify": Classify,
		"tensorflow/serving/regress":  Regress,
	}[method]
	if !ok {
		return TFMethod(0), fmt.Errorf(UnsupportedSignatureMethodError, key, method)
	}
	return tfMethod, nil
}

func (t *TFSignatureDef) Schema() (*openapi3.Schema, *openapi3.Schema, error) {
	if t.Method != Predict {
		return &openapi3.Schema{}, &openapi3.Schema{}, errors.New(UnsupportedAPISchemaError)
	}
	// response format follows request format
	// https://www.tensorflow.org/tfx/serving/api_rest#response_format_4
	if canHaveRowSchema(t.Inputs) {
		if !canHaveRowSchema(t.Outputs) {
			return &openapi3.Schema{}, &openapi3.Schema{}, errors.New(InconsistentInputOutputFormatError)
		}
		requestSchema, responseSchema := t.rowFormatWrapper()
		return requestSchema, responseSchema, nil
	}

	requestSchema, responseSchema := t.colFormatWrapper()
	return requestSchema, responseSchema, nil
}

func canHaveRowSchema(t []TFTensor) bool {
	for _, input := range t {
		if input.Rank == -1 {
			return false
		}

		// tensor is a scalar or tensor doesn't have -1 in 0th dim
		if len(input.Shape) == 0 || input.Shape[0] != -1 {
			return false
		}
	}
	return true
}

func (t *TFSignatureDef) rowFormatWrapper() (*openapi3.Schema, *openapi3.Schema) {
	// https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_row_format
	return &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"instances": rowSchema(t.Inputs).NewRef(),
			},
			Required: []string{"instances"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has:    ptr.To(false),
				Schema: nil,
			},
		}, &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"predictions": rowSchema(t.Outputs).NewRef(),
			},
			Required: []string{"predictions"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has:    ptr.To(false),
				Schema: nil,
			},
		}
}

func (t *TFSignatureDef) colFormatWrapper() (*openapi3.Schema, *openapi3.Schema) {
	// https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_column_format
	return &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"inputs": colSchema(t.Inputs).NewRef(),
			},
			Required: []string{"inputs"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has:    ptr.To(false),
				Schema: nil,
			},
		}, &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"outputs": colSchema(t.Outputs).NewRef(),
			},
			Required: []string{"outputs"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has:    ptr.To(false),
				Schema: nil,
			},
		}
}

func rowSchema(t []TFTensor) *openapi3.Schema {
	if len(t) == 1 {
		// e.g. [val1, val2, etc.]
		singleTensorSchema := t[0].RowSchema()
		return openapi3.NewArraySchema().WithItems(singleTensorSchema)
	}
	// e.g. [{tensor1: val1, tensor2: val3, ..}, {tensor1: val2, tensor2: val4, ..}..]
	multiTensorSchema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	schema := openapi3.NewArraySchema().WithItems(multiTensorSchema)
	for _, i := range t {
		schema.Items.Value.Properties[i.Name] = i.RowSchema().NewRef()
		schema.Items.Value.Required = append(schema.Items.Value.Required, i.Name)
	}
	schema.AdditionalProperties = openapi3.AdditionalProperties{
		Has:    ptr.To(false),
		Schema: nil,
	}
	return schema
}

func colSchema(t []TFTensor) *openapi3.Schema {
	if len(t) == 1 {
		// e.g. val
		return t[0].ColSchema()
	}
	// e.g. {tensor1: [val1, val2, ..], tensor2: [val3, val4, ..] ..}
	schema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	for _, i := range t {
		schema.Properties[i.Name] = i.ColSchema().NewRef()
		schema.Required = append(schema.Required, i.Name)
	}
	schema.AdditionalProperties = openapi3.AdditionalProperties{
		Has:    ptr.To(false),
		Schema: nil,
	}
	return schema
}
