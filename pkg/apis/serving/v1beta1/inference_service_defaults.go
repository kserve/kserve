/*
Copyright 2020 kubeflow.org.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
		log.Error(err, "unable to create api client")
		return
	}
	configMap, err := GetInferenceServicesConfig(cli)
	if err != nil {
		panic(err)
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
