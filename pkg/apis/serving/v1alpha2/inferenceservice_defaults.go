/*

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

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter

var _ webhook.Defaulter = &InferenceService{}

func (isvc *InferenceService) Default() {
	logger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		panic("Failed to create client in defauler")
	}
	if err := isvc.applyDefaultsEndpoint(&isvc.Spec.Default, client); err != nil {
		logger.Error(err, "Failed to apply defaults for default endpoints")
	}
	if err := isvc.applyDefaultsEndpoint(isvc.Spec.Canary, client); err != nil {
		logger.Error(err, "Failed to apply defaults for canary endpoints")
	}
}

func (isvc *InferenceService) applyDefaultsEndpoint(endpoint *EndpointSpec, client client.Client) error {
	if endpoint != nil {
		configMap, err := GetInferenceServicesConfig(client)
		if err != nil {
			return err
		}
		endpoint.Predictor.ApplyDefaults(configMap)

		if endpoint.Transformer != nil {
			endpoint.Transformer.ApplyDefaults(configMap)
		}

		if endpoint.Explainer != nil {
			endpoint.Explainer.ApplyDefaults(configMap)
		}
	}
	return nil
}
