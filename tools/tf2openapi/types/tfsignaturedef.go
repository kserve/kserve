package types

/**
TFSignatureDef defines the signature of supported computations in the TensorFlow graph, including their inputs and
outputs. It is the internal model representation for the SignatureDef defined in the TensorFlow repository
[tensorflow/core/protobuf/meta_graph.proto]
*/
import (
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
		return TFMethod(0), fmt.Errorf("signature (%s) contains unsupported method (%s)", key, method)
	}
	return tfMethod, nil
}

func (t *TFSignatureDef) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
