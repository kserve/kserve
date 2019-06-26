package main

import (
	"flag"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/tools/tf2openapi/generator"
	pb "github.com/kubeflow/kfserving/tools/tf2openapi/generated/protobuf"
	"io/ioutil"
	"log"
	"os"
)

var (
	model = flag.String("model", "", "Absolute path of SavedModel file")
	out   = flag.String("out", "std", "Either 'std' to display on standard out or 'file' to save output")
	// outFile not required if output option is std
	outFile = flag.String("outFile", "", "Absolute path of file to write OpenAPI spec to")
)

/** Convert SavedModel to OpenAPI. Note that the SavedModel must have at least one signature defined**/
func main() {
	flag.Parse()
	if *out == "file" && *outFile == "" {
		log.Fatalln("Please specify output file name using the 'outFile' flag")
	}
	modelPb, err := ioutil.ReadFile(*model)
	if err != nil {
		log.Fatalln("Error reading file \n" + err.Error())
	}

	/** Convert Go struct to inner model */
	model := UnmarshalSavedModelPb(modelPb)

	/** Schema generation example **/
	spec := generator.GenerateOpenAPI(model)
	if *out == "file" {
		f, err := os.Create(*outFile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if _, err = f.WriteString(spec); err != nil {
			panic(err)
		}
	} else {
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
