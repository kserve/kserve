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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	kservetypes "github.com/kserve/kserve/pkg/types"
)

// +kubebuilder:webhook:path=/mutate-pods,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create,versions=v1,name=inferenceservice.kserve-webhook-server.pod-mutator,reinvocationPolicy=IfNeeded
var log = logf.Log.WithName(constants.PodMutatorWebhookName)

// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get

// Mutator is a webhook that injects incoming pods. Handle additionally runs
// the CacheRequired import-record gate on every pod (see
// gateLocalModelCacheWaiting), which reads the claim the pod's local model
// cache bindings point at.
type Mutator struct {
	Client    client.Client
	Clientset kubernetes.Interface
	Decoder   admission.Decoder
	// APIReader bypasses the manager's cache for objects the manager
	// controllers do not watch (e.g. the PVC import-record check done for
	// CacheRequired local model caches, which has no cache informer and no
	// RBAC in the manager role). It must only be used for direct reads.
	APIReader client.Reader
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	if err := mutator.Decoder.Decode(req, pod); err != nil {
		log.Error(err, "Failed to decode pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusBadRequest, err)
	}

	// For some reason pod namespace is always empty when coming to pod mutator, need to set from admission request
	pod.Namespace = req.Namespace

	// Fail closed for CacheRequired pods bound to shared-PVC local model caches
	// whose import record does not attest the current claim. This runs before
	// needMutate so LLMInferenceService workload pods -- which do not carry the
	// InferenceService pod label and are otherwise skipped by the injection
	// pipeline -- are gated too. Rejecting the pod at creation is also what
	// closes the window where a workload's ReplicaSet keeps creating pods onto
	// a claim that was recreated empty after a delete: server-side
	// reconciliation may have failed closed, but only the webhook sees every
	// pod the ReplicaSet attempts. The gate reads the claim directly through
	// the APIReader: the manager has no PVC informer.
	if err := (&StorageInitializerInjector{client: mutator.Client, reader: mutator.APIReader}).gateLocalModelCacheWaiting(ctx, pod); err != nil {
		log.Error(err, "Failed local model cache import-record gate", "name", pod.Name, "namespace", pod.Namespace)
		return admission.Errored(http.StatusForbidden, err)
	}

	if !needMutate(pod) {
		return admission.ValidationResponse(true, "")
	}

	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, mutator.Clientset)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	isvcName := pod.Labels[constants.InferenceServiceLabel]
	isvc := &v1beta1.InferenceService{}
	if err := mutator.Client.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      isvcName,
	}, isvc); err != nil {
		// failing to look up the inference service is not fatal, it only prevents accessing the embedded logging spec.
		// log the error but allow the mutator to continue
		log.Info("Failed to get InferenceService", "name", isvcName, "err", err.Error())
	}

	if err := mutator.mutate(ctx, pod, configMap, isvc); err != nil {
		log.Error(err, "Failed to mutate pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(pod)
	if err != nil {
		log.Error(err, "Failed to marshal pod", "name", pod.Labels[constants.InferenceServicePodLabelKey])
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, patch)
}

func (mutator *Mutator) mutate(ctx context.Context, pod *corev1.Pod, configMap *corev1.ConfigMap, isvc *v1beta1.InferenceService) error {
	credentialBuilder := credentials.NewCredentialBuilder(mutator.Client, mutator.Clientset, configMap)

	storageInitializerConfig, err := v1beta1.GetStorageInitializerConfigs(configMap)
	if err != nil {
		return err
	}

	storageInitializer := &StorageInitializerInjector{
		credentialBuilder: credentialBuilder,
		config:            storageInitializerConfig,
		client:            mutator.Client,
		reader:            mutator.APIReader,
	}

	loggerConfig, err := getLoggerConfigs(pod, configMap, isvc)
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
		func(pod *corev1.Pod) error {
			return storageInitializer.InjectStorageInitializer(ctx, pod)
		},
		storageInitializer.SetIstioCniSecurityContext,
		agentInjector.InjectAgent,
		metricsAggregator.InjectMetricsAggregator,
	}

	// A LocalModel pod gets its model from the cache PVC, even when the original source URI is OCI.
	if _, cached := pod.Labels[constants.LocalModelLabel]; !cached && kservetypes.ResolveOciModelMode(storageInitializer.config) != "" {
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
