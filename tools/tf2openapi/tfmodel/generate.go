package tfmodel

import (
	"github.com/golang/protobuf/proto"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
	"log"
)

func UnmarshalSavedModelPb(in []byte) pb.SavedModel {
	model := &pb.SavedModel{}
	if err := proto.Unmarshal(in, model); err != nil {
		log.Fatalln("SavedModel not in expected format. May be corrupted.")
	}
	return *model
}

func generateTFSigDef(sigDefMapping map[string]*pb.TensorInfo, sigDefArr *[]*TFTensor) {
	for key := range sigDefMapping {
		tfTensor := &TFTensor{}
		tfTensor.SetKey(key)
		tfTensor.SetShape(append(ShapeInternal{}, 1))
		tfTensor.SetDType(DTypeInternal(2))
		//TODO *tensorPtr.Dtype
		//TODO *tensorPtr.TensorShape.Dim
		//> unknown rank
		//> otherwise
		*sigDefArr = append(*sigDefArr, tfTensor)
	}
}

func GenerateTFModel(model pb.SavedModel) *TFSavedModel {
	tfSavedModel := &TFSavedModel{}
	tfMetaGraphs := &[]*TFMetaGraph{}
	for _, mPtr := range model.MetaGraphs {
		metaGraph := *mPtr
		tfMetaGraph := &TFMetaGraph{}
		tfSigDefs := &[]*TFSignatureDef{}
		for sigDefKey, sigDefPtr := range metaGraph.SignatureDef {
			tfSigDef := &TFSignatureDef{}
			tfSigDef.SetName(sigDefKey)
			tfSigDefInputs := &[]*TFTensor{}
			tfSigDefOutputs := &[]*TFTensor{}
			sigDef := *sigDefPtr
			generateTFSigDef(sigDef.Inputs, tfSigDefInputs)
			generateTFSigDef(sigDef.Outputs, tfSigDefOutputs)
			tfSigDef.SetInputs(*tfSigDefInputs)
			tfSigDef.SetOutputs(*tfSigDefOutputs)
			*tfSigDefs = append(*tfSigDefs, tfSigDef)
		}
		tfMetaGraph.SetSignatureDefs(*tfSigDefs)
		*tfMetaGraphs = append(*tfMetaGraphs, tfMetaGraph)
	}
	tfSavedModel.SetMetaGraphs(*tfMetaGraphs)
	return tfSavedModel
}
