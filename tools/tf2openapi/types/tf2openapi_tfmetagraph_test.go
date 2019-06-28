package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func makeExpectedTFMetaGraph() TFMetaGraph {
	return TFMetaGraph{
		SignatureDefs: []TFSignatureDef{
			{
				Key: "sigDefKey",
				Inputs: []TFTensor{
					{
						Name:  "inputTensorName",
						DType: DtInt8,
						Shape: TFShape{-1, 3},
						Rank:  2,
					},
				},
				Outputs: []TFTensor{
					{
						Name:  "outputTensorName",
						DType: DtInt8,
						Shape: TFShape{-1, 3},
						Rank:  2,
					},
				},
			},
		},
	}
}

/* Fake protobuf structs to use as test inputs */
func makeMetaGraphPb() *pb.MetaGraphDef {
	return &pb.MetaGraphDef{
		MetaInfoDef: &pb.MetaGraphDef_MetaInfoDef{
			Tags: []string{
				"tag",
			},
		},
		SignatureDef: map[string]*pb.SignatureDef{
			"sigDefKey": {
				MethodName: "tensorflow/serving/predict",
				Inputs: map[string]*pb.TensorInfo{
					"inputTensorName": {
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
				},
				Outputs: map[string]*pb.TensorInfo{
					"outputTensorName": {
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
				},
			},
		},
	}
}

func TestNewTFMetaGraphTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph, err := NewTFMetaGraph(makeMetaGraphPb())
	expectedMetaGraph := makeExpectedTFMetaGraph()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfMetaGraph).Should(gomega.Equal(expectedMetaGraph))
}

func TestNewTFMetaGraphWithoutPredictSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := makeMetaGraphPb()
	metaGraphPb.SignatureDef["sigDefKey"].MethodName = "tensorflow/serving/classify"
	_, err := NewTFMetaGraph(metaGraphPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestNewTFMetaGraphWithErrSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := makeMetaGraphPb()
	metaGraphPb.SignatureDef["sigDefKey"].Inputs["inputTensorName"].Dtype = framework.DataType_DT_HALF
	_, err := NewTFMetaGraph(metaGraphPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
