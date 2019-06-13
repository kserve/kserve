package tfmodel

import (
	"github.com/golang/protobuf/proto"
	pb "github.com/tensorflow/tensorflow/tensorflow/go/core/protobuf"
	"log"
	"strings"
)

/**
Raises errors when model is missing fields that would pose an issue for OpenAPI generation
 */

const servingMetaGraphTag string = "serve"
const b64Indicator string = "_bytes"
const predictReqSigDefMethod string = "tensorflow/serving/predict"

func UnmarshalSavedModelPb(in []byte) pb.SavedModel {
	model := &pb.SavedModel{}
	if err := proto.Unmarshal(in, model); err != nil {
		log.Fatalln("SavedModel not in expected format. May be corrupted.")
	}
	return *model
}

func tagInList(tag string, tagList []string) (bool) {
	for _, t := range tagList {
		if t == tag {
			return true
		}
	}
	return false
}

func GenerateTFModel(model pb.SavedModel) TFSavedModel {
	tfSavedModel := TFSavedModel{}
	tfMetaGraphs := []TFMetaGraph{}
	for _, mPtr := range model.MetaGraphs {
		metaGraph := *mPtr
		if tagInList(servingMetaGraphTag, metaGraph.MetaInfoDef.Tags) {
			tfMetaGraph := TFMetaGraph{}
			tfSigDefs := []TFSignatureDef{}
			for sigDefKey, sigDefPtr := range metaGraph.SignatureDef {

				if sigDefPtr.MethodName == predictReqSigDefMethod {
					tfSigDef := TFSignatureDef{}
					tfSigDef.Name = sigDefKey
					sigDef := *sigDefPtr
					tfSigDefInputs := generateTFSigDef(sigDef.Inputs)
					tfSigDefOutputs := generateTFSigDef(sigDef.Outputs)
					tfSigDef.Inputs = tfSigDefInputs
					tfSigDef.Outputs = tfSigDefOutputs
					tfSigDefs = append(tfSigDefs, tfSigDef)
				}

			}
			tfMetaGraph.SignatureDefs = tfSigDefs
			tfMetaGraphs = append(tfMetaGraphs, tfMetaGraph)
		}
	}
	tfSavedModel.MetaGraphs = tfMetaGraphs
	if len(tfSavedModel.MetaGraphs) == 0 {
		panic("No graph to serve from SavedModel.")
	}
	return tfSavedModel
}

func generateTFSigDef(sigDefMapping map[string]*pb.TensorInfo) []TFTensor {
	sigDefArr := []TFTensor{}
	for key, tensorInfo := range sigDefMapping {
		tfTensor := TFTensor{}
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
		tfTensor.DType = generateTFDType(tensorInfo.Dtype.String(), tfTensor)
		sigDefArr = append(sigDefArr, tfTensor)
	}
	return sigDefArr
}

func generateTFDType(tensorInfoDType string, tfTensor TFTensor) TFDType{
	var dType TFDType
	switch tensorInfoDType {
	case "DT_BOOL":
		dType = DT_BOOL
	case "DT_STRING":
		if strings.HasSuffix(tfTensor.Key, b64Indicator) {
			dType = DT_B64_STRING
		} else {
			dType = DT_STRING
		}
	case "DT_INT8":
		dType = DT_INT8
	case "DT_UINT8":
		dType = DT_UINT8
	case "DT_INT16":
		dType = DT_INT16
	case "DT_INT32":
		dType = DT_INT32
	case "DT_UINT32":
		dType = DT_UINT32
	case "DT_INT64":
		dType = DT_INT64
	case "DT_UINT64":
		dType = DT_UINT64
	case "DT_FLOAT":
		dType = DT_FLOAT
	case "DT_DOUBLE":
		dType = DT_DOUBLE
	default:
		panic("Unsupported data type for generating payloads")
	}
	return dType
}
