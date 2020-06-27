package v1beta1

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = logf.Log.WithName("kfserving-v1beta1-defaulter")

func (isvc *InferenceService) Default(client client.Client) {
	logger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	/*configMap, err := GetInferenceServicesConfig(client)
	if err != nil {
		logger.Error(err, "Failed to get configmap")
		return
	}
	isvc.Spec.Predictor.ApplyDefaults(configMap)

	if isvc.Spec.Transformer != nil {
		isvc.Spec.Transformer.ApplyDefaults(configMap)
	}

	if isvc.Spec.Explainer != nil {
		isvc.Spec.Explainer.ApplyDefaults(configMap)
	}*/
}
