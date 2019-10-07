/*
Copyright 2019 kubeflow.org.

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

package v1alpha2

import "sigs.k8s.io/controller-runtime/pkg/client"

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter
func (kfsvc *KFService) Default(client client.Client) {
	logger.Info("Defaulting KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	if err := kfsvc.applyDefaultsEndpoint(&kfsvc.Spec.Default, client); err != nil {
		logger.Error(err, "Failed to apply defaults for default endpoints")
	}
	if err := kfsvc.applyDefaultsEndpoint(kfsvc.Spec.Canary, client); err != nil {
		logger.Error(err, "Failed to apply defaults for canary endpoints")
	}
}

func (kfsvc *KFService) applyDefaultsEndpoint(endpoint *EndpointSpec, client client.Client) error {
	if endpoint != nil {
		configMap, err := GetKFServiceConfigMap(client)
		if err != nil {
			return err
		}
		predictorConfig, err := GetPredictorConfigs(configMap)
		if err != nil {
			return err
		}
		endpoint.Predictor.ApplyDefaults(predictorConfig)

		if endpoint.Transformer != nil {
			transformerConfig, err := GetTransformerConfigs(configMap)
			if err != nil {
				return err
			}
			endpoint.Transformer.ApplyDefaults(transformerConfig)
		}

		if endpoint.Explainer != nil {
			explainersConfig, err := GetExplainerConfigs(configMap)
			if err != nil {
				return err
			}
			endpoint.Explainer.ApplyDefaults(explainersConfig)
		}
	}
	return nil
}
