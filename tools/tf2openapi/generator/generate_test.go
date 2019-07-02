package generator

import (
	"bytes"
	"context"
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

func testLoadPayload(t *testing.T) (*openapi3.Swagger, []byte){
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

func TestColumnFmtMultipleTensors(t *testing.T) {
	// model src: gs://kfserving-samples/models/tensorflow/flowers
	g := gomega.NewGomegaWithT(t)

	// OpenAPI spec is as expected (API has not changed)
	model, goldenFile := testReadFiles(t)
	spec, specErr := GenerateOpenAPI(model, "model", "1", "serving_default")
	expectedSpec := string(goldenFile)
	g.Expect(spec).Should(gomega.MatchJSON(expectedSpec))
	g.Expect(specErr).Should(gomega.BeNil())

	// OpenAPI accepts valid input
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
