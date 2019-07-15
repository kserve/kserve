package types

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeflow/kfserving/tools/tf2openapi/generated/framework"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"testing"
)

/** Utils **/
func newBool(b bool) *bool {
	return &b
}

func newUInt64(u uint64) *uint64 {
	return &u
}

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

func TestTFSignatureDefRowSchemaMultipleTensors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
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
		Outputs: []TFTensor{
			{
				Name:  "output",
				DType: DtInt8,
				Shape: TFShape{-1, 3},
				Rank:  2,
			},
		},
	}
	expectedSchema := &openapi3.Schema{
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
										MaxItems: newUInt64(5),
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
										MaxItems: newUInt64(2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: newUInt64(2),
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
							AdditionalPropertiesAllowed: newBool(false),
						},
					},
				},
			},
		},
		Required:                    []string{"instances"},
		AdditionalPropertiesAllowed: newBool(false),
	}

	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefRowSchemaSingleTensor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := expectedTFSignatureDef()
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"instances": {
				Value: &openapi3.Schema{
					Type: "array",
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: newUInt64(3),
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
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefNonPredict(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := expectedTFSignatureDef()
	tfSigDef.Method = Classify
	_, err := tfSigDef.Schema()
	g.Expect(err).To(gomega.MatchError(UnsupportedAPISchemaError))
}

func TestTFSignatureDefColSchemaMultipleTensors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
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
		Outputs: []TFTensor{
			{
				Name:  "output",
				DType: DtInt8,
				Shape: TFShape{-1, 3},
				Rank:  2,
			},
		},
	}
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"inputs": {
				Value: &openapi3.Schema{
					Type: "object",
					Properties: map[string]*openapi3.SchemaRef{
						"signal": {
							Value: &openapi3.Schema{
								Type:     "array",
								MaxItems: newUInt64(2),
								MinItems: 2,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type:     "array",
										MaxItems: newUInt64(5),
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
								MaxItems: newUInt64(2),
								MinItems: 2,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type:     "array",
										MaxItems: newUInt64(2),
										MinItems: 2,
										Items: &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type:     "array",
												MaxItems: newUInt64(2),
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
					AdditionalPropertiesAllowed: newBool(false),
				},
			},
		},
		Required:                    []string{"inputs"},
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefColSchemaSingleTensor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
		Key: "Signature Def Key",
		Inputs: []TFTensor{
			{
				Name:  "signal",
				DType: DtInt8,
				Shape: TFShape{2, 5},
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
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"inputs": {
				Value: &openapi3.Schema{
					Type:     "array",
					MaxItems: newUInt64(2),
					MinItems: 2,
					Items: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     "array",
							MaxItems: newUInt64(5),
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
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefSchemaUnknownRank(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
		Key: "Signature Def Key",
		Inputs: []TFTensor{
			{
				Name:  "signal",
				DType: DtInt8,
				Rank:  -1,
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
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"inputs": {
				Value: &openapi3.Schema{},
			},
		},
		Required:                    []string{"inputs"},
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefSchemaScalar(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
		Key: "Signature Def Key",
		Inputs: []TFTensor{
			{
				Name:  "signal",
				DType: DtInt8,
				Shape: TFShape{},
				Rank:  0,
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
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"inputs": {
				Value: &openapi3.Schema{
					Type: "number",
				},
			},
		},
		Required:                    []string{"inputs"},
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}

func TestTFSignatureDefSchemaMultipleScalar(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfSigDef := TFSignatureDef{
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
		Outputs: []TFTensor{
			{
				Name:  "output",
				DType: DtInt8,
				Shape: TFShape{-1, 3},
				Rank:  2,
			},
		},
	}
	expectedSchema := &openapi3.Schema{
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
					AdditionalPropertiesAllowed: newBool(false),
				},
			},
		},
		Required:                    []string{"inputs"},
		AdditionalPropertiesAllowed: newBool(false),
	}
	schema, err := tfSigDef.Schema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
	g.Expect(err).To(gomega.BeNil())
}
