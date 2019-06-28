package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func makeExpectedTFSavedModel() TFSavedModel {
	return TFSavedModel{
		MetaGraphs: []TFMetaGraph{
			{
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
			},
		},
	}
}

/* Fake protobuf structs to use as test inputs */
func makeSavedModelPb() *pb.SavedModel {
	return &pb.SavedModel{
		MetaGraphs: []*pb.MetaGraphDef{
			{
				MetaInfoDef: &pb.MetaGraphDef_MetaInfoDef{
					Tags: []string{
						"serve",
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
			},
		},
	}
}

func TestNewTFSavedModelTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSavedModel, err := NewTFSavedModel(makeSavedModelPb())
	expectedSavedModel := makeExpectedTFSavedModel()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfSavedModel).Should(gomega.Equal(expectedSavedModel))
}

func TestNewTFSavedModelWithErrMetaGraph(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	savedModelPb := makeSavedModelPb()
	savedModelPb.MetaGraphs[0].SignatureDef["sigDefKey"].MethodName = "tensorflow/serving/classify"
	_, err := NewTFSavedModel(savedModelPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestNewTFSavedModelWithNoServableMetaGraph(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	savedModelPb := makeSavedModelPb()
	savedModelPb.MetaGraphs[0].MetaInfoDef.Tags[0] = "tag"
	_, err := NewTFSavedModel(savedModelPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
