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

func NewTFSignatureDef(key string, inputs map[string]*pb.TensorInfo, outputs map[string]*pb.TensorInfo) TFSignatureDef {
	return TFSignatureDef{
		Key:     key,
		Inputs:  extractTensors(inputs),
		Outputs: extractTensors(outputs),
	}
}

func extractTensors(tensors map[string]*pb.TensorInfo) []TFTensor {
	tfTensors := []TFTensor{}
	for key, tensor := range tensors {
		tfTensors = append(tfTensors, NewTFTensor(key, tensor))
	}
	return tfTensors
}

func (t *TFSignatureDef) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
