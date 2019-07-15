package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang/protobuf/proto"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"
)

func testReadFiles(t *testing.T) (*pb.SavedModel, []byte) {
	model := &pb.SavedModel{}
	modelPb, err := ioutil.ReadFile(filepath.Join("testdata", t.Name()+".pb"))
	if err != nil {
		t.Fatalf("failed reading .pb: %s", err)
	}
	if err := proto.Unmarshal(modelPb, model); err != nil {
		t.Fatal("SavedModel not in expected format. May be corrupted: " + err.Error())
	}
	openAPI, err := ioutil.ReadFile(filepath.Join("testdata", t.Name()+".golden"))
	if err != nil {
		t.Fatalf("failed reading .golden: %s", err)
	}
	return model, openAPI
}

func testLoadPayload(t *testing.T) (*openapi3.Swagger, []byte) {
	loader := openapi3.NewSwaggerLoader()
	swagger, e1 := loader.LoadSwaggerFromFile(filepath.Join("testdata", t.Name()+".golden"))
	if e1 != nil {
		t.Fatalf("failed reading .golden: %s", e1)
	}
	payload, e2 := ioutil.ReadFile(filepath.Join("testdata", t.Name()+"Req.json"))
	if e2 != nil {
		t.Fatalf("failed reading .json: %s", e2)
	}
	return swagger, payload
}

func TestRowFmtMultipleTensors(t *testing.T) {
	// model src: gs://kfserving-samples/models/tensorflow/flowers
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	g.Expect(specErr).Should(gomega.BeNil())
	expectedSpec := string(goldenFile)
	var swagger openapi3.Swagger
	var expectedSwagger openapi3.Swagger
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	// remove any formatting from golden file: tabs or newlines
	buffer := new(bytes.Buffer)
	if err := json.Compact(buffer, []byte(expectedSpec)); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(buffer.Bytes(), &expectedSwagger)

	// walk through structure because can't do a simple JSON comparison- gomega checks for order in array, but "required" array is unordered
	instances := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["instances"].Value
	expectedInstances := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["instances"].Value
	g.Expect(instances.Items.Value.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(instances.Items.Value.Required).To(gomega.ConsistOf(expectedInstances.Items.Value.Required))
	g.Expect(instances.Items.Value.AdditionalPropertiesAllowed).Should(gomega.Equal(expectedInstances.Items.Value.AdditionalPropertiesAllowed))
	g.Expect(instances.Items.Value.Properties).Should(gomega.Equal(expectedInstances.Items.Value.Properties))

	// OpenAPI accepts real, valid input
	openapi, payload := testLoadPayload(t)
	router := openapi3filter.NewRouter().WithSwagger(openapi)
	req, _ := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	route, pathParams, _ := router.FindRoute(req.Method, req.URL)
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}

func TestColFmtMultipleTensors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	g.Expect(specErr).Should(gomega.BeNil())

	expectedSpec := string(goldenFile)
	var swagger openapi3.Swagger
	var expectedSwagger openapi3.Swagger
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	// remove any formatting from golden file: tabs or newlines
	buffer := new(bytes.Buffer)
	if err := json.Compact(buffer, []byte(expectedSpec)); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(buffer.Bytes(), &expectedSwagger)

	// walk through structure because can't do a simple JSON comparison- gomega checks for order in array, but "required" array is unordered
	inputs := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["inputs"].Value
	expectedInputs := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["inputs"].Value
	g.Expect(inputs.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(inputs.Required).To(gomega.ConsistOf(expectedInputs.Required))
	g.Expect(inputs.Properties).Should(gomega.Equal(expectedInputs.Properties))
	g.Expect(inputs.AdditionalPropertiesAllowed).Should(gomega.Equal(expectedInputs.AdditionalPropertiesAllowed))

	// OpenAPI accepts real, valid input
	openapi, payload := testLoadPayload(t)
	router := openapi3filter.NewRouter().WithSwagger(openapi)
	req, _ := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	route, pathParams, _ := router.FindRoute(req.Method, req.URL)
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}

func TestColFmtSingleTensor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	expectedSpec := string(goldenFile)
	g.Expect(spec).Should(gomega.MatchJSON(expectedSpec))
	g.Expect(specErr).Should(gomega.BeNil())

	// OpenAPI accepts real, valid input
	swagger, payload := testLoadPayload(t)
	router := openapi3filter.NewRouter().WithSwagger(swagger)
	req, _ := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	route, pathParams, _ := router.FindRoute(req.Method, req.URL)
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}

func TestColFmtScalar(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	expectedSpec := string(goldenFile)
	g.Expect(spec).Should(gomega.MatchJSON(expectedSpec))
	g.Expect(specErr).Should(gomega.BeNil())

	// OpenAPI accepts real, valid input
	swagger, payload := testLoadPayload(t)
	router := openapi3filter.NewRouter().WithSwagger(swagger)
	req, _ := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	route, pathParams, _ := router.FindRoute(req.Method, req.URL)
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}

func TestRowFmtSingleTensor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	expectedSpec := string(goldenFile)
	g.Expect(spec).Should(gomega.MatchJSON(expectedSpec))
	g.Expect(specErr).Should(gomega.BeNil())

	// OpenAPI accepts real, valid input
	swagger, payload := testLoadPayload(t)
	router := openapi3filter.NewRouter().WithSwagger(swagger)
	req, _ := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	route, pathParams, _ := router.FindRoute(req.Method, req.URL)
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}

func TestComplex(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	model := &pb.SavedModel{}
	modelPb, err := ioutil.ReadFile(("testdata/saved_model.pb"))
	if err != nil {
		t.Fatalf("failed reading .pb: %s", err)
	}
	if err := proto.Unmarshal(modelPb, model); err != nil {
		t.Fatal("SavedModel not in expected format. May be corrupted: " + err.Error())
	}

	// OpenAPI spec is as expected (API has not changed)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "predict")
	g.Expect(specErr).Should(gomega.BeNil())

	// remove any formatting from golden file: tabs or newlines
	loader := openapi3.NewSwaggerLoader()
	swagger, e1 := loader.LoadSwaggerFromData([]byte(spec))
	if e1 != nil {
		t.Fatalf("eep: %s", e1)
	}
	payload, e2 := ioutil.ReadFile("testdata/req.json")
	if e2 != nil {
		t.Fatalf("failed reading .json: %s", e2)
	}
	// OpenAPI accepts real, valid input
	router := openapi3filter.NewRouter().WithSwagger(swagger)
	req, err := http.NewRequest(http.MethodPost, "/v1/models/model/versions/1:predict", bytes.NewReader(payload))
	g.Expect(err).Should(gomega.BeNil())
	route, pathParams, err2 := router.FindRoute(req.Method, req.URL)
	g.Expect(err2).Should(gomega.BeNil())
	req.Header.Set("Content-Type", "application/json")
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	payloadErr := openapi3filter.ValidateRequest(context.TODO(), requestValidationInput)
	g.Expect(payloadErr).Should(gomega.BeNil())
}
