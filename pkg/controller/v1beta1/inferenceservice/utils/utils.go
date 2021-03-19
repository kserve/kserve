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

package utils

import (
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("inferenceservice-v1beta1--utils")
)

// Only enable MMS predictor when predictor config sets MMS to true and storage uri is not set
func IsMMSPredictor(isvc *v1beta1api.InferenceService, isvcConfig *v1beta1api.InferenceServicesConfig) bool {
	predictor := isvc.Spec.Predictor
	isMMS := predictor.GetImplementation().IsMMS(isvcConfig) && predictor.GetImplementation().GetStorageUri() == nil

	if isMMS {
		log.V(5).Info("Predictor is configured for multi-model serving", "InferenceService", isvc.Name)
	} else {
		log.V(5).Info("Predictor is configured for single model serving", "InferenceService", isvc.Name)
	}

	return isMMS
}
