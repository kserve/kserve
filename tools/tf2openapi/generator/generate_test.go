package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang/protobuf/proto"
	"github.com/onsi/gomega"

	"github.com/kserve/kserve/tools/tf2openapi/generated/framework"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
	"github.com/kserve/kserve/tools/tf2openapi/types"
)

func TestGeneratorBuilderSpecifiedFields(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	builder := &Builder{}
	builder.SetName("model")
	builder.SetVersion("1")
	builder.SetMetaGraphTags([]string{"tag"})
	builder.SetSigDefKey("sigDefKey")
	generator := builder.Build()
	expectedGenerator := Generator{
		name:          "model",
		version:       "1",
		metaGraphTags: []string{"tag"},
		sigDefKey:     "sigDefKey",
	}
	g.Expect(generator).Should(gomega.Equal(expectedGenerator))
}

func TestGeneratorBuilderDefault(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	builder := &Builder{}
	builder.SetName("model")
	builder.SetVersion("1")
	generator := builder.Build()
	expectedGenerator := defaultGenerator()
	g.Expect(generator).Should(gomega.Equal(expectedGenerator))
}

func TestGenerateOpenAPIConstructionErr(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	generator := defaultGenerator()
	model := &pb.SavedModel{
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
								// Unsupported data type will err
								Dtype: framework.DataType_DT_COMPLEX128,
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
	_, specErr := generator.GenerateOpenAPI(model)
	expectedErr := fmt.Sprintf(types.UnsupportedDataTypeError, "inputTensorName", "DT_COMPLEX128")
	g.Expect(specErr).To(gomega.MatchError(expectedErr))
}

func TestGenerateOpenAPISpecGenerationErr(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	generator := defaultGenerator()
	model := &pb.SavedModel{
		MetaGraphs: []*pb.MetaGraphDef{
			{
				MetaInfoDef: &pb.MetaGraphDef_MetaInfoDef{
					Tags: []string{
						"serve",
					},
				},
				SignatureDef: map[string]*pb.SignatureDef{
					"serving_default": {
						MethodName: "tensorflow/serving/classify",
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
	_, specErr := generator.GenerateOpenAPI(model)
	expectedErr := fmt.Sprintf(SpecGenerationError, types.UnsupportedAPISchemaError)
	g.Expect(specErr).To(gomega.MatchError(expectedErr))
}

func TestGenerateOpenAPIForRowFmtMultipleTensors(t *testing.T) {
	// model src: gs://kfserving-samples/models/tensorflow/flowers
	g := gomega.NewGomegaWithT(t)
	model := model(t, "TestRowFmtMultipleTensors")
	generator := defaultGenerator()
	spec, specErr := generator.GenerateOpenAPI(model)
	g.Expect(specErr).Should(gomega.BeNil())

	swagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	expectedSpec := string(openAPI(t, "TestRowFmtMultipleTensors"))
	expectedSwagger := &openapi3.Swagger{}
	// remove any formatting from expectedSpec
	buffer := new(bytes.Buffer)
	if err := json.Compact(buffer, []byte(expectedSpec)); err != nil {
		t.Fatal(err)
	}
	g.Expect(json.Unmarshal(buffer.Bytes(), &expectedSwagger)).To(gomega.Succeed())

	// test equality, ignoring order in JSON arrays
	instances := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["instances"].Value.Items.Value
	expectedInstances := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["instances"].Value.Items.Value
	expectJsonEquality(instances, expectedInstances, g)

	predictions := swagger.Components.Responses["modelOutput"].Value.Content.Get("application/json").
		Schema.Value.Properties["predictions"].Value.Items.Value
	expectedPredictions := expectedSwagger.Components.Responses["modelOutput"].Value.Content.Get("application/json").
		Schema.Value.Properties["predictions"].Value.Items.Value
	expectJsonEquality(predictions, expectedPredictions, g)
}

func TestGenerateOpenAPIForColFmtMultipleTensors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	model := model(t, "TestColFmtMultipleTensors")
	generator := defaultGenerator()
	spec, specErr := generator.GenerateOpenAPI(model)
	g.Expect(specErr).Should(gomega.BeNil())

	swagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	expectedSpec := string(openAPI(t, "TestColFmtMultipleTensors"))
	expectedSwagger := &openapi3.Swagger{}
	// remove any formatting from expectedSpec
	buffer := new(bytes.Buffer)
	if err := json.Compact(buffer, []byte(expectedSpec)); err != nil {
		t.Fatal(err)
	}
	g.Expect(json.Unmarshal(buffer.Bytes(), &expectedSwagger)).To(gomega.Succeed())

	// ignore order in JSON arrays
	inputs := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["inputs"].Value
	expectedInputs := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["inputs"].Value
	expectJsonEquality(inputs, expectedInputs, g)
	g.Expect(swagger.Components.Responses).Should(gomega.Equal(expectedSwagger.Components.Responses))
}

func expectJsonEquality(actual *openapi3.Schema, expected *openapi3.Schema, g *gomega.GomegaWithT) {
	g.Expect(actual.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(actual.Required).To(gomega.ConsistOf(expected.Required))
	g.Expect(actual.Properties).Should(gomega.Not(gomega.BeNil()))
	g.Expect(actual.Properties).Should(gomega.Equal(expected.Properties))
	g.Expect(actual.AdditionalPropertiesAllowed).Should(gomega.Equal(expected.AdditionalPropertiesAllowed))
}

func TestGenerateOpenAPIForVariousFmtsStrictly(t *testing.T) {
	inputFmts := []struct {
		name string
	}{
		{"TestColFmtSingleTensor"},
		{"TestColFmtScalar"},
		{"TestRowFmtSingleTensor"},
	}
	for _, fmt := range inputFmts {
		g := gomega.NewGomegaWithT(t)
		model := model(t, fmt.name)
		generator := defaultGenerator()
		spec, specErr := generator.GenerateOpenAPI(model)
		expectedSpec := openAPI(t, fmt.name)
		g.Expect(spec).Should(gomega.MatchJSON(expectedSpec))
		g.Expect(specErr).Should(gomega.BeNil())
	}
}

func TestAcceptsValidTFServingInput(t *testing.T) {
	inputFmts := []struct {
		name string
	}{
		{"TestColFmtSingleTensor"},
		{"TestColFmtScalar"},
		{"TestRowFmtSingleTensor"},
		{"TestColFmtMultipleTensors"},
		{"TestRowFmtMultipleTensors"},
	}
	for _, fmt := range inputFmts {
		g := gomega.NewGomegaWithT(t)
		g.Expect(acceptsValidReq(t, fmt.name)).Should(gomega.BeNil())
	}
}

func defaultGenerator() Generator {
	return Generator{
		name:          "model",
		version:       "1",
		metaGraphTags: []string{DefaultTag},
		sigDefKey:     DefaultSigDefKey,
	}
}

func model(t *testing.T, fName string) *pb.SavedModel {
	model := &pb.SavedModel{}
	fPath := filepath.Join("testdata", fName+".pb")
	modelPb, err := ioutil.ReadFile(fPath)
	if err != nil {
		t.Fatalf("failed reading %s: %s", fPath, err)
	}
	if err := proto.Unmarshal(modelPb, model); err != nil {
		t.Fatal("SavedModel not in expected format. May be corrupted: " + err.Error())
	}
	return model
}

func openAPI(t *testing.T, fName string) []byte {
	fPath := filepath.Join("testdata", fName+".golden.json")
	openAPI, err := ioutil.ReadFile(fPath)
	if err != nil {
		t.Fatalf("failed reading %s: %s", fPath, err)
	}
	return openAPI
}

func acceptsValidReq(t *testing.T, fName string) error {
	router := openapi3filter.NewRouter().WithSwagger(loadSwagger(t, fName))
	req, reqErr := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict",
		bytes.NewReader(loadPayload(t, fName)))
	if reqErr != nil {
		t.Fatalf("error creating request: %s", reqErr)
	}
	route, pathParams, routeErr := router.FindRoute(req.Method, req.URL)
	if routeErr != nil {
		t.Fatalf("error finding route: %s", routeErr)
	}
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	return openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
}

func loadSwagger(t *testing.T, fName string) *openapi3.Swagger {
	fPath := filepath.Join("testdata", fName+".golden.json")
	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromFile(fPath)
	if err != nil {
		t.Fatalf("failed reading %s: %s", fPath, err)
	}
	return swagger
}

func loadPayload(t *testing.T, fName string) []byte {
	fPath := filepath.Join("testdata", fName+"Req.json")
	payload, err := ioutil.ReadFile(fPath)
	if err != nil {
		t.Fatalf("failed reading %s: %s", fPath, err)
	}
	return payload
}
