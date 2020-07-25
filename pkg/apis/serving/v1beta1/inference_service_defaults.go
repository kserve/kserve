package v1beta1

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = logf.Log.WithName("kfserving-v1beta1-defaulter")

func (isvc *InferenceService) Default() {
	logger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	cli, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		log.Error(err, "unable to create apiReader")
		return
	}
	configMap, err := GetInferenceServicesConfig(cli)
	if err != nil {
		logger.Error(err, "Failed to get configmap")
		return
	}
	if predictor, err := isvc.GetPredictor(); err == nil {
		predictor.Default(configMap)
	}

	/*isvc.Spec.Predictor.ApplyDefaults(configMap)

	if isvc.Spec.Transformer != nil {
		isvc.Spec.Transformer.ApplyDefaults(configMap)
	}

	if isvc.Spec.Explainer != nil {
		isvc.Spec.Explainer.ApplyDefaults(configMap)
	}*/
}
