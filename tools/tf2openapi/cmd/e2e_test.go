package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/onsi/gomega"
	"github.com/kubeflow/kfserving/tools/tf2openapi/types"
)

func TestFlowers(t *testing.T) {
	// model src: gs://kfserving-samples/models/tensorflow/flowers
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", wd + "/testdata/" + t.Name() + ".pb"}
	cmd := exec.Command(cmdName, cmdArgs...)
	spec, err := cmd.Output()
	g.Expect(err).Should(gomega.BeNil())
	acceptableSpec := readFile("TestFlowers.golden", t)
	acceptableSpecPermuted := readFile("TestFlowers2.golden", t)
	g.Expect(spec).Should(gomega.Or(gomega.MatchJSON(acceptableSpec), gomega.MatchJSON(acceptableSpecPermuted)))
}

func TestCensus(t *testing.T) {
	// estimator model src: https://github.com/GoogleCloudPlatform/cloudml-samples/tree/master/census
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", wd + "/testdata/" + t.Name() + ".pb", "--signature_def", "predict"}
	cmd := exec.Command(cmdName, cmdArgs...)
	spec, err := cmd.Output()
	g.Expect(err).Should(gomega.BeNil())
	swagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	expectedSpec := readFile("TestCensus.golden", t)
	expectedSwagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal(expectedSpec, &expectedSwagger)).To(gomega.Succeed())

	// test equality, ignoring order in JSON arrays
	instances := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["instances"].Value
	expectedInstances := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["instances"].Value
	g.Expect(swagger.Paths).Should(gomega.Equal(expectedSwagger.Paths))
	g.Expect(swagger.OpenAPI).Should(gomega.Equal(expectedSwagger.OpenAPI))
	g.Expect(swagger.Info).Should(gomega.Equal(expectedSwagger.Info))
	g.Expect(swagger.Components.Responses).Should(gomega.Equal(expectedSwagger.Components.Responses))
	g.Expect(instances.Items.Value.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(instances.Items.Value.Required).To(gomega.ConsistOf(expectedInstances.Items.Value.Required))
	g.Expect(instances.Items.Value.AdditionalPropertiesAllowed).Should(gomega.Not(gomega.BeNil()))
	g.Expect(instances.Items.Value.AdditionalPropertiesAllowed).Should(gomega.Equal(expectedInstances.Items.Value.AdditionalPropertiesAllowed))
	g.Expect(instances.Items.Value.Properties).Should(gomega.Equal(expectedInstances.Items.Value.Properties))
}

func TestInvalidFilePath(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", "badPath"}
	cmd := exec.Command(cmdName, cmdArgs...)
	output, err := cmd.CombinedOutput()
	g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(1))
	expectedErr := fmt.Sprintf(ModelBasePathError, "badPath", "")
	g.Expect(output).Should(gomega.ContainSubstring(expectedErr))
}

func TestCustomFlags(t *testing.T) {
	// estimator model src: https://github.com/GoogleCloudPlatform/cloudml-samples/tree/master/census
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", wd + "/testdata/" + t.Name() + ".pb",
		"--signature_def", "predict", "--name", "customName", "--version", "1000",
		"--metagraph_tags", "serve"}
	cmd := exec.Command(cmdName, cmdArgs...)
	spec, err := cmd.Output()
	g.Expect(err).Should(gomega.BeNil())
	swagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	expectedSpec := readFile("TestCustomFlags.golden", t)
	expectedSwagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal(expectedSpec, &expectedSwagger)).To(gomega.Succeed())

	// test equality, ignoring order in JSON arrays
	instances := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["instances"].Value
	expectedInstances := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["instances"].Value
	g.Expect(swagger.Paths).Should(gomega.Equal(expectedSwagger.Paths))
	g.Expect(swagger.OpenAPI).Should(gomega.Equal(expectedSwagger.OpenAPI))
	g.Expect(swagger.Info).Should(gomega.Equal(expectedSwagger.Info))
	g.Expect(swagger.Components.Responses).Should(gomega.Equal(expectedSwagger.Components.Responses))
	g.Expect(instances.Items.Value.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(instances.Items.Value.Required).To(gomega.ConsistOf(expectedInstances.Items.Value.Required))
	g.Expect(instances.Items.Value.AdditionalPropertiesAllowed).Should(gomega.Not(gomega.BeNil()))
	g.Expect(instances.Items.Value.AdditionalPropertiesAllowed).Should(gomega.Equal(expectedInstances.Items.Value.AdditionalPropertiesAllowed))
	g.Expect(instances.Items.Value.Properties).Should(gomega.Equal(expectedInstances.Items.Value.Properties))
}

