package types

import (
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/utils"
	"github.com/kserve/kserve/tools/tf2openapi/generated/framework"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
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
		Outputs: outputTensors(),
	}
}

func outputTensors() []TFTensor {
	return []TFTensor{
		{
			Name:  "output",
			DType: DtInt8,
			Shape: TFShape{-1, 3},
			Rank:  2,
		},
	}
}

// corresponding response schemas in row and col fmt for SignatureDef above
func expectedResponseSchemaRowFmt() *openapi3.Schema {
	return &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"predictions": {
				Value: &openapi3.Schema{
					Type: "array",
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: utils.UInt64(3),
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
		Required:                    []string{"predictions"},
		AdditionalPropertiesAllowed: utils.Bool(false),
	}

}
func expectedResponseSchemaColFmt() *openapi3.Schema {
	return &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"outputs": {
				Value: &openapi3.Schema{
					Type: "array",
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: utils.UInt64(3),
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
		Required:                    []string{"outputs"},
		AdditionalPropertiesAllowed: utils.Bool(false),
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
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "input", "DT_COMPLEX128")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestCreateTFSignatureDefWithErrOutputs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := goodTensorsPb("input")
	outputTensors := badTensorsPb("output")
	_, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/predict", inputTensors, outputTensors)
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "output", "DT_COMPLEX128")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestCreateTFSignatureDefWithErrMethod(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	inputTensors := goodTensorsPb("input")
	outputTensors := goodTensorsPb("output")
	_, err := NewTFSignatureDef("Signature Def Key", "tensorflow/serving/bad", inputTensors, outputTensors)
	expectedErr := fmt.Sprintf(UnsupportedSignatureMethodError, "Signature Def Key", "tensorflow/serving/bad")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestTFSignatureDefVariousFmt(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		tfSigDef               TFSignatureDef
		expectedRequestSchema  *openapi3.Schema
		expectedResponseSchema *openapi3.Schema
	}{
		"RowSchemaMultipleTensors": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Shape: TFShape{-1, 5},
						Rank:  2,
					},
					{
						Name:  "sensor",
						DType: DtInt8,
						Shape: TFShape{-1, 2, 2},
						Rank:  3,
					},
				},
				Outputs: outputTensors(),
			},
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"instances": {
						Value: &openapi3.Schema{
							Type: "array",
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: "object",
									Properties: map[string]*openapi3.SchemaRef{
										"signal": {
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: utils.UInt64(5),
												MinItems: 5,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type: "number",
													},
												},
											},
										},
										"sensor": {
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: utils.UInt64(2),
												MinItems: 2,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type:     "array",
														MaxItems: utils.UInt64(2),
														MinItems: 2,
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
									Required:                    []string{"signal", "sensor"},
									AdditionalPropertiesAllowed: utils.Bool(false),
								},
							},
						},
					},
				},
				Required:                    []string{"instances"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaRowFmt(),
		},
		"RowSchemaSingleTensor": {
			tfSigDef: expectedTFSignatureDef(),
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"instances": {
						Value: &openapi3.Schema{
							Type: "array",
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:     "array",
									MaxItems: utils.UInt64(3),
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
				Required:                    []string{"instances"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaRowFmt(),
		},
		"ColSchemaMultipleTensors": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Shape: TFShape{2, 5},
						Rank:  2,
					},
					{
						Name:  "sensor",
						DType: DtInt8,
						Shape: TFShape{2, 2, 2},
						Rank:  3,
					},
				},
				Outputs: outputTensors(),
			},
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: "object",
							Properties: map[string]*openapi3.SchemaRef{
								"signal": {
									Value: &openapi3.Schema{
										Type:     "array",
										MaxItems: utils.UInt64(2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: utils.UInt64(5),
												MinItems: 5,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type: "number",
													},
												},
											},
										},
									},
								},
								"sensor": {
									Value: &openapi3.Schema{
										Type:     "array",
										MaxItems: utils.UInt64(2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: utils.UInt64(2),
												MinItems: 2,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type:     "array",
														MaxItems: utils.UInt64(2),
														MinItems: 2,
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
								},
							},
							Required:                    []string{"signal", "sensor"},
							AdditionalPropertiesAllowed: utils.Bool(false),
						},
					},
				},
				Required:                    []string{"inputs"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaColFmt(),
		},
		"ColSchemaSingleTensor": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Shape: TFShape{2, 5},
						Rank:  2,
					},
				},
				Outputs: outputTensors(),
			}, expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: utils.UInt64(2),
							MinItems: 2,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:     "array",
									MaxItems: utils.UInt64(5),
									MinItems: 5,
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
				Required:                    []string{"inputs"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			}, expectedResponseSchema: expectedResponseSchemaColFmt(),
		},
		"UnknownRank": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Rank:  -1,
					},
				},
				Outputs: outputTensors(),
			},
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{},
					},
				},
				Required:                    []string{"inputs"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaColFmt(),
		},
		"Scalar": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Shape: TFShape{},
						Rank:  0,
					},
				},
				Outputs: outputTensors(),
			},
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: "number",
						},
					},
				},
				Required:                    []string{"inputs"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaColFmt(),
		},
		"MultipleScalar": {
			tfSigDef: TFSignatureDef{
				Key: "Signature Def Key",
				Inputs: []TFTensor{
					{
						Name:  "signal",
						DType: DtInt8,
						Shape: TFShape{},
						Rank:  0,
					},
					{
						Name:  "sensor",
						DType: DtInt8,
						Shape: TFShape{},
						Rank:  0,
					},
				},
				Outputs: outputTensors(),
			},
			expectedRequestSchema: &openapi3.Schema{
				Type: "object",
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: "object",
							Properties: map[string]*openapi3.SchemaRef{
								"signal": {
									Value: &openapi3.Schema{
										Type: "number",
									},
								},
								"sensor": {
									Value: &openapi3.Schema{
										Type: "number",
									},
								},
							},
							Required:                    []string{"signal", "sensor"},
							AdditionalPropertiesAllowed: utils.Bool(false),
						},
					},
				},
				Required:                    []string{"inputs"},
				AdditionalPropertiesAllowed: utils.Bool(false),
			},
			expectedResponseSchema: expectedResponseSchemaColFmt(),
		},
	}
	for name, scenario := range scenarios {
		t.Logf("Running %s ...", name)
		requestSchema, responseSchema, err := scenario.tfSigDef.Schema()
		g.Expect(requestSchema).Should(gomega.Equal(scenario.expectedRequestSchema))
		g.Expect(responseSchema).Should(gomega.Equal(scenario.expectedResponseSchema))
		g.Expect(err).To(gomega.BeNil())
	}
}
func TestTFSignatureDefNonPredict(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := expectedTFSignatureDef()
	tfSigDef.Method = Classify
	_, _, err := tfSigDef.Schema()
	g.Expect(err).To(gomega.MatchError(UnsupportedAPISchemaError))
}

func TestTFSignatureDefInconsistentInputOutputFormatError(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
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
				Rank:  -1,
			},
		},
	}
	_, _, err := tfSigDef.Schema()
	g.Expect(err).Should(gomega.MatchError(InconsistentInputOutputFormatError))

}
