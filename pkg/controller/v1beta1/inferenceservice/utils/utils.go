package utils

import v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"

// Only enable MMS predictor for sklearn and xgboost model server
// TODO should read the InferenceService configmap to decide if MMS should be enabled for this predictor
func IsMMSPredictor(predictor *v1beta1api.PredictorSpec) bool {
	if (predictor.SKLearn != nil || predictor.XGBoost != nil || predictor.Triton != nil) && predictor.GetImplementation().GetStorageUri() == nil {
		return true
	}
	return false
}
