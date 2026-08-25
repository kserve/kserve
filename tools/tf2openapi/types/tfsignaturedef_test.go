package types

import (
	"fmt"
	"k8s.io/utils/ptr"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/onsi/gomega"

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
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: map[string]*openapi3.SchemaRef{
			"predictions": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{openapi3.TypeArray},
							MaxItems: ptr.To[uint64](3),
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

}
func expectedResponseSchemaColFmt() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeArray},
		Properties: map[string]*openapi3.SchemaRef{
			"outputs": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{openapi3.TypeArray},
							MaxItems: ptr.To[uint64](3),
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
		Required: []string{"outputs"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Has:    ptr.To(false),
			Schema: nil,
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"instances": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeArray},
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{openapi3.TypeArray},
									Properties: map[string]*openapi3.SchemaRef{
										"signal": {
											Value: &openapi3.Schema{
												Type:     &openapi3.Types{openapi3.TypeArray},
												MaxItems: ptr.To[uint64](5),
												MinItems: 5,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type: &openapi3.Types{openapi3.TypeNumber},
													},
												},
											},
										},
										"sensor": {
											Value: &openapi3.Schema{
												Type:     &openapi3.Types{openapi3.TypeArray},
												MaxItems: ptr.To[uint64](2),
												MinItems: 2,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type:     &openapi3.Types{openapi3.TypeArray},
														MaxItems: ptr.To[uint64](2),
														MinItems: 2,
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
									Required: []string{"signal", "sensor"},
									AdditionalProperties: openapi3.AdditionalProperties{
										Has:    ptr.To(false),
										Schema: nil,
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
			},
			expectedResponseSchema: expectedResponseSchemaRowFmt(),
		},
		"RowSchemaSingleTensor": {
			tfSigDef: expectedTFSignatureDef(),
			expectedRequestSchema: &openapi3.Schema{
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"instances": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeArray},
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:     &openapi3.Types{openapi3.TypeArray},
									MaxItems: ptr.To[uint64](3),
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeArray},
							Properties: map[string]*openapi3.SchemaRef{
								"signal": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{openapi3.TypeArray},
										MaxItems: ptr.To[uint64](2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     &openapi3.Types{openapi3.TypeArray},
												MaxItems: ptr.To[uint64](5),
												MinItems: 5,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type: &openapi3.Types{openapi3.TypeNumber},
													},
												},
											},
										},
									},
								},
								"sensor": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{openapi3.TypeArray},
										MaxItems: ptr.To[uint64](2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     &openapi3.Types{openapi3.TypeArray},
												MaxItems: ptr.To[uint64](2),
												MinItems: 2,
												Items: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type:     &openapi3.Types{openapi3.TypeArray},
														MaxItems: ptr.To[uint64](2),
														MinItems: 2,
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
								},
							},
							Required: []string{"signal", "sensor"},
							AdditionalProperties: openapi3.AdditionalProperties{
								Has:    ptr.To(false),
								Schema: nil,
							},
						},
					},
				},
				Required: []string{"inputs"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has:    ptr.To(false),
					Schema: nil,
				},
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{openapi3.TypeArray},
							MaxItems: ptr.To[uint64](2),
							MinItems: 2,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:     &openapi3.Types{openapi3.TypeArray},
									MaxItems: ptr.To[uint64](5),
									MinItems: 5,
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
				Required: []string{"inputs"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has:    ptr.To(false),
					Schema: nil,
				},
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{},
					},
				},
				Required: []string{"inputs"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has:    ptr.To(false),
					Schema: nil,
				},
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeNumber},
						},
					},
				},
				Required: []string{"inputs"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has:    ptr.To(false),
					Schema: nil,
				},
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
				Type: &openapi3.Types{openapi3.TypeArray},
				Properties: map[string]*openapi3.SchemaRef{
					"inputs": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeArray},
							Properties: map[string]*openapi3.SchemaRef{
								"signal": {
									Value: &openapi3.Schema{
										Type: &openapi3.Types{openapi3.TypeNumber},
									},
								},
								"sensor": {
									Value: &openapi3.Schema{
										Type: &openapi3.Types{openapi3.TypeNumber},
									},
								},
							},
							Required: []string{"signal", "sensor"},
							AdditionalProperties: openapi3.AdditionalProperties{
								Has:    ptr.To(false),
								Schema: nil,
							},
						},
					},
				},
				Required: []string{"inputs"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has:    ptr.To(false),
					Schema: nil,
				},
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