func TestInvalidCommand(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--bad_flag"}
	cmd := exec.Command(cmdName, cmdArgs...)
	stdErr, err := cmd.CombinedOutput()
	g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(1))
	g.Expect(stdErr).Should(gomega.And(gomega.ContainSubstring("Usage"), gomega.ContainSubstring("Flags:")))
}

func TestInvalidSavedModel(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", wd + "/testdata/" + t.Name() + ".pb"}
	cmd := exec.Command(cmdName, cmdArgs...)
	stdErr, err := cmd.CombinedOutput()
	g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(1))
	g.Expect(stdErr).Should(gomega.ContainSubstring(SavedModelFormatError))
}

func TestPropagateOpenAPIGenerationError(t *testing.T) {
	// model src: https://github.com/tensorflow/serving/tree/master/tensorflow_serving/example
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	cmdArgs := []string{"--model_base_path", wd + "/testdata/" + t.Name() + ".pb",
		"--signature_def", "serving_default"}
	cmd := exec.Command(cmdName, cmdArgs...)
	stdErr, err := cmd.CombinedOutput()
	g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(1))
	g.Expect(stdErr).Should(gomega.ContainSubstring(types.UnsupportedAPISchemaError))
}

func TestOutputToFile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	defer os.Remove(wd + "/testdata/" + t.Name() + ".json")
	outputFileName := t.Name() + ".json"
	outputFilePath := wd + "/testdata/" + outputFileName
	cmdArgs := []string{"--model_base_path", wd + "/testdata/TestFlowers.pb",
		"--output_file", outputFilePath}
	cmd := exec.Command(cmdName, cmdArgs...)
	_, err := cmd.CombinedOutput()
	g.Expect(err).Should(gomega.BeNil())
	spec := readFile(outputFileName, t)
	acceptableSpec := readFile("TestFlowers.golden", t)
	acceptableSpecPermuted := readFile("TestFlowers2.golden", t)
	g.Expect(spec).Should(gomega.Or(gomega.MatchJSON(acceptableSpec), gomega.MatchJSON(acceptableSpecPermuted)))
}

func TestOutputToFileTargetDirectoryError(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	defer os.Remove(wd + "/testdata/" + t.Name() + ".json")
	outputFileName := t.Name() + ".json"
	badOutputFilePath := wd + "/nonexistent/" + outputFileName
	cmdArgs := []string{"--model_base_path", wd + "/testdata/TestFlowers.pb",
		"--output_file", badOutputFilePath}
	cmd := exec.Command(cmdName, cmdArgs...)
	stdErr, err := cmd.CombinedOutput()
	g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(1))
	expectedErr := fmt.Sprintf(OutputFilePathError, badOutputFilePath, "")
	g.Expect(stdErr).Should(gomega.ContainSubstring(expectedErr))
}

func readFile(fName string, t *testing.T) []byte {
	fPath := filepath.Join("testdata", fName)
	openAPI, err := ioutil.ReadFile(fPath)
	if err != nil {
		t.Fatalf("failed reading %s: %s", fPath, err)
	}
	return openAPI
}

func workingDir(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed os.Getwd() = %v, %v", wd, err)
	}
	return wd
}

func cmd(wd string) string {
	return filepath.Dir(wd) + "/bin/tf2openapi"
}
