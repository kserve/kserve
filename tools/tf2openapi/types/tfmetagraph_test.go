package types

import (
	"fmt"
	"k8s.io/utils/ptr"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kserve/kserve/tools/tf2openapi/generated/framework"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
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
	g.Expect(tfMetaGraph.Tags).Should(gomega.Equal(expectedMetaGraph.Tags))
	g.Expect(tfMetaGraph.SignatureDefs).Should(gomega.ConsistOf(expectedMetaGraph.SignatureDefs))
	g.Expect(err).Should(gomega.BeNil())
}

func TestNewTFMetaGraphWithErrSignatureDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	metaGraphPb := metaGraphPb()
	metaGraphPb.SignatureDef["sigDefKey"] = badSigDefPb()
	_, err := NewTFMetaGraph(metaGraphPb)
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "inputTensorName", "DT_HALF")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestTFMetaGraphTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph := expectedTFMetaGraph()
	expectedRequestSchema := &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: map[string]*openapi3.SchemaRef{
			"instances": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{openapi3.TypeArray},
							MaxItems: func(u uint64) *uint64 { return &u }(3),
							MinItems: 3,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{openapi3.TypeNumber},
								},
							},
						},
					},
				},
			},
		},
		Required: []string{"instances"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Has:    ptr.To(false),
			Schema: nil,
		},
	}
	expectedResponseSchema := &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: map[string]*openapi3.SchemaRef{
			"predictions": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{openapi3.TypeArray},
							MaxItems: func(u uint64) *uint64 { return &u }(3),
							MinItems: 3,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{openapi3.TypeNumber},
								},
							},
						},
					},
				},
			},
		},
		Required: []string{"predictions"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Has:    ptr.To(false),
			Schema: nil,
		},
	}
	requestSchema, responseSchema, err := tfMetaGraph.Schema("sigDefKey")
	g.Expect(requestSchema).Should(gomega.Equal(expectedRequestSchema))
	g.Expect(responseSchema).Should(gomega.Equal(expectedResponseSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFMetaGraphMissingSigDef(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfMetaGraph := expectedTFMetaGraph()
	_, _, err := tfMetaGraph.Schema("missingSigDefKey")
	expectedErr := fmt.Sprintf(SignatureDefNotFoundError, "missingSigDefKey")
	g.Expect(err).To(gomega.MatchError(expectedErr))
}
