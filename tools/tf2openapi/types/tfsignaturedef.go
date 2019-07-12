package types

/**
TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"errors"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

type TFSignatureDef struct {
	Key     string
	Method  TFMethod
	Inputs  [] TFTensor
	Outputs [] TFTensor
}

type TFMethod int

const (
	Predict TFMethod = iota
	Classify
	Regress
)

//Known error messages
const (
	UnsupportedSignatureMethodError = "signature (%s) contains unsupported method (%s)"
	UnsupportedAPISchemaError       = "schemas for classify/regress APIs currently not supported"
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

func (t *TFSignatureDef) Schema() (*openapi3.Schema, error) {
	if t.Method != Predict {
		return &openapi3.Schema{}, errors.New(UnsupportedAPISchemaError)
	}
	// https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_row_format
	if canHaveRowSchema(t.Inputs) {
		return openapi3.NewObjectSchema().WithProperty("instances", t.rowSchema()), nil
	}

	// https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_column_format
	return openapi3.NewObjectSchema().WithProperty("inputs", t.colSchema()), nil
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

func (t *TFSignatureDef) rowSchema() *openapi3.Schema {
	if len(t.Inputs) == 1 {
		// e.g. [val1, val2, etc.]
		singleTensorSchema := t.Inputs[0].RowSchema()
		return openapi3.NewArraySchema().WithItems(singleTensorSchema)
	}
	// e.g. [{tensor1: val1, tensor2: val3, ..}, {tensor1: val2, tensor2: val4, ..}..]
	multiTensorSchema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	schema := openapi3.NewArraySchema().WithItems(multiTensorSchema)
	for _, i := range t.Inputs {
		schema.Items.Value.Properties[i.Name] = i.RowSchema().NewRef()
		schema.Items.Value.Required = append(schema.Items.Value.Required, i.Name)
	}
	return schema
}

func (t *TFSignatureDef) colSchema() *openapi3.Schema {
	if len(t.Inputs) == 1 {
		// e.g. val
		return t.Inputs[0].ColSchema()
	}
	// e.g. {tensor1: [val1, val2, ..], tensor2: [val3, val4, ..] ..}
	schema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	for _, i := range t.Inputs {
		schema.Properties[i.Name] = i.ColSchema().NewRef()
		schema.Required = append(schema.Required, i.Name)
	}
	return schema
}
