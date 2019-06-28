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
		Key: "Signature Def Key",
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
func tensorsPb(name string, willErr bool) map[string]*pb.TensorInfo {
	if willErr {
		return map[string]*pb.TensorInfo{
			name: {
				Dtype: framework.DataType_DT_COMPLEX128,
				TensorShape: &framework.TensorShapeProto{
					Dim: []*framework.TensorShapeProto_Dim{
						{
							Size: -1,
						},
						{
							Size: 3,
						},
					},
					UnknownRank: false,
				},
			},
		}
	}
	return map[string]*pb.TensorInfo{
		name: {
			Dtype: framework.DataType_DT_INT8,
			TensorShape: &framework.TensorShapeProto{
				Dim: []*framework.TensorShapeProto_Dim{
					{
						Size: -1,
					},
					{
						Size: 3,
					},
				},
				UnknownRank: false,
			},
		},
	}
}

func TestCreateTFSignatureDefTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSignatureDef, err := NewTFSignatureDef("Signature Def Key",
		tensorsPb("input", false),
		tensorsPb("output", false))
	expectedSignatureDef := expectedTFSignatureDef()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfSignatureDef).Should(gomega.Equal(expectedSignatureDef))
}

func TestCreateTFSignatureDefWithErrInputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := tensorsPb("input", true)
	outputTensors := tensorsPb("output", false)
	_, err := NewTFSignatureDef("Signature Def Key", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFSignatureDefWithErrOutputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := tensorsPb("input", false)
	outputTensors := tensorsPb("output", true)
	_, err := NewTFSignatureDef("Signature Def Key", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
