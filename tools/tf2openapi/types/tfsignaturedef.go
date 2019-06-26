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
	Name    string
	Inputs  [] TFTensor
	Outputs [] TFTensor
}

func NewTFSignatureDef(key string, inputs map[string]*pb.TensorInfo, outputs map[string]*pb.TensorInfo) TFSignatureDef {
	return TFSignatureDef{}
}

func extractTensors(tensors map[string]*pb.TensorInfo) []TFTensor {
	return []TFTensor{}
}

func (t *TFSignatureDef) Schema() *openapi3.Schema {
	return &openapi3.Schema{}
}
