package types

import (
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func expectedTFMetaGraph() TFMetaGraph {
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
func desiredSigDefPb() *pb.SignatureDef {
	return &pb.SignatureDef{
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
	}
}

func undesiredSigDefPb() *pb.SignatureDef {
	return &pb.SignatureDef{
		MethodName: "tensorflow/serving/predict",
		Inputs: map[string]*pb.TensorInfo{
			"inputTensorName": {
				Dtype: framework.DataType_DT_INT8,
				TensorShape: &framework.TensorShapeProto{
					Dim: []*framework.TensorShapeProto_Dim{
						{
							Size: -1,
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
					UnknownRank: true,
				},
			},
		},
	}
}

func metaGraphPb() *pb.MetaGraphDef {
	return &pb.MetaGraphDef{
		MetaInfoDef: &pb.MetaGraphDef_MetaInfoDef{
			Tags: []string{
				"tag",
			},
		},
		SignatureDef: map[string]*pb.SignatureDef{
			"sigDefKey": desiredSigDefPb(),
		},
	}
}

func metaGraphPbWithMultipleSigDefs() *pb.MetaGraphDef {
	return &pb.MetaGraphDef{
		MetaInfoDef: &pb.MetaGraphDef_MetaInfoDef{
			Tags: []string{
				"tag",
			},
		},
		SignatureDef: map[string]*pb.SignatureDef{
			"sigDefKey":          desiredSigDefPb(),
			"undesiredSigDefKey": undesiredSigDefPb(),
		},
	}
}

func TestNewTFMetaGraphTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph, err := NewTFMetaGraph(metaGraphPb(), "sigDefKey")
	expectedMetaGraph := expectedTFMetaGraph()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfMetaGraph).Should(gomega.Equal(expectedMetaGraph))
}

func TestNewTFMetaGraphWithMultipleSignatureDefs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph, err := NewTFMetaGraph(metaGraphPbWithMultipleSigDefs(), "sigDefKey")
	expectedMetaGraph := expectedTFMetaGraph()
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(tfMetaGraph).Should(gomega.Equal(expectedMetaGraph))
}

func TestNewTFMetaGraphWithoutDesiredSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := metaGraphPb()
	metaGraphPb.SignatureDef["undesiredSigDefKey"] = metaGraphPb.SignatureDef["sigDefKey"]
	delete(metaGraphPb.SignatureDef, "sigDefKey")
	_, err := NewTFMetaGraph(metaGraphPb, "sigDefKey")
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestNewTFMetaGraphWithErrSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := metaGraphPb()
	metaGraphPb.SignatureDef["sigDefKey"].Inputs["inputTensorName"].Dtype = framework.DataType_DT_HALF
	_, err := NewTFMetaGraph(metaGraphPb, "sigDefKey")
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}
