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
	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"
	"net/http"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/webhook/third_party"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var log = logf.Log.WithName(constants.PodMutatorWebhookName)

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client  client.Client
	Decoder types.Decoder
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req types.Request) types.Response {
	pod := &v1.Pod{}

	if err := mutator.Decoder.Decode(req, pod); err != nil {
		log.Error(err, "Failed to decode pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	if !needMutate(pod) {
		return admission.ValidationResponse(true, "")
	}

	configMap := &v1.ConfigMap{}
	err := mutator.Client.Get(context.TODO(), k8types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	// For some reason pod namespace is always empty when coming to pod mutator, need to set from admission request
	pod.Namespace = req.AdmissionRequest.Namespace

	if err := mutator.mutate(pod, configMap); err != nil {
		log.Error(err, "Failed to mutate pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(pod)
	if err != nil {
		log.Error(err, "Failed to marshal pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return third_party.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

func (mutator *Mutator) mutate(pod *v1.Pod, configMap *v1.ConfigMap) error {
	credentialBuilder := credentials.NewCredentialBulder(mutator.Client, configMap)

	storageInitializerConfig, err := getStorageInitializerConfigs(configMap)
	if err != nil {
		return err
	}

	storageInitializer := &StorageInitializerInjector{
		credentialBuilder: credentialBuilder,
		config:            storageInitializerConfig,
	}

	loggerConfig, err := getLoggerConfigs(configMap)
	if err != nil {
		return err
	}

	loggerInjector := &LoggerInjector{
		config: loggerConfig,
	}

	mutators := []func(pod *v1.Pod) error{
		InjectGKEAcceleratorSelector,
		storageInitializer.InjectStorageInitializer,
		loggerInjector.InjectLogger,
	}

	for _, mutator := range mutators {
		if err := mutator(pod); err != nil {
			return err
		}
	}

	return nil
}

func needMutate(pod *v1.Pod) bool {
	// Skip webhook if pod not managed by kfserving
	_, ok := pod.Labels[constants.InferenceServicePodLabelKey]
	return ok
}
