package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func expectedTFSignatureDef() TFSignatureDef {
	return TFSignatureDef{
		Key:    "Signature Def Key",
		Method: Predict,
		Inputs: []TFTensor{
			{
				Name:  "input",
				DType: DtInt8,
				Shape: TFShape{-1, 3},
				Rank:  2,
			},
		},
		Outputs: []TFTensor{
			{
				Name:  "output",
				DType: DtInt8,
				Shape: TFShape{-1, 3},
				Rank:  2,
			},
		},
	}
}

/* Fake protobuf structs to use as test inputs */
func goodTensorsPb(name string) map[string]*pb.TensorInfo {
	return map[string]*pb.TensorInfo{
		name: {
			Dtype: framework.DataType_DT_INT8,
			TensorShape: &framework.TensorShapeProto{
				Dim: []*framework.TensorShapeProto_Dim{
					{Size: -1},
					{Size: 3},
				},
				UnknownRank: false,
			},
		},
	}
}

func badTensorsPb(name string) map[string]*pb.TensorInfo {
	return map[string]*pb.TensorInfo{
		name: {
			Dtype: framework.DataType_DT_COMPLEX128,
			TensorShape: &framework.TensorShapeProto{
				Dim: []*framework.TensorShapeProto_Dim{
					{Size: -1},
					{Size: 3},
				},
				UnknownRank: false,
			},
		},
	}
}

func TestCreateTFSignatureDefTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSignatureDef, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/predict",
		goodTensorsPb("input"),
		goodTensorsPb("output"))
	expectedSignatureDef := expectedTFSignatureDef()
	g.Expect(tfSignatureDef).Should(gomega.Equal(expectedSignatureDef))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFSignatureDefWithErrInputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := badTensorsPb("input")
	outputTensors := goodTensorsPb("output")
	_, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/predict", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFSignatureDefWithErrOutputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := goodTensorsPb("input")
	outputTensors := badTensorsPb("output")
	_, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/predict", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFSignatureDefWithErrMethod(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := goodTensorsPb("input")
	outputTensors := goodTensorsPb("output")
	_, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/bad", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
