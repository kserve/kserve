package types

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/* Expected values */
func expectedTFSavedModel() TFSavedModel {
	return TFSavedModel{
		MetaGraphs: []TFMetaGraph{
			{
				Tags: []string{"serve"},
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
			},
		},
	}
}

/* Fake protobuf structs to use as test inputs */
func savedModelPb() *pb.SavedModel {
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
					},
				},
			},
		},
	}
}

func TestNewTFSavedModelTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSavedModel, err := NewTFSavedModel(savedModelPb())
	expectedSavedModel := expectedTFSavedModel()
	g.Expect(tfSavedModel).Should(gomega.Equal(expectedSavedModel))
	g.Expect(err).Should(gomega.BeNil())
}

func TestNewTFSavedModelWithErrMetaGraph(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	savedModelPb := savedModelPb()
	savedModelPb.MetaGraphs[0].SignatureDef["sigDefKey"].Inputs["inputTensorName"].Dtype = framework.DataType_DT_HALF
	_, err := NewTFSavedModel(savedModelPb)
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "inputTensorName", "DT_HALF")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestTFSavedModelTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSavedModel := expectedTFSavedModel()
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
		Required: []string{"instances"},
		AdditionalPropertiesAllowed: func(b bool) *bool {return &b}(false),
	}
	schema, err := tfSavedModel.Schema([]string{"serve"}, "sigDefKey")
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSavedModelMissingMetaGraph(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSavedModel := expectedTFSavedModel()
	_, err := tfSavedModel.Schema([]string{"serve", "missing"}, "sigDefKey")
	expectedErr := fmt.Sprintf(MetaGraphNotFoundError, "[missing serve]")
	expectedErrPermuted := fmt.Sprintf(MetaGraphNotFoundError, "[serve missing]")
	g.Expect(err).To(gomega.Or(gomega.MatchError(expectedErr), gomega.MatchError(expectedErrPermuted)))
}

func TestTFSavedModelErrMetaGraph(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSavedModel := expectedTFSavedModel()
	_, err := tfSavedModel.Schema([]string{"serve"}, "missingSigDefKey")
	expectedErr := fmt.Sprintf(SignatureDefNotFoundError, "missingSigDefKey")
	g.Expect(err).To(gomega.MatchError(expectedErr))
}
