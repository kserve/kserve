package main

import (
	"github.com/golang/protobuf/proto"
	gen "github.com/kubeflow/kfserving/tools/tf2openapi/generator"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"os"
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
		log.Fatalf("Error reading file %s \n%s", modelBasePath, err.Error())
	}

	model := UnmarshalSavedModelPb(modelPb)
	generator := gen.NewGenerator()
	if modelName != "" {
		generator.WithName(modelName)
	}
	if modelVersion != "" {
		generator.WithVersion(modelVersion)
	}
	if sigDefKey != "" {
		generator.WithSigDefKey(sigDefKey)
	}
	if len(metaGraphTags) != 0 {
		generator.WithMetaGraphTags(metaGraphTags)
	}
	spec, err := generator.GenerateOpenAPI(model)
	if err != nil {
		log.Fatalln(err.Error())
	}
	if outFile != "" {
		f, err := os.Create(outFile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if _, err = f.WriteString(spec); err != nil {
			panic(err)
		}
	} else {
		// Default to std::out
		log.Println(spec)
	}
}

/**
Raises errors when model is missing fields that would pose an issue for Schema generation
 */
func UnmarshalSavedModelPb(modelPb []byte) *pb.SavedModel {
	model := &pb.SavedModel{}
	if err := proto.Unmarshal(modelPb, model); err != nil {
		log.Fatalln("SavedModel not in expected format. May be corrupted: " + err.Error())
	}
	return model
}
