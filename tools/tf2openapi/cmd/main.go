package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/golang/protobuf/proto"
	pb "github.com/kserve/kserve/tools/tf2openapi/generated/protobuf"
	"github.com/kserve/kserve/tools/tf2openapi/generator"
	"github.com/spf13/cobra"
)

// Known error messages
const (
	ModelBasePathError    = "Error reading file %s \n%s"
	OutputFilePathError   = "Failed writing to %s: %s"
	SavedModelFormatError = "SavedModel not in expected format. May be corrupted: "
)

var (
	modelBasePath string
	modelName     string
	modelVersion  string
	sigDefKey     string
	metaGraphTags []string
	outFile       string
)

/** Convert SavedModel to OpenAPI. Note that the SavedModel must have at least one signature defined**/
func main() {
	rootCmd := &cobra.Command{
		Use:   "tf2openapi",
		Short: "tf2openapi is an OpenAPI generator for TensorFlow SavedModels",
		Long: `This tool takes TensorFlow SavedModel files as inputs and generates OpenAPI 3.0 
			specifications for HTTP prediction requests. The SavedModel files must 
			contain signature definitions (SignatureDefs) for models.`,
		Run: viewAPI,
	}

	rootCmd.Flags().StringVarP(&modelBasePath, "model_base_path", "m", "", "Absolute path of SavedModel file")
	rootCmd.MarkFlagRequired("model_base_path")
	rootCmd.Flags().StringVarP(&modelName, "name", "n", "model", "Name of model")
	rootCmd.Flags().StringVarP(&modelVersion, "version", "v", "1", "Model version")
	rootCmd.Flags().StringVarP(&outFile, "output_file", "o", "", "Absolute path of file to write OpenAPI spec to")
	rootCmd.Flags().StringVarP(&sigDefKey, "signature_def", "s", "", "Serving SignatureDef Key")
	rootCmd.Flags().StringSliceVarP(&metaGraphTags, "metagraph_tags", "t", []string{}, "All tags identifying desired MetaGraph")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err.Error())
	}
}

func viewAPI(cmd *cobra.Command, args []string) {
	modelPb, err := ioutil.ReadFile(modelBasePath)
	if err != nil {
		log.Fatalf(ModelBasePathError, modelBasePath, err.Error())
	}
	generatorBuilder := &generator.Builder{}
	if modelName != "" {
		generatorBuilder.SetName(modelName)
	}
	if modelVersion != "" {
		generatorBuilder.SetVersion(modelVersion)
	}
	if sigDefKey != "" {
		generatorBuilder.SetSigDefKey(sigDefKey)
	}
	if len(metaGraphTags) != 0 {
		generatorBuilder.SetMetaGraphTags(metaGraphTags)
	}

	model := unmarshalSavedModelPb(modelPb)
	gen := generatorBuilder.Build()
	spec, err := gen.GenerateOpenAPI(model)
	if err != nil {
		log.Fatalln(err.Error())
	}
	if outFile != "" {
		f, err := os.Create(outFile)
		if err != nil {
			log.Fatalf(OutputFilePathError, outFile, err)
		}
		defer f.Close()
		if _, err = f.WriteString(spec); err != nil {
			panic(err)
		}
	} else {
		// Default to std::out
		fmt.Println(spec)
	}
}

/**
Raises errors when model is missing fields that would pose an issue for Schema generation
*/
func unmarshalSavedModelPb(modelPb []byte) *pb.SavedModel {
	model := &pb.SavedModel{}
	if err := proto.Unmarshal(modelPb, model); err != nil {
		log.Fatalln(SavedModelFormatError + err.Error())
	}
	return model
}
