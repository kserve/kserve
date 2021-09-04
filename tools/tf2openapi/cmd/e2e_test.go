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
	gomegaTypes "github.com/onsi/gomega/types"

	"github.com/kserve/kserve/tools/tf2openapi/types"
)

// Functional E2E example
func TestFunctionalDifferentFlags(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	scenarios := map[string]struct {
		cmdArgs      []string
		expectedSpec []byte
	}{
		"Flowers": {
			cmdArgs:      []string{"--model_base_path", wd + "/testdata/TestFlowers.pb"},
			expectedSpec: readFile("TestFlowers.golden.json", t),
		},
		// estimator model src: https://github.com/GoogleCloudPlatform/cloudml-samples/tree/master/census
		"Census": {
			cmdArgs:      []string{"--model_base_path", wd + "/testdata/TestCensus.pb", "--signature_def", "predict"},
			expectedSpec: readFile("TestCensus.golden.json", t),
		},
		"CustomFlags": {
			cmdArgs: []string{"--model_base_path", wd + "/testdata/TestCensus.pb",
				"--signature_def", "predict", "--name", "customName", "--version", "1000",
				"--metagraph_tags", "serve"},
			expectedSpec: readFile("TestCustomFlags.golden.json", t),
		},
	}
	for name, scenario := range scenarios {
		t.Logf("Running %s ...", name)
		cmd := exec.Command(cmdName, scenario.cmdArgs...)
		spec, err := cmd.Output()
		g.Expect(err).Should(gomega.BeNil())
		swagger := &openapi3.Swagger{}
		g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

		expectedSwagger := &openapi3.Swagger{}
		g.Expect(json.Unmarshal(scenario.expectedSpec, &expectedSwagger)).To(gomega.Succeed())

		// test equality, ignoring order in JSON arrays
		expectSwaggerEquality(swagger, expectedSwagger, g)
	}
}

func TestInputErrors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wd := workingDir(t)
	cmdName := cmd(wd)
	scenarios := map[string]struct {
		cmdArgs             []string
		matchExpectedStdErr gomegaTypes.GomegaMatcher
		expectedExitCode    int
	}{
		"InvalidCommand": {
			cmdArgs:             []string{"--bad_flag"},
			matchExpectedStdErr: gomega.And(gomega.ContainSubstring("Usage"), gomega.ContainSubstring("Flags:")),
			expectedExitCode:    1,
		},
		"InvalidSavedModel": {
			cmdArgs:             []string{"--model_base_path", wd + "/testdata/TestInvalidSavedModel.pb"},
			matchExpectedStdErr: gomega.ContainSubstring(SavedModelFormatError),
			expectedExitCode:    1,
		},
		"PropagateOpenAPIGenerationError": {
			// model src: https://github.com/tensorflow/serving/tree/master/tensorflow_serving/example
			cmdArgs: []string{"--model_base_path", wd + "/testdata/TestPropagateOpenAPIGenerationError.pb",
				"--signature_def", "serving_default"},
			matchExpectedStdErr: gomega.ContainSubstring(types.UnsupportedAPISchemaError),
			expectedExitCode:    1,
		},
		"InvalidFilePath": {
			cmdArgs:             []string{"--model_base_path", "badPath"},
			matchExpectedStdErr: gomega.ContainSubstring(fmt.Sprintf(ModelBasePathError, "badPath", "")),
			expectedExitCode:    1,
		},
	}
	for name, scenario := range scenarios {
		t.Logf("Running %s ...", name)
		cmd := exec.Command(cmdName, scenario.cmdArgs...)
		stdErr, err := cmd.CombinedOutput()
		g.Expect(stdErr).Should(scenario.matchExpectedStdErr)
		g.Expect(err.(*exec.ExitError).ExitCode()).To(gomega.Equal(scenario.expectedExitCode))
	}
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
	stdErr, err := cmd.CombinedOutput()
	g.Expect(stdErr).To(gomega.BeEmpty())
	g.Expect(err).Should(gomega.BeNil())
	spec := readFile(outputFileName, t)
	swagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal([]byte(spec), &swagger)).To(gomega.Succeed())

	expectedSpec := readFile("TestFlowers.golden.json", t)
	expectedSwagger := &openapi3.Swagger{}
	g.Expect(json.Unmarshal(expectedSpec, &expectedSwagger)).To(gomega.Succeed())

	// test equality, ignoring order in JSON arrays
	expectSwaggerEquality(swagger, expectedSwagger, g)
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

func expectSwaggerEquality(swagger *openapi3.Swagger, expectedSwagger *openapi3.Swagger, g *gomega.GomegaWithT) {
	g.Expect(swagger.Paths).Should(gomega.Equal(expectedSwagger.Paths))
	g.Expect(swagger.OpenAPI).Should(gomega.Equal(expectedSwagger.OpenAPI))
	g.Expect(swagger.Info).Should(gomega.Equal(expectedSwagger.Info))
	instances := swagger.Components.RequestBodies["modelInput"].Value.Content.Get("application/json").
		Schema.Value.Properties["instances"].Value.Items.Value
	expectedInstances := expectedSwagger.Components.RequestBodies["modelInput"].Value.Content.
		Get("application/json").Schema.Value.Properties["instances"].Value.Items.Value
	expectJsonEquality(instances, expectedInstances, g)

	predictions := swagger.Components.Responses["modelOutput"].Value.Content.Get("application/json").Schema.Value.Properties["predictions"].Value.Items.Value
	expectedPredictions := expectedSwagger.Components.Responses["modelOutput"].Value.Content.Get("application/json").Schema.Value.Properties["predictions"].Value.Items.Value
	expectJsonEquality(predictions, expectedPredictions, g)

}

func expectJsonEquality(actual *openapi3.Schema, expected *openapi3.Schema, g *gomega.GomegaWithT) {
	g.Expect(actual.Required).Should(gomega.Not(gomega.BeNil()))
	g.Expect(actual.Required).To(gomega.ConsistOf(expected.Required))
	g.Expect(actual.Properties).Should(gomega.Not(gomega.BeNil()))
	g.Expect(actual.Properties).Should(gomega.Equal(expected.Properties))
	g.Expect(actual.AdditionalPropertiesAllowed).Should(gomega.Equal(expected.AdditionalPropertiesAllowed))
}
