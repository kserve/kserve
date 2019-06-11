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

func GenerateTFModel(model pb.SavedModel) *TFSavedModel {
	tfSavedModel := &TFSavedModel{}
	tfMetaGraphs := &[]*TFMetaGraph{}
	for _, mPtr := range model.MetaGraphs {
		metaGraph := *mPtr
		tfMetaGraph := &TFMetaGraph{}
		tfSigDefs := &[]*TFSignatureDef{}
		for sigDefKey, sigDefPtr := range metaGraph.SignatureDef {
			tfSigDef := &TFSignatureDef{}
			tfSigDef.Name = sigDefKey
			tfSigDefInputs := &[]*TFTensor{}
			tfSigDefOutputs := &[]*TFTensor{}
			sigDef := *sigDefPtr
			generateTFSigDef(sigDef.Inputs, tfSigDefInputs)
			generateTFSigDef(sigDef.Outputs, tfSigDefOutputs)
			tfSigDef.Inputs = *tfSigDefInputs
			tfSigDef.Outputs = *tfSigDefOutputs
			*tfSigDefs = append(*tfSigDefs, tfSigDef)
		}
		tfMetaGraph.SignatureDefs = *tfSigDefs
		*tfMetaGraphs = append(*tfMetaGraphs, tfMetaGraph)
	}
	tfSavedModel.MetaGraphs = *tfMetaGraphs
	return tfSavedModel
}

func generateTFSigDef(sigDefMapping map[string]*pb.TensorInfo, sigDefArr *[]*TFTensor) {
	for key, tensorInfo := range sigDefMapping {
		tfTensor := &TFTensor{}
		tfTensor.Key = key
		tfShape := TFShape{}
		if tensorInfo.TensorShape.UnknownRank {
			tfTensor.Rank = -1
		} else {
			for _, d := range tensorInfo.TensorShape.Dim {
				tfShape = append(tfShape, d.Size)
			}
			tfTensor.Rank = int64(len(tfShape))
		}
		tfTensor.Shape = tfShape
		generateTFDType(tensorInfo.Dtype.String(), tfTensor)
		*sigDefArr = append(*sigDefArr, tfTensor)
	}
}

func generateTFDType(tensorInfoDType string, tfTensor *TFTensor) {
	switch tensorInfoDType {
	case "DT_BOOL":
		tfTensor.DType = DT_BOOL
	case "DT_STRING":
		tfTensor.DType = DT_STRING
	case "DT_INT8":
		tfTensor.DType = DT_INT8
	case "DT_UINT8":
		tfTensor.DType = DT_UINT8
	case "DT_INT16":
		tfTensor.DType = DT_INT16
	case "DT_INT32":
		tfTensor.DType = DT_INT32
	case "DT_UINT32":
		tfTensor.DType = DT_UINT32
	case "DT_INT64":
		tfTensor.DType = DT_INT64
	case "DT_UINT64":
		tfTensor.DType = DT_UINT64
	case "DT_FLOAT":
		tfTensor.DType = DT_FLOAT
	case "DT_DOUBLE":
		tfTensor.DType = DT_DOUBLE
	default:
		panic("Unsupported data type for generating payloads")
	}
}
