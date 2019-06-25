package types

/**
TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
	"github.com/getkin/kin-openapi/openapi3"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
)

type TFSignatureDef struct {
	Key     string
	Inputs  [] TFTensor
	Outputs [] TFTensor
}

func NewTFSignatureDef(key string, inputs map[string]*pb.TensorInfo, outputs map[string]*pb.TensorInfo) (TFSignatureDef, error) {
	inputTensors, inputErr := extractTensors(inputs)
	if inputErr != nil {
		return TFSignatureDef{}, inputErr
	}
	outputTensors, outputErr := extractTensors(outputs)
	if outputErr != nil {
		return TFSignatureDef{}, outputErr
	}
	return TFSignatureDef{
		Key:     key,
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

func canHaveRowSchema(t []TFTensor) bool {
	for _, input := range t {
		// unknown rank
		if input.Rank == -1 {
			return false
		}
		// not batchable: either 1. tensor is a scalar or
		// 2. model builder didn't follow the convention of having -1 in all 0th dim to indicate batchable inputs
		if len(input.Shape) == 0 || input.Shape[0] != -1 {
			return false
		}
	}
	return true
}

func (t *TFSignatureDef) rowSchema() *openapi3.Schema {
	if len(t.Inputs) == 1 {
		// only one named input tensor
		// while schema can be [{tensor: val1}, {tensor: val2}, ..]
		// choose this schema of the form [val1, val2, etc.]
		singleTensorSchema := t.Inputs[0].Schema(true)
		return openapi3.NewArraySchema().WithItems(singleTensorSchema)
	}
	// multiple named input tensors
	// schema is of the form [{tensor1: val1, tensor2: val3, ..}, {tensor1: val2, tensor2: val4, ..}..]
	multiTensorSchema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	schema := openapi3.NewArraySchema().WithItems(multiTensorSchema)
	for _, i := range t.Inputs {
		schema.Items.Value.Properties[i.Name] = i.Schema(true).NewRef()
		schema.Items.Value.Required = append(schema.Items.Value.Required, i.Name)
	}
	return schema
}

func (t *TFSignatureDef) colSchema() *openapi3.Schema {
	if len(t.Inputs) == 1 {
		// only one named input tensor
		// while schema can be {tensor name: val}
		// choose the schema of the form val
		// see description of FillTensorMapFromInputsMap in TFServing
		// https://github.com/tensorflow/serving/blob/master/tensorflow_serving/util/json_tensor.cc
		return t.Inputs[0].Schema(false)
	}

	// multiple named input tensors
	// schema is of the form {tensor1: [val1, val2, ..], tensor2: [val3, val4, ..] ..}
	schema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
	for _, i := range t.Inputs {
		schema.Properties[i.Name] = i.Schema(false).NewRef()
		schema.Required = append(schema.Required, i.Name)
	}
	return schema

}

func (t *TFSignatureDef) Schema() *openapi3.Schema {
	// Prefer the row format (https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_row_format)
	// when possible - it's more readable
	if canHaveRowSchema(t.Inputs) {
		if len(t.Inputs) == 1 {
			// single input tensor
			singleTensorSchema := t.Inputs[0].Schema(true)
			return openapi3.NewArraySchema().WithItems(singleTensorSchema)

		}
		// multi-input tensor
		// TODO how it differs for row v column

		multiTensorSchema := openapi3.NewObjectSchema().WithProperties(make(map[string]*openapi3.Schema))
		schema := openapi3.NewArraySchema().WithItems(multiTensorSchema)
		for _, i := range t.Inputs {
			schema.Items.Value.Properties[i.Name] = i.Schema(true).NewRef()
			schema.Items.Value.Required = append(schema.Items.Value.Required, i.Name)
		}
		return schema
	}

	// Else, use the column format (https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_column_format)
	colSchema := t.colSchema()
	return openapi3.NewObjectSchema().WithProperty("inputs", colSchema)
}
