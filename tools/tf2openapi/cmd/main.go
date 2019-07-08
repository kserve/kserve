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
	model     = flag.String("model", "", "Absolute path of SavedModel file")
	outFile   = flag.String("output-file", "", "Absolute path of file to write OpenAPI spec to")
	sigDefKey = flag.String("signature-def", "", "Serving Signature Def Key ")
)

/** Convert SavedModel to OpenAPI. Note that the SavedModel must have at least one signature defined**/
func main() {
	flag.Parse()
	if *model == "" {
		log.Fatalln("Please specify the absolute path of the SavedModel file")
	}

	modelPb, err := ioutil.ReadFile(*model)
	if err != nil {
		log.Fatalf("Error reading file %s \n%s", *model, err.Error())
	}

	/** Convert Go struct to inner model */
	model := UnmarshalSavedModelPb(modelPb)

	/** Schema generation example **/
	spec, err := generator.GenerateOpenAPI(model, *sigDefKey)
	if err != nil {
		log.Fatalln(err.Error())
	}
	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if _, err = f.WriteString(spec); err != nil {
			panic(err)
		}
	} else {
		// default to std::out
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
