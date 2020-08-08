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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = logf.Log.WithName("kfserving-v1beta1-defaulter")

var (
	defaultResource = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("1"),
		v1.ResourceMemory: resource.MustParse("2Gi"),
	}
	// logger for the mutating webhook.
	mutatorLogger = logf.Log.WithName("inferenceservice-v1beta1-mutating-webhook")
)

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}
	for k, v := range defaultResource {
		if _, ok := requirements.Requests[k]; !ok {
			requirements.Requests[k] = v
		}
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}
	for k, v := range defaultResource {
		if _, ok := requirements.Limits[k]; !ok {
			requirements.Limits[k] = v
		}
	}
}

func (isvc *InferenceService) Default() {
	logger.Info("Defaulting InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	cli, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		mutatorLogger.Error(err, "unable to create api client")
		panic("unable to create api client")
	}
	configMap, err := GetInferenceServicesConfig(cli)
	if err != nil {
		panic(err)
	}
	if predictor, err := isvc.Spec.Predictor.GetPredictor(); err == nil {
		predictor.Default(configMap)
	}

	if isvc.Spec.Transformer != nil {
		if transformer, err := isvc.Spec.Transformer.GetTransformer(); err == nil {
			transformer.Default()
		}
	}

	if isvc.Spec.Explainer != nil {
		if explainer, err := isvc.Spec.Explainer.GetExplainer(); err == nil {
			explainer.Default(configMap)
		}
	}
}
