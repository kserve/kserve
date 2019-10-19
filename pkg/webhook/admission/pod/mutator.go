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
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/webhook/third_party"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client  client.Client
	Decoder types.Decoder
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req types.Request) types.Response {
	pod := &v1.Pod{}

	if err := mutator.Decoder.Decode(req, pod); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	configMap := &v1.ConfigMap{}
	err := mutator.Client.Get(context.TODO(), k8types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		klog.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	// For some reason pod namespace is always empty when coming to pod mutator, need to set from admission request
	pod.Namespace = req.AdmissionRequest.Namespace

	if err := mutator.mutate(pod, configMap); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return third_party.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

func (mutator *Mutator) mutate(pod *v1.Pod, configMap *v1.ConfigMap) error {

	credentialBuilder := credentials.NewCredentialBulder(mutator.Client, configMap)
	var storageInitializerConfig StorageInitializerConfig
	if initializerConfig, ok := configMap.Data[StorageInitializerConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(initializerConfig), &storageInitializerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall storage initializer json string due to %v ", err))
		}
	}
	storageInitializer := &StorageInitializerInjector{
		credentialBuilder: credentialBuilder,
		config:            &storageInitializerConfig,
	}

	var inferenceLoggerConfig InferenceLoggerConfig
	if loggerConfig, ok := configMap.Data[InferenceLoggerConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(loggerConfig), &inferenceLoggerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall inference inference-logger json string due to %v ", err))
		}
	}

	inferenceLoggerInjector := &InferenceLoggerInjector{
		config: &inferenceLoggerConfig,
	}

	mutators := []func(pod *v1.Pod) error{
		InjectGKEAcceleratorSelector,
		storageInitializer.InjectStorageInitializer,
		inferenceLoggerInjector.InjectInferenceLogger,
	}

	for _, mutator := range mutators {
		if err := mutator(pod); err != nil {
			return err
		}
	}

	return nil
}
