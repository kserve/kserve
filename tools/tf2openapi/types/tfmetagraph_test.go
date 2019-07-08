package types

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func expectedTFMetaGraph() TFMetaGraph {
	return TFMetaGraph{
		Tags: []string{"tag"},
		SignatureDefs: []TFSignatureDef{
			{
				Key:    "sigDefKey",
				Method: Predict,
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
func goodSigDefPb() *pb.SignatureDef {
	return &pb.SignatureDef{
		MethodName: "tensorflow/serving/predict",
		Inputs: map[string]*pb.TensorInfo{
			"inputTensorName": {
				Dtype: framework.DataType_DT_INT8,
				TensorShape: &framework.TensorShapeProto{
					Dim: []*framework.TensorShapeProto_Dim{
						{Size: -1},
						{Size: 3},
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
						{Size: -1},
						{Size: 3},
					},
					UnknownRank: false,
				},
			},
		},
	}
}

func badSigDefPb() *pb.SignatureDef {
	return &pb.SignatureDef{
		MethodName: "tensorflow/serving/predict",
		Inputs: map[string]*pb.TensorInfo{
			"inputTensorName": {
				// Incompatible Dtype will err
				Dtype: framework.DataType_DT_HALF,
				TensorShape: &framework.TensorShapeProto{
					Dim: []*framework.TensorShapeProto_Dim{
						{Size: -1},
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
			"sigDefKey": goodSigDefPb(),
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
			"sigDefKey":        goodSigDefPb(),
			"anotherSigDefKey": goodSigDefPb(),
		},
	}
}

func TestNewTFMetaGraphTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph, err := NewTFMetaGraph(metaGraphPb())
	expectedMetaGraph := expectedTFMetaGraph()
	g.Expect(tfMetaGraph).Should(gomega.Equal(expectedMetaGraph))
	g.Expect(err).Should(gomega.BeNil())
}

func TestNewTFMetaGraphWithMultipleSignatureDefs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph, err := NewTFMetaGraph(metaGraphPbWithMultipleSigDefs())
	expectedMetaGraph := TFMetaGraph{
		Tags: []string{"tag"},
		SignatureDefs: []TFSignatureDef{
			{
				Key:    "sigDefKey",
				Method: Predict,
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
			{
				Key:    "anotherSigDefKey",
				Method: Predict,
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
	g.Expect(tfMetaGraph).Should(gomega.Equal(expectedMetaGraph))
	g.Expect(err).Should(gomega.BeNil())
}

func TestNewTFMetaGraphWithErrSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := metaGraphPb()
	metaGraphPb.SignatureDef["sigDefKey"] = badSigDefPb()
	_, err := NewTFMetaGraph(metaGraphPb)
	g.Expect(err).Should(gomega.Not(gomega.BeNil()))
}

func TestTFMetaGraphTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph := expectedTFMetaGraph()
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"instances": {
				Value: &openapi3.Schema{
					Type: "array",
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: func(u uint64) *uint64 { return &u }(3),
							MinItems: 3,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: "number",
								},
							},
						},
					},
				},
			},
		},
	}
	schema, err := tfMetaGraph.Schema("sigDefKey")
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}
