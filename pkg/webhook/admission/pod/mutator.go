/*
Copyright 2021 The KServe Authors.

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
	"net/http"

	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-pods,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=inferenceservice.kserve-webhook-server.pod-mutator
var log = logf.Log.WithName(constants.PodMutatorWebhookName)

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &v1.Pod{}

	if err := mutator.Decoder.Decode(req, pod); err != nil {
		log.Error(err, "Failed to decode pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !needMutate(pod) {
		return admission.ValidationResponse(true, "")
	}

	configMap := &v1.ConfigMap{}
	err := mutator.Client.Get(context.TODO(), k8types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// For some reason pod namespace is always empty when coming to pod mutator, need to set from admission request
	pod.Namespace = req.AdmissionRequest.Namespace

	if err := mutator.mutate(pod, configMap); err != nil {
		log.Error(err, "Failed to mutate pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(pod)
	if err != nil {
		log.Error(err, "Failed to marshal pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
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

	batcherConfig, err := getBatcherConfigs(configMap)
	if err != nil {
		return err
	}

	agentConfig, err := getAgentConfigs(configMap)
	if err != nil {
		return err
	}

	agentInjector := &AgentInjector{
		credentialBuilder: credentialBuilder,
		agentConfig:       agentConfig,
		loggerConfig:      loggerConfig,
		batcherConfig:     batcherConfig,
	}

	mutators := []func(pod *v1.Pod) error{
		InjectGKEAcceleratorSelector,
		storageInitializer.InjectStorageInitializer,
		agentInjector.InjectAgent,
	}

	for _, mutator := range mutators {
		if err := mutator(pod); err != nil {
			return err
		}
	}

	return nil
}

func needMutate(pod *v1.Pod) bool {
	// Skip webhook if pod not managed by kserve
	_, ok := pod.Labels[constants.InferenceServicePodLabelKey]
	return ok
}

// InjectClient injects the client.
func (mutator *Mutator) InjectClient(c client.Client) error {
	mutator.Client = c
	return nil
}

// podAnnotator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (mutator *Mutator) InjectDecoder(d *admission.Decoder) error {
	mutator.Decoder = d
	return nil
}
