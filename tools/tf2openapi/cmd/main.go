package main

import (
	"flag"
	"github.com/golang/protobuf/proto"
	"github.com/kfserving/tools/tf2openapi/generator"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
	"io/ioutil"
	"log"
)

var model = flag.String("model", "", "Absolute path of SavedModel file")

func main() {
	flag.Parse()

	in, err := ioutil.ReadFile(*model)
	if err != nil {
		log.Print(*model)
		log.Fatalln("Error reading file:", err)
	}

	/** Convert Go struct to inner model */
	model := UnmarshalSavedModelPb(in)

	/** Schema generation example **/
	log.Println(generator.GenerateOpenAPI(model))
}

/**
Raises errors when model is missing fields that would pose an issue for Schema generation
 */
func UnmarshalSavedModelPb(in []byte) pb.SavedModel {
	model := &pb.SavedModel{}
	if err := proto.Unmarshal(in, model); err != nil {
		panic("SavedModel not in expected format. May be corrupted.")
	}
	return *model
}
