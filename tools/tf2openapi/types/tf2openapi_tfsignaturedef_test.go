package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func makeExpectedTFSignatureDef() TFSignatureDef {
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
func makeTensorsPb(name string, willErr bool) map[string]*pb.TensorInfo {
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
		makeTensorsPb("input", false),
		makeTensorsPb("output", false))
	expectedSignatureDef := makeExpectedTFSignatureDef()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfSignatureDef).Should(gomega.Equal(expectedSignatureDef))
}

func TestCreateTFSignatureDefWithErrInputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := makeTensorsPb("input", true)
	outputTensors := makeTensorsPb("output", false)
	_, err := NewTFSignatureDef("Signature Def Key", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestCreateTFSignatureDefWithErrOutputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := makeTensorsPb("input", false)
	outputTensors := makeTensorsPb("output", true)
	_, err := NewTFSignatureDef("Signature Def Key", inputTensors, outputTensors)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
