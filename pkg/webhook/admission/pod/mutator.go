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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
)

// +kubebuilder:webhook:path=/mutate-pods,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create,versions=v1,name=inferenceservice.kserve-webhook-server.pod-mutator,reinvocationPolicy=IfNeeded
var log = logf.Log.WithName(constants.PodMutatorWebhookName)

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client    client.Client
	Clientset kubernetes.Interface
	Decoder   admission.Decoder
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	if err := mutator.Decoder.Decode(req, pod); err != nil {
		log.Error(err, "Failed to decode pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !needMutate(pod) {
		return admission.ValidationResponse(true, "")
	}

	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, mutator.Clientset)
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

func (mutator *Mutator) mutate(pod *corev1.Pod, configMap *corev1.ConfigMap) error {
	credentialBuilder := credentials.NewCredentialBuilder(mutator.Client, mutator.Clientset, configMap)

	storageInitializerConfig, err := getStorageInitializerConfigs(configMap)
	if err != nil {
		return err
	}

	storageInitializer := &StorageInitializerInjector{
		credentialBuilder: credentialBuilder,
		config:            storageInitializerConfig,
		client:            mutator.Client,
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

	metricsAggregator := newMetricsAggregator(configMap)

	mutators := []func(pod *corev1.Pod) error{
		InjectGKEAcceleratorSelector,
		storageInitializer.InjectStorageInitializer,
		storageInitializer.SetIstioCniSecurityContext,
		agentInjector.InjectAgent,
		metricsAggregator.InjectMetricsAggregator,
	}

	if storageInitializer.config.EnableOciImageSource {
		mutators = append(mutators, storageInitializer.InjectModelcar)
	}

	for _, mutator := range mutators {
		if err := mutator(pod); err != nil {
			return err
		}
	}

	return nil
}

func needMutate(pod *corev1.Pod) bool {
	// Skip webhook if pod not managed by kserve
	_, ok := pod.Labels[constants.InferenceServicePodLabelKey]
	return ok
}
